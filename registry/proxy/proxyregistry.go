package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/distribution/reference"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/client"
	"github.com/distribution/distribution/v3/internal/client/auth"
	"github.com/distribution/distribution/v3/internal/client/auth/challenge"
	"github.com/distribution/distribution/v3/internal/client/transport"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/proxy/scheduler"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
)

var repositoryTTL = 24 * 7 * time.Hour

// Without HTTP/2 health checks, a request on a connection that was silently
// dropped (e.g. by a NAT restart) blocks until the kernel TCP retransmission
// timeout, which is about 15 minutes on Linux.
const (
	upstreamSendPingTimeout = 30 * time.Second
	upstreamPingTimeout     = 15 * time.Second
)

// newUpstreamTransport returns a transport for talking to the remote registry
// that detects dead HTTP/2 connections.
func newUpstreamTransport(sendPingTimeout, pingTimeout time.Duration) http.RoundTripper {
	dt, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}
	t := dt.Clone()
	t.HTTP2 = &http.HTTP2Config{
		SendPingTimeout: sendPingTimeout,
		PingTimeout:     pingTimeout,
	}
	return t
}

// proxyingRegistry fetches content from a remote registry and caches it locally
type proxyingRegistry struct {
	embedded          distribution.Namespace // provides local registry functionality
	scheduler         *scheduler.TTLExpirationScheduler
	ttl               *time.Duration
	cacheWriteTimeout time.Duration
	rangePassthrough  bool
	remoteURL         url.URL
	authChallenger    authChallenger
	basicAuth         auth.CredentialStore
	transport         http.RoundTripper
}

// NewRegistryPullThroughCache creates a registry acting as a pull through cache
func NewRegistryPullThroughCache(ctx context.Context, registry distribution.Namespace, driver driver.StorageDriver, config configuration.Proxy) (distribution.Namespace, error) {
	remoteURL, err := url.Parse(config.RemoteURL)
	if err != nil {
		return nil, err
	}

	v := storage.NewVacuum(ctx, driver)

	var s *scheduler.TTLExpirationScheduler
	var ttl *time.Duration
	if config.TTL == nil {
		// Default TTL is 7 days
		ttl = &repositoryTTL
	} else if *config.TTL > 0 {
		ttl = config.TTL
	} else {
		// TTL is disabled, never expire
		ttl = nil
	}

	// Set default cache write timeout if not specified
	cacheWriteTimeout := 5 * time.Minute
	if config.CacheWriteTimeout != nil && *config.CacheWriteTimeout > 0 {
		cacheWriteTimeout = *config.CacheWriteTimeout
	}

	if ttl != nil {
		s = scheduler.New(ctx, driver, "/scheduler-state.json")
		s.OnBlobExpire(func(ref reference.Reference) error {
			var r reference.Canonical
			var ok bool
			if r, ok = ref.(reference.Canonical); !ok {
				return fmt.Errorf("unexpected reference type : %T", ref)
			}

			repo, err := registry.Repository(ctx, r)
			if err != nil {
				return err
			}

			blobs := repo.Blobs(ctx)

			// Clear the repository reference and descriptor caches
			err = blobs.Delete(ctx, r.Digest())
			if err != nil {
				return err
			}

			err = v.RemoveBlob(r.Digest().String())
			if err != nil {
				return err
			}

			return nil
		})

		s.OnManifestExpire(func(ref reference.Reference) error {
			var r reference.Canonical
			var ok bool
			if r, ok = ref.(reference.Canonical); !ok {
				return fmt.Errorf("unexpected reference type : %T", ref)
			}

			repo, err := registry.Repository(ctx, r)
			if err != nil {
				return err
			}

			manifests, err := repo.Manifests(ctx)
			if err != nil {
				return err
			}
			err = manifests.Delete(ctx, r.Digest())
			if err != nil {
				return err
			}
			return nil
		})

		err = s.Start()
		if err != nil {
			return nil, err
		}
	}

	tr := newUpstreamTransport(upstreamSendPingTimeout, upstreamPingTimeout)
	client := &http.Client{Transport: tr}

	cs, b, err := func() (auth.CredentialStore, auth.CredentialStore, error) {
		switch {
		case config.Exec != nil:
			cs, err := configureExecAuth(*config.Exec)
			return cs, cs, err
		default:
			return configureAuth(client, config.Username, config.Password, config.RemoteURL)
		}
	}()
	if err != nil {
		return nil, err
	}

	return &proxyingRegistry{
		embedded:          registry,
		scheduler:         s,
		ttl:               ttl,
		cacheWriteTimeout: cacheWriteTimeout,
		rangePassthrough:  config.RangePassthrough,
		remoteURL:         *remoteURL,
		authChallenger: &remoteAuthChallenger{
			remoteURL: *remoteURL,
			cm:        challenge.NewSimpleManager(),
			cs:        cs,
			client:    client,
		},
		basicAuth: b,
		transport: tr,
	}, nil
}

func (pr *proxyingRegistry) Scope() distribution.Scope {
	return distribution.GlobalScope
}

func (pr *proxyingRegistry) Repositories(ctx context.Context, repos []string, last string) (n int, err error) {
	return pr.embedded.Repositories(ctx, repos, last)
}

func (pr *proxyingRegistry) Repository(ctx context.Context, name reference.Named) (distribution.Repository, error) {
	c := pr.authChallenger

	tkopts := auth.TokenHandlerOptions{
		Transport:   pr.transport,
		Credentials: c.credentialStore(),
		Scopes: []auth.Scope{
			auth.RepositoryScope{
				Repository: name.Name(),
				Actions:    []string{"pull"},
			},
		},
		Logger: dcontext.GetLogger(ctx),
	}

	tr := transport.NewTransport(pr.transport,
		auth.NewAuthorizer(c.challengeManager(),
			auth.NewTokenHandlerWithOptions(tkopts),
			auth.NewBasicHandler(pr.basicAuth)))

	localRepo, err := pr.embedded.Repository(ctx, name)
	if err != nil {
		return nil, err
	}
	localManifests, err := localRepo.Manifests(ctx, storage.SkipLayerVerification())
	if err != nil {
		return nil, err
	}

	remoteRepo, err := client.NewRepository(name, pr.remoteURL.String(), tr)
	if err != nil {
		return nil, err
	}

	remoteManifests, err := remoteRepo.Manifests(ctx)
	if err != nil {
		return nil, err
	}

	return &proxiedRepository{
		blobStore: &proxyBlobStore{
			localStore:        localRepo.Blobs(ctx),
			remoteStore:       remoteRepo.Blobs(ctx),
			scheduler:         pr.scheduler,
			ttl:               pr.ttl,
			cacheWriteTimeout: pr.cacheWriteTimeout,
			rangePassthrough:  pr.rangePassthrough,
			repositoryName:    name,
			authChallenger:    pr.authChallenger,
		},
		manifests: &proxyManifestStore{
			repositoryName:  name,
			localManifests:  localManifests, // Options?
			remoteManifests: remoteManifests,
			ctx:             ctx,
			scheduler:       pr.scheduler,
			ttl:             pr.ttl,
			authChallenger:  pr.authChallenger,
		},
		name: name,
		tags: &proxyTagService{
			localTags:      localRepo.Tags(ctx),
			remoteTags:     remoteRepo.Tags(ctx),
			authChallenger: pr.authChallenger,
		},
	}, nil
}

func (pr *proxyingRegistry) Blobs() distribution.BlobEnumerator {
	return pr.embedded.Blobs()
}

func (pr *proxyingRegistry) BlobStatter() distribution.BlobStatter {
	return pr.embedded.BlobStatter()
}

type Closer interface {
	// Close release all resources used by this object
	Close() error
}

func (pr *proxyingRegistry) Close() error {
	// Do not close idle connections of the shared http.DefaultTransport.
	if t, ok := pr.transport.(interface{ CloseIdleConnections() }); ok && pr.transport != http.DefaultTransport {
		t.CloseIdleConnections()
	}
	if pr.scheduler == nil {
		return nil
	}
	return pr.scheduler.Stop()
}

// authChallenger encapsulates a request to the upstream to establish credential challenges
type authChallenger interface {
	tryEstablishChallenges(context.Context) error
	challengeManager() challenge.Manager
	credentialStore() auth.CredentialStore
}

type remoteAuthChallenger struct {
	remoteURL url.URL
	sync.Mutex
	cm     challenge.Manager
	cs     auth.CredentialStore
	client *http.Client
}

func (r *remoteAuthChallenger) credentialStore() auth.CredentialStore {
	return r.cs
}

func (r *remoteAuthChallenger) challengeManager() challenge.Manager {
	return challenge.NewFilteringManager(r.cm, func(c challenge.Challenge) bool {
		return !strings.EqualFold(c.Scheme, "bearer") || realmAllowed(&r.remoteURL, c.Parameters["realm"])
	})
}

// tryEstablishChallenges will attempt to get a challenge type for the upstream if none currently exist
func (r *remoteAuthChallenger) tryEstablishChallenges(ctx context.Context) error {
	r.Lock()
	defer r.Unlock()

	remoteURL := r.remoteURL
	remoteURL.Path = "/v2/"
	challenges, err := r.cm.GetChallenges(remoteURL)
	if err != nil {
		return err
	}

	if len(challenges) > 0 {
		return nil
	}

	// establish challenge type with upstream
	if err := ping(r.client, r.cm, remoteURL.String(), challengeHeader); err != nil {
		return err
	}
	dcontext.GetLogger(ctx).Infof("Challenge established with upstream: %s", remoteURL.Redacted())

	return nil
}

// proxiedRepository uses proxying blob and manifest services to serve content
// locally, or pulling it through from a remote and caching it locally if it doesn't
// already exist
type proxiedRepository struct {
	blobStore distribution.BlobStore
	manifests distribution.ManifestService
	name      reference.Named
	tags      distribution.TagService
}

func (pr *proxiedRepository) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	return pr.manifests, nil
}

func (pr *proxiedRepository) Blobs(ctx context.Context) distribution.BlobStore {
	return pr.blobStore
}

func (pr *proxiedRepository) Named() reference.Named {
	return pr.name
}

func (pr *proxiedRepository) Tags(ctx context.Context) distribution.TagService {
	return pr.tags
}

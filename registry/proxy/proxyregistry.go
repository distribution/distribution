package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/distribution/reference"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/client"
	"github.com/distribution/distribution/v3/internal/client/auth"
	"github.com/distribution/distribution/v3/internal/client/auth/challenge"
	"github.com/distribution/distribution/v3/internal/client/transport"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
)

// InitFunc is the type of an EvictionController factory function and is used
// to register the constructor for different EvictionController backends.
type InitFunc func(ctx context.Context, driver driver.StorageDriver, path string, options map[string]interface{}) (EvictionController, error)

var evictionControllers map[string]InitFunc

func init() {
	evictionControllers = make(map[string]InitFunc)
}

// proxyingRegistry fetches content from a remote registry and caches it locally
type proxyingRegistry struct {
	embedded           distribution.Namespace // provides local registry functionality
	evictionController EvictionController
	remoteURL          url.URL
	authChallenger     authChallenger
	basicAuth          auth.CredentialStore
}

// NewRegistryPullThroughCache creates a registry acting as a pull through cache
func NewRegistryPullThroughCache(ctx context.Context, registry distribution.Namespace, driver driver.StorageDriver, config configuration.Proxy) (distribution.Namespace, error) {
	remoteURL, err := url.Parse(config.RemoteURL)
	if err != nil {
		return nil, err
	}

	v := storage.NewVacuum(ctx, driver)

	var evictionController EvictionController

	// Legacy TTL configuration
	if config.TTL != "" {
		if config.EvictionPolicy != nil {
			panic(fmt.Sprintf("unable to configure eviction, provided both eviction policy (%s) and ttl (%s)", config.EvictionPolicy.Type(), config.TTL))
		}

		config.EvictionPolicy = &configuration.EvictionPolicy{
			"ttl": configuration.Parameters{
				"ttl": config.TTL,
			},
		}
	}

	if config.EvictionPolicy != nil {
		evictionPolicy := config.EvictionPolicy.Type()
		if evictionPolicy != "" && !strings.EqualFold(evictionPolicy, "none") {
			controller, err := getEvictionController(config.EvictionPolicy.Type(), ctx, driver, "/eviction-state.json", config.EvictionPolicy.Parameters())
			if err != nil {
				panic(fmt.Sprintf("unable to configure eviction (%s): %v", evictionPolicy, err))
			}
			evictionController = controller
		}
	}

	if evictionController != nil {
		evictionController.OnBlobEvict(func(ref reference.Reference) error {
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

		evictionController.OnManifestEvict(func(ref reference.Reference) error {
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

		err = evictionController.Start()
		if err != nil {
			return nil, err
		}
	}

	cs, b, err := func() (auth.CredentialStore, auth.CredentialStore, error) {
		switch {
		case config.Exec != nil:
			cs, err := configureExecAuth(*config.Exec)
			return cs, cs, err
		default:
			return configureAuth(config.Username, config.Password, config.RemoteURL)
		}
	}()
	if err != nil {
		return nil, err
	}

	return &proxyingRegistry{
		embedded:           registry,
		evictionController: evictionController,
		remoteURL:          *remoteURL,
		authChallenger: &remoteAuthChallenger{
			remoteURL: *remoteURL,
			cm:        challenge.NewSimpleManager(),
			cs:        cs,
		},
		basicAuth: b,
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
		Transport:   http.DefaultTransport,
		Credentials: c.credentialStore(),
		Scopes: []auth.Scope{
			auth.RepositoryScope{
				Repository: name.Name(),
				Actions:    []string{"pull"},
			},
		},
		Logger: dcontext.GetLogger(ctx),
	}

	tr := transport.NewTransport(http.DefaultTransport,
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
			localStore:         localRepo.Blobs(ctx),
			remoteStore:        remoteRepo.Blobs(ctx),
			evictionController: pr.evictionController,
			repositoryName:     name,
			authChallenger:     pr.authChallenger,
		},
		manifests: &proxyManifestStore{
			repositoryName:     name,
			localManifests:     localManifests, // Options?
			remoteManifests:    remoteManifests,
			ctx:                ctx,
			evictionController: pr.evictionController,
			authChallenger:     pr.authChallenger,
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
	return pr.evictionController.Stop()
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
	cm challenge.Manager
	cs auth.CredentialStore
}

func (r *remoteAuthChallenger) credentialStore() auth.CredentialStore {
	return r.cs
}

func (r *remoteAuthChallenger) challengeManager() challenge.Manager {
	return r.cm
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
	if err := ping(r.cm, remoteURL.String(), challengeHeader); err != nil {
		return err
	}

	dcontext.GetLogger(ctx).Infof("Challenge established with upstream : %s %s", remoteURL, r.cm)
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

// OnEvictFunc is called when a cached entry is evicted
type OnEvictFunc func(reference.Reference) error

// EvictionController controls the eviction policy that the proxied registry follows.
type EvictionController interface {
	Start() error
	Stop() error
	// OnBlobEvict attaches the function f to run on a reference when a blob is evicted
	OnBlobEvict(f OnEvictFunc)
	// OnManifestEvict attaches the function f to run on a reference when a manifest is evicted
	OnManifestEvict(f OnEvictFunc)
	// AddBlob adds a blob to the eviction policy and errors when it is unable to
	AddBlob(blobRef reference.Canonical) error
	// AddManifest adds a blob to the eviction policy and errors when it is unable to
	AddManifest(manifestRef reference.Canonical) error
}

// Register is used to register an InitFunc for
// an EvictionController backend with the given name.
func Register(name string, initFunc InitFunc) error {
	if _, exists := evictionControllers[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	evictionControllers[name] = initFunc

	return nil
}

// getEvictionController constructs an EvictionController
// with the given options using the named backend.
func getEvictionController(name string, ctx context.Context, driver driver.StorageDriver, path string, options map[string]interface{}) (EvictionController, error) {
	if initFunc, exists := evictionControllers[name]; exists {
		return initFunc(ctx, driver, path, options)
	}

	return nil, fmt.Errorf("no access controller registered with name: %s", name)
}

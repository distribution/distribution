package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"

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

// proxyingRegistry fetches content from a remote registry and caches it locally
type proxyingRegistry struct {
	embedded          distribution.Namespace // provides local registry functionality
	scheduler         *scheduler.TTLExpirationScheduler
	ttl               *time.Duration
	cacheWriteTimeout time.Duration
	remoteURL         url.URL
	authChallenger    authChallenger
	basicAuth         auth.CredentialStore
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
			r, ok := ref.(reference.Canonical)
			if !ok {
				return fmt.Errorf("unexpected reference type : %T", ref)
			}
			return evictBlob(ctx, registry, driver, v, s, r)
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

		s.OnReconcile(reconcileFromStorage(ctx, driver, *ttl))

		err = s.Start()
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
		embedded:          registry,
		scheduler:         s,
		ttl:               ttl,
		cacheWriteTimeout: cacheWriteTimeout,
		remoteURL:         *remoteURL,
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
			localStore:        localRepo.Blobs(ctx),
			remoteStore:       remoteRepo.Blobs(ctx),
			scheduler:         pr.scheduler,
			ttl:               pr.ttl,
			cacheWriteTimeout: pr.cacheWriteTimeout,
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
	cm challenge.Manager
	cs auth.CredentialStore
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
	if err := ping(r.cm, remoteURL.String(), challengeHeader); err != nil {
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

// evictBlob handles a scheduled blob expiry for repo@digest in r. It
// acquires the scheduler's per-digest eviction lock first so that
// concurrent expiries for the same digest run one after the other rather
// than racing; only then it deletes the per-repository link (clearing the
// descriptor cache for the expiring repo). The shared blob file is
// vacuumed only when no other scheduled entry is still live for the
// digest and no other on-disk link still references it — by the time the
// last serialised eviction runs, every prior link has been deleted and
// every prior entry has been dropped, so that callback observes "no
// surviving references" and reclaims the blob.
// Split out from the OnBlobExpire closure so it can be unit-tested without
// constructing a full HTTP-backed proxyingRegistry.
func evictBlob(ctx context.Context, registry distribution.Namespace, drv driver.StorageDriver, v storage.Vacuum, s *scheduler.TTLExpirationScheduler, r reference.Canonical) error {
	evictMu := s.EvictionLock(r.Digest())
	evictMu.Lock()
	defer evictMu.Unlock()

	repo, err := registry.Repository(ctx, r)
	if err != nil {
		return err
	}

	if err := repo.Blobs(ctx).Delete(ctx, r.Digest()); err != nil {
		return err
	}

	expiringKey := r.String()
	if s.HasOtherReferencesToDigest(expiringKey, r.Digest()) {
		dcontext.GetLogger(ctx).Infof(
			"skipping blob vacuum for %s: another scheduled entry still references the digest",
			r.Digest())
		return nil
	}

	hasLink, err := anyRepoHasBlobLink(ctx, drv, r.Digest())
	if err != nil {
		return fmt.Errorf("checking surviving blob links for %s: %w", r.Digest(), err)
	}
	if hasLink {
		dcontext.GetLogger(ctx).Infof(
			"skipping blob vacuum for %s: a repository link still references the digest on disk",
			r.Digest())
		return nil
	}

	return v.RemoveBlob(r.Digest().String())
}

// errStopWalk is a sentinel returned from the Walk callback to stop the
// traversal on the first matching link file. Drivers may wrap the
// returned error so the surviving result is conveyed through a separate
// found flag captured in the closure, not via errors.Is.
var errStopWalk = errors.New("stop walk")

// anyRepoHasBlobLink reports whether any repository in storage holds a
// layer link pointing at dgst. It walks the repositories tree directly via
// the storage driver so it can find orphan link files that would not be
// reported by Namespace.Repositories (which requires the per-repo
// `_manifests` marker directory to exist). Pruning skips every subtree
// that cannot contain a match, so the typical cost is one descent per
// repository.
func anyRepoHasBlobLink(ctx context.Context, drv driver.StorageDriver, dgst digest.Digest) (bool, error) {
	root := storage.RepositoriesRootPath()
	alg := string(dgst.Algorithm())
	hex := dgst.Encoded()
	matchSuffix := "/_layers/" + alg + "/" + hex + "/link"

	var found bool
	err := drv.Walk(ctx, root, func(info driver.FileInfo) error {
		p := info.Path()
		if info.IsDir() {
			switch path.Base(p) {
			case "_manifests", "_uploads":
				return driver.ErrSkipDir
			}
			// Inside `_layers`, the layout is <alg>/<hex>/. Prune any
			// algorithm or digest hex that does not match the target.
			if idx := strings.LastIndex(p, "/_layers/"); idx >= 0 {
				sub := p[idx+len("/_layers/"):]
				segs := strings.Split(sub, "/")
				switch len(segs) {
				case 1:
					if segs[0] != alg {
						return driver.ErrSkipDir
					}
				case 2:
					if segs[0] != alg || segs[1] != hex {
						return driver.ErrSkipDir
					}
				}
			}
			return nil
		}
		if strings.HasSuffix(p, matchSuffix) {
			found = true
			return errStopWalk
		}
		return nil
	})
	if found {
		return true, nil
	}
	if err != nil {
		if errors.As(err, &driver.PathNotFoundError{}) {
			return false, nil
		}
		return false, fmt.Errorf("walking %s: %w", root, err)
	}
	return false, nil
}

// reconcileFromStorage returns a scheduler.Reconciler that walks the
// repositories tree once, discovers blob and manifest link files that have
// no entry in the scheduler, and schedules them with ttl. Designed to
// recover from prior unclean shutdowns and from state written by binaries
// that did not maintain the scheduler index for all on-disk links.
func reconcileFromStorage(ctx context.Context, drv driver.StorageDriver, ttl time.Duration) scheduler.Reconciler {
	return func(s *scheduler.TTLExpirationScheduler) error {
		root := storage.RepositoriesRootPath()
		var blobCount, manifestCount int
		walkErr := drv.Walk(ctx, root, func(info driver.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			repo, kind, dgst, ok := parseRepoLinkPath(root, info.Path())
			if !ok {
				return nil
			}
			named, err := reference.WithName(repo)
			if err != nil {
				dcontext.GetLogger(ctx).Debugf("reconcile: invalid repository name %q: %v", repo, err)
				return nil
			}
			canonical, err := reference.WithDigest(named, dgst)
			if err != nil {
				dcontext.GetLogger(ctx).Debugf("reconcile: cannot canonicalize %s@%s: %v", repo, dgst, err)
				return nil
			}
			switch kind {
			case linkKindBlob:
				added, addErr := s.AddBlobIfAbsent(canonical, ttl)
				if addErr != nil {
					dcontext.GetLogger(ctx).Warnf("reconcile: AddBlobIfAbsent %s failed: %v", canonical, addErr)
					return nil
				}
				if added {
					blobCount++
				}
			case linkKindManifest:
				added, addErr := s.AddManifestIfAbsent(canonical, ttl)
				if addErr != nil {
					dcontext.GetLogger(ctx).Warnf("reconcile: AddManifestIfAbsent %s failed: %v", canonical, addErr)
					return nil
				}
				if added {
					manifestCount++
				}
			}
			return nil
		})
		if walkErr != nil {
			if errors.As(walkErr, &driver.PathNotFoundError{}) {
				return nil
			}
			return fmt.Errorf("walking %s: %w", root, walkErr)
		}
		dcontext.GetLogger(ctx).Infof(
			"scheduler bootstrap reconcile: discovered %d orphan blob links, %d orphan manifest links",
			blobCount, manifestCount)
		return nil
	}
}

const (
	linkKindBlob     = "blob"
	linkKindManifest = "manifest"

	layersInfix    = "/_layers/"
	manifestsInfix = "/_manifests/revisions/"
	linkSuffix     = "/link"
)

// parseRepoLinkPath extracts (repository name, link kind, digest) from a
// driver path rooted at root, returning ok=false for anything that is not
// a recognised layer or manifest revision link file. Repository names
// cannot contain "_layers" or "_manifests" as a path component (the
// reference grammar forbids leading underscores), so the infix search is
// unambiguous.
func parseRepoLinkPath(root, p string) (repo string, kind string, dgst digest.Digest, ok bool) {
	prefix := root + "/"
	if !strings.HasPrefix(p, prefix) || !strings.HasSuffix(p, linkSuffix) {
		return "", "", "", false
	}
	sub := p[len(prefix):]

	if idx := strings.Index(sub, layersInfix); idx >= 0 {
		return parseAlgHex(sub[:idx], sub[idx+len(layersInfix):], linkKindBlob)
	}
	if idx := strings.Index(sub, manifestsInfix); idx >= 0 {
		return parseAlgHex(sub[:idx], sub[idx+len(manifestsInfix):], linkKindManifest)
	}
	return "", "", "", false
}

func parseAlgHex(repo, rest, kind string) (string, string, digest.Digest, bool) {
	// rest must be exactly "<algorithm>/<encoded>/link"
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[2] != "link" {
		return "", "", "", false
	}
	d := digest.NewDigestFromEncoded(digest.Algorithm(parts[0]), parts[1])
	if err := d.Validate(); err != nil {
		return "", "", "", false
	}
	return repo, kind, d, true
}

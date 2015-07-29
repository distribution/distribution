package proxy

import (
	"net/http"
	"net/url"

	"github.com/docker/distribution"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/client"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/transport"
	"github.com/docker/distribution/registry/proxy/scheduler"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver"
)

// proxyingRegistry fetches content from a remote registry and caches it locally
type proxyingRegistry struct {
	embedded distribution.Namespace // provides local registry functionality

	scheduler *scheduler.TTLExpirationScheduler

	remoteURL        string
	credentialStore  auth.CredentialStore
	challengeManager auth.ChallengeManager
}

// NewRegistryPullThroughCache creates a registry acting as a pull through cache
func NewRegistryPullThroughCache(ctx context.Context, registry distribution.Namespace, driver driver.StorageDriver, config configuration.Proxy) (distribution.Namespace, error) {
	_, err := url.Parse(config.RemoteURL)
	if err != nil {
		return nil, err
	}

	v := storage.NewVacuum(ctx, driver)

	s := scheduler.New(ctx, driver, "/scheduler-state.json")
	s.OnBlobExpire(func(digest string) error {
		return v.RemoveBlob(digest)
	})
	s.OnManifestExpire(func(repoName string) error {
		return v.RemoveRepository(repoName)
	})
	err = s.Start()
	if err != nil {
		return nil, err
	}

	challengeManager := auth.NewSimpleChallengeManager()
	cs, err := ConfigureAuth(config.RemoteURL, config.Username, config.Password, challengeManager)
	if err != nil {
		return nil, err
	}

	return &proxyingRegistry{
		embedded:         registry,
		scheduler:        s,
		challengeManager: challengeManager,
		credentialStore:  cs,
		remoteURL:        config.RemoteURL,
	}, nil
}

func (pr *proxyingRegistry) Scope() distribution.Scope {
	return distribution.GlobalScope
}

func (pr *proxyingRegistry) Repositories(ctx context.Context, repos []string, last string) (n int, err error) {
	return pr.embedded.Repositories(ctx, repos, last)
}

func (pr *proxyingRegistry) Repository(ctx context.Context, name string) (distribution.Repository, error) {
	tr := transport.NewTransport(http.DefaultTransport,
		auth.NewAuthorizer(pr.challengeManager, auth.NewTokenHandler(http.DefaultTransport, pr.credentialStore, name, "pull")))

	localRepo, err := pr.embedded.Repository(ctx, name)
	if err != nil {
		return nil, err
	}
	localManifests, err := localRepo.Manifests(ctx, storage.SkipLayerVerification)
	if err != nil {
		return nil, err
	}

	remoteRepo, err := client.NewRepository(ctx, name, pr.remoteURL, tr)
	if err != nil {
		return nil, err
	}

	remoteManifests, err := remoteRepo.Manifests(ctx)
	if err != nil {
		return nil, err
	}

	return &proxiedRepository{
		blobStore: proxyBlobStore{
			localStore:  localRepo.Blobs(ctx),
			remoteStore: remoteRepo.Blobs(ctx),
			scheduler:   pr.scheduler,
		},
		manifests: proxyManifestStore{
			repositoryName:  name,
			localManifests:  localManifests, // Options?
			remoteManifests: remoteManifests,
			ctx:             ctx,
			scheduler:       pr.scheduler,
		},
		name:       name,
		signatures: localRepo.Signatures(),
	}, nil
}

// proxiedRepository uses proxying blob and manifest services to serve content
// locally, or pulling it through from a remote and caching it locally if it doesn't
// already exist
type proxiedRepository struct {
	blobStore  distribution.BlobStore
	manifests  distribution.ManifestService
	name       string
	signatures distribution.SignatureService
}

func (pr *proxiedRepository) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	// options
	return pr.manifests, nil
}

func (pr *proxiedRepository) Blobs(ctx context.Context) distribution.BlobStore {
	return pr.blobStore
}

func (pr *proxiedRepository) Name() string {
	return pr.name
}

func (pr *proxiedRepository) Signatures() distribution.SignatureService {
	return pr.signatures
}

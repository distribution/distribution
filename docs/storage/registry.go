package storage

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/registry/storage/cache"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// registry is the top-level implementation of Registry for use in the storage
// package. All instances should descend from this object.
type registry struct {
	blobStore                   *blobStore
	blobServer                  *blobServer
	statter                     *blobStatter // global statter service.
	blobDescriptorCacheProvider cache.BlobDescriptorCacheProvider
	deleteEnabled               bool
	resumableDigestEnabled      bool
}

// RegistryOption is the type used for functional options for NewRegistry.
type RegistryOption func(*registry) error

// EnableRedirect is a functional option for NewRegistry. It causes the backend
// blob server to attempt using (StorageDriver).URLFor to serve all blobs.
func EnableRedirect(registry *registry) error {
	registry.blobServer.redirect = true
	return nil
}

// EnableDelete is a functional option for NewRegistry. It enables deletion on
// the registry.
func EnableDelete(registry *registry) error {
	registry.deleteEnabled = true
	return nil
}

// DisableDigestResumption is a functional option for NewRegistry. It should be
// used if the registry is acting as a caching proxy.
func DisableDigestResumption(registry *registry) error {
	registry.resumableDigestEnabled = false
	return nil
}

// BlobDescriptorCacheProvider returns a functional option for
// NewRegistry. It creates a cached blob statter for use by the
// registry.
func BlobDescriptorCacheProvider(blobDescriptorCacheProvider cache.BlobDescriptorCacheProvider) RegistryOption {
	// TODO(aaronl): The duplication of statter across several objects is
	// ugly, and prevents us from using interface types in the registry
	// struct. Ideally, blobStore and blobServer should be lazily
	// initialized, and use the current value of
	// blobDescriptorCacheProvider.
	return func(registry *registry) error {
		if blobDescriptorCacheProvider != nil {
			statter := cache.NewCachedBlobStatter(blobDescriptorCacheProvider, registry.statter)
			registry.blobStore.statter = statter
			registry.blobServer.statter = statter
			registry.blobDescriptorCacheProvider = blobDescriptorCacheProvider
		}
		return nil
	}
}

// NewRegistry creates a new registry instance from the provided driver. The
// resulting registry may be shared by multiple goroutines but is cheap to
// allocate. If the Redirect option is specified, the backend blob server will
// attempt to use (StorageDriver).URLFor to serve all blobs.
func NewRegistry(ctx context.Context, driver storagedriver.StorageDriver, options ...RegistryOption) (distribution.Namespace, error) {
	// create global statter
	statter := &blobStatter{
		driver: driver,
	}

	bs := &blobStore{
		driver:  driver,
		statter: statter,
	}

	registry := &registry{
		blobStore: bs,
		blobServer: &blobServer{
			driver:  driver,
			statter: statter,
			pathFn:  bs.path,
		},
		statter:                statter,
		resumableDigestEnabled: true,
	}

	for _, option := range options {
		if err := option(registry); err != nil {
			return nil, err
		}
	}

	return registry, nil
}

// Scope returns the namespace scope for a registry. The registry
// will only serve repositories contained within this scope.
func (reg *registry) Scope() distribution.Scope {
	return distribution.GlobalScope
}

// Repository returns an instance of the repository tied to the registry.
// Instances should not be shared between goroutines but are cheap to
// allocate. In general, they should be request scoped.
func (reg *registry) Repository(ctx context.Context, name string) (distribution.Repository, error) {
	if err := v2.ValidateRepositoryName(name); err != nil {
		return nil, distribution.ErrRepositoryNameInvalid{
			Name:   name,
			Reason: err,
		}
	}

	var descriptorCache distribution.BlobDescriptorService
	if reg.blobDescriptorCacheProvider != nil {
		var err error
		descriptorCache, err = reg.blobDescriptorCacheProvider.RepositoryScoped(name)
		if err != nil {
			return nil, err
		}
	}

	return &repository{
		ctx:             ctx,
		registry:        reg,
		name:            name,
		descriptorCache: descriptorCache,
	}, nil
}

// repository provides name-scoped access to various services.
type repository struct {
	*registry
	ctx             context.Context
	name            string
	descriptorCache distribution.BlobDescriptorService
}

// Name returns the name of the repository.
func (repo *repository) Name() string {
	return repo.name
}

// Manifests returns an instance of ManifestService. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (repo *repository) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	manifestLinkPathFns := []linkPathFunc{
		// NOTE(stevvooe): Need to search through multiple locations since
		// 2.1.0 unintentionally linked into  _layers.
		manifestRevisionLinkPath,
		blobLinkPath,
	}

	ms := &manifestStore{
		ctx:        ctx,
		repository: repo,
		revisionStore: &revisionStore{
			ctx:        ctx,
			repository: repo,
			blobStore: &linkedBlobStore{
				ctx:           ctx,
				blobStore:     repo.blobStore,
				repository:    repo,
				deleteEnabled: repo.registry.deleteEnabled,
				blobAccessController: &linkedBlobStatter{
					blobStore:   repo.blobStore,
					repository:  repo,
					linkPathFns: manifestLinkPathFns,
				},

				// TODO(stevvooe): linkPath limits this blob store to only
				// manifests. This instance cannot be used for blob checks.
				linkPathFns: manifestLinkPathFns,
			},
		},
		tagStore: &tagStore{
			ctx:        ctx,
			repository: repo,
			blobStore:  repo.registry.blobStore,
		},
	}

	// Apply options
	for _, option := range options {
		err := option(ms)
		if err != nil {
			return nil, err
		}
	}

	return ms, nil
}

// Blobs returns an instance of the BlobStore. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (repo *repository) Blobs(ctx context.Context) distribution.BlobStore {
	var statter distribution.BlobDescriptorService = &linkedBlobStatter{
		blobStore:   repo.blobStore,
		repository:  repo,
		linkPathFns: []linkPathFunc{blobLinkPath},
	}

	if repo.descriptorCache != nil {
		statter = cache.NewCachedBlobStatter(repo.descriptorCache, statter)
	}

	return &linkedBlobStore{
		blobStore:            repo.blobStore,
		blobServer:           repo.blobServer,
		blobAccessController: statter,
		repository:           repo,
		ctx:                  ctx,

		// TODO(stevvooe): linkPath limits this blob store to only layers.
		// This instance cannot be used for manifest checks.
		linkPathFns:   []linkPathFunc{blobLinkPath},
		deleteEnabled: repo.registry.deleteEnabled,
	}
}

func (repo *repository) Signatures() distribution.SignatureService {
	return &signatureStore{
		repository: repo,
		blobStore:  repo.blobStore,
		ctx:        repo.ctx,
	}
}

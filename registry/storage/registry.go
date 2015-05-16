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
	blobServer                  distribution.BlobServer
	statter                     distribution.BlobStatter // global statter service.
	blobDescriptorCacheProvider cache.BlobDescriptorCacheProvider
}

// NewRegistryWithDriver creates a new registry instance from the provided
// driver. The resulting registry may be shared by multiple goroutines but is
// cheap to allocate.
func NewRegistryWithDriver(ctx context.Context, driver storagedriver.StorageDriver, blobDescriptorCacheProvider cache.BlobDescriptorCacheProvider) distribution.Namespace {

	// create global statter, with cache.
	var statter distribution.BlobStatter = &blobStatter{
		driver: driver,
		pm:     defaultPathMapper,
	}

	if blobDescriptorCacheProvider != nil {
		statter = &cachedBlobStatter{
			cache:   blobDescriptorCacheProvider,
			backend: statter,
		}
	}

	bs := &blobStore{
		driver:  driver,
		pm:      defaultPathMapper,
		statter: statter,
	}

	return &registry{
		blobStore: bs,
		blobServer: &blobServer{
			driver:  driver,
			statter: statter,
			pathFn:  bs.path,
		},
		blobDescriptorCacheProvider: blobDescriptorCacheProvider,
	}
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
	if err := v2.ValidateRespositoryName(name); err != nil {
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
func (repo *repository) Manifests() distribution.ManifestService {
	return &manifestStore{
		ctx:        repo.ctx,
		repository: repo,
		revisionStore: &revisionStore{
			ctx:        repo.ctx,
			repository: repo,
			blobStore: &linkedBlobStore{
				ctx:        repo.ctx,
				blobStore:  repo.blobStore,
				repository: repo,
				statter: &linkedBlobStatter{
					blobStore:  repo.blobStore,
					repository: repo,
					linkPath:   manifestRevisionLinkPath,
				},

				// TODO(stevvooe): linkPath limits this blob store to only
				// manifests. This instance cannot be used for blob checks.
				linkPath: manifestRevisionLinkPath,
			},
		},
		tagStore: &tagStore{
			ctx:        repo.ctx,
			repository: repo,
			blobStore:  repo.registry.blobStore,
		},
	}
}

// Blobs returns an instance of the BlobStore. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (repo *repository) Blobs(ctx context.Context) distribution.BlobStore {
	var statter distribution.BlobStatter = &linkedBlobStatter{
		blobStore:  repo.blobStore,
		repository: repo,
		linkPath:   blobLinkPath,
	}

	if repo.descriptorCache != nil {
		statter = &cachedBlobStatter{
			cache:   repo.descriptorCache,
			backend: statter,
		}
	}

	return &linkedBlobStore{
		blobStore:  repo.blobStore,
		blobServer: repo.blobServer,
		statter:    statter,
		repository: repo,
		ctx:        ctx,

		// TODO(stevvooe): linkPath limits this blob store to only layers.
		// This instance cannot be used for manifest checks.
		linkPath: blobLinkPath,
	}
}

func (repo *repository) Signatures() distribution.SignatureService {
	return &signatureStore{
		repository: repo,
		blobStore:  repo.blobStore,
		ctx:        repo.ctx,
	}
}

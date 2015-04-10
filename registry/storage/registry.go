package storage

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/registry/storage/cache"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"golang.org/x/net/context"
)

// registry is the top-level implementation of Registry for use in the storage
// package. All instances should descend from this object.
type registry struct {
	driver         storagedriver.StorageDriver
	pm             *pathMapper
	blobStore      *blobStore
	layerInfoCache cache.LayerInfoCache
}

// NewRegistryWithDriver creates a new registry instance from the provided
// driver. The resulting registry may be shared by multiple goroutines but is
// cheap to allocate.
func NewRegistryWithDriver(driver storagedriver.StorageDriver, layerInfoCache cache.LayerInfoCache) distribution.Namespace {
	bs := &blobStore{
		driver: driver,
		pm:     defaultPathMapper,
	}

	return &registry{
		driver:    driver,
		blobStore: bs,

		// TODO(sday): This should be configurable.
		pm:             defaultPathMapper,
		layerInfoCache: layerInfoCache,
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

	return &repository{
		ctx:      ctx,
		registry: reg,
		name:     name,
	}, nil
}

// repository provides name-scoped access to various services.
type repository struct {
	*registry
	ctx  context.Context
	name string
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
		repository: repo,
		revisionStore: &revisionStore{
			repository: repo,
		},
		tagStore: &tagStore{
			repository: repo,
		},
	}
}

// Layers returns an instance of the LayerService. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (repo *repository) Layers() distribution.LayerService {
	ls := &layerStore{
		repository: repo,
	}

	if repo.registry.layerInfoCache != nil {
		// TODO(stevvooe): This is not the best place to setup a cache. We would
		// really like to decouple the cache from the backend but also have the
		// manifeset service use the layer service cache. For now, we can simply
		// integrate the cache directly. The main issue is that we have layer
		// access and layer data coupled in a single object. Work is already under
		// way to decouple this.

		return &cachedLayerService{
			LayerService: ls,
			repository:   repo,
			ctx:          repo.ctx,
			driver:       repo.driver,
			blobStore:    repo.blobStore,
			cache:        repo.registry.layerInfoCache,
		}
	}

	return ls
}

func (repo *repository) Signatures() distribution.SignatureService {
	return &signatureStore{
		repository: repo,
	}
}

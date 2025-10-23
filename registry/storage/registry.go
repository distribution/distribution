package storage

import (
	"context"
	"regexp"
	"runtime"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
)

var (
	DefaultConcurrencyLimit = runtime.GOMAXPROCS(0)
)

// registry is the top-level implementation of Registry for use in the storage
// package. All instances should descend from this object.
type registry struct {
	blobStore                    *blobStore
	blobServer                   *blobServer
	statter                      *blobStatter // global statter service.
	blobDescriptorCacheProvider  cache.BlobDescriptorCacheProvider
	deleteEnabled                bool
	tagLookupConcurrencyLimit    int
	resumableDigestEnabled       bool
	blobDescriptorServiceFactory distribution.BlobDescriptorServiceFactory
	driver                       storagedriver.StorageDriver

	// Validation
	manifestURLs         manifestURLs
	validateImageIndexes validateImageIndexes
}

// manifestURLs holds regular expressions for controlling manifest URL whitelisting
type manifestURLs struct {
	allow *regexp.Regexp
	deny  *regexp.Regexp
}

// validateImageIndexImages holds configuration for validation of image indexes
type validateImageIndexes struct {
	// exist can be used to disable checking that platform images exist entirely. Default true.
	imagesExist bool
	// platforms can be used to only validate the existence of images for a set of platforms. The empty array means validate all platforms.
	imagePlatforms []platform
}

// platform represents a platform to validate exists in the
type platform struct {
	architecture string
	os           string
}

// RegistryOption is the type used for functional options for NewRegistry.
type RegistryOption func(*registry) error

// EnableRedirect is a functional option for NewRegistry. It causes the backend
// blob server to attempt using (StorageDriver).RedirectURL to serve all blobs.
func EnableRedirect(registry *registry) error {
	registry.blobServer.redirect = true
	return nil
}

func TagLookupConcurrencyLimit(concurrencyLimit int) RegistryOption {
	return func(registry *registry) error {
		registry.tagLookupConcurrencyLimit = concurrencyLimit
		return nil
	}
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

// ManifestURLsAllowRegexp is a functional option for NewRegistry.
func ManifestURLsAllowRegexp(r *regexp.Regexp) RegistryOption {
	return func(registry *registry) error {
		registry.manifestURLs.allow = r
		return nil
	}
}

// ManifestURLsDenyRegexp is a functional option for NewRegistry.
func ManifestURLsDenyRegexp(r *regexp.Regexp) RegistryOption {
	return func(registry *registry) error {
		registry.manifestURLs.deny = r
		return nil
	}
}

// EnableValidateImageIndexImagesExist is a functional option for NewRegistry. It enables
// validation that references exist before an image index is accepted.
func EnableValidateImageIndexImagesExist(registry *registry) error {
	registry.validateImageIndexes.imagesExist = true
	return nil
}

// AddValidateImageIndexImagesExistPlatform returns a functional option for NewRegistry.
// It adds a platform to check for existence before an image index is accepted.
func AddValidateImageIndexImagesExistPlatform(architecture string, os string) RegistryOption {
	return func(registry *registry) error {
		registry.validateImageIndexes.imagePlatforms = append(
			registry.validateImageIndexes.imagePlatforms,
			platform{
				architecture: architecture,
				os:           os,
			},
		)
		return nil
	}
}

// BlobDescriptorServiceFactory returns a functional option for NewRegistry. It sets the
// factory to create BlobDescriptorServiceFactory middleware.
func BlobDescriptorServiceFactory(factory distribution.BlobDescriptorServiceFactory) RegistryOption {
	return func(registry *registry) error {
		registry.blobDescriptorServiceFactory = factory
		return nil
	}
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
// attempt to use (StorageDriver).RedirectURL to serve all blobs.
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
		driver:                 driver,
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
func (reg *registry) Repository(ctx context.Context, canonicalName reference.Named) (distribution.Repository, error) {
	var descriptorCache distribution.BlobDescriptorService
	if reg.blobDescriptorCacheProvider != nil {
		var err error
		descriptorCache, err = reg.blobDescriptorCacheProvider.RepositoryScoped(canonicalName.Name())
		if err != nil {
			return nil, err
		}
	}

	return &repository{
		ctx:             ctx,
		registry:        reg,
		name:            canonicalName,
		descriptorCache: descriptorCache,
	}, nil
}

func (reg *registry) Blobs() distribution.BlobEnumerator {
	return reg.blobStore
}

func (reg *registry) BlobStatter() distribution.BlobStatter {
	return reg.statter
}

// repository provides name-scoped access to various services.
type repository struct {
	*registry
	ctx             context.Context
	name            reference.Named
	descriptorCache distribution.BlobDescriptorService
}

// Name returns the name of the repository.
func (repo *repository) Named() reference.Named {
	return repo.name
}

func (repo *repository) Tags(ctx context.Context) distribution.TagService {
	limit := DefaultConcurrencyLimit
	if repo.tagLookupConcurrencyLimit > 0 {
		limit = repo.tagLookupConcurrencyLimit
	}
	tags := &tagStore{
		repository:       repo,
		blobStore:        repo.registry.blobStore,
		concurrencyLimit: limit,
	}

	return tags
}

// Manifests returns an instance of ManifestService. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (repo *repository) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	manifestDirectoryPathSpec := manifestRevisionsPathSpec{name: repo.name.Name()}

	var statter distribution.BlobDescriptorService = &linkedBlobStatter{
		blobStore:  repo.blobStore,
		repository: repo,
		linkPath:   manifestRevisionLinkPath,
	}

	if repo.descriptorCache != nil {
		statter = cache.NewCachedBlobStatter(repo.descriptorCache, statter)
	}

	if repo.registry.blobDescriptorServiceFactory != nil {
		statter = repo.registry.blobDescriptorServiceFactory.BlobAccessController(statter)
	}

	blobStore := &linkedBlobStore{
		ctx:                  ctx,
		blobStore:            repo.blobStore,
		repository:           repo,
		deleteEnabled:        repo.registry.deleteEnabled,
		blobAccessController: statter,

		// TODO(stevvooe): linkPath limits this blob store to only
		// manifests. This instance cannot be used for blob checks.
		linkPath:              manifestRevisionLinkPath,
		linkDirectoryPathSpec: manifestDirectoryPathSpec,
	}

	manifestListHandler := &manifestListHandler{
		ctx:                  ctx,
		repository:           repo,
		blobStore:            blobStore,
		validateImageIndexes: repo.validateImageIndexes,
	}

	ms := &manifestStore{
		ctx:        ctx,
		repository: repo,
		blobStore:  blobStore,
		schema2Handler: &schema2ManifestHandler{
			ctx:          ctx,
			repository:   repo,
			blobStore:    blobStore,
			manifestURLs: repo.registry.manifestURLs,
		},
		manifestListHandler: manifestListHandler,
		ocischemaHandler: &ocischemaManifestHandler{
			ctx:          ctx,
			repository:   repo,
			blobStore:    blobStore,
			manifestURLs: repo.registry.manifestURLs,
			references: &referenceHandler{
				blobStore:  repo.blobStore,
				repository: repo,
				pathFn:     subjectReferrerLinkPath,
			},
		},
		ocischemaIndexHandler: &ocischemaIndexHandler{
			manifestListHandler: manifestListHandler,
		},
	}

	// Apply options
	for _, option := range options {
		err := option.Apply(ms)
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
		blobStore:  repo.blobStore,
		repository: repo,
		linkPath:   blobLinkPath,
	}

	if repo.descriptorCache != nil {
		statter = cache.NewCachedBlobStatter(repo.descriptorCache, statter)
	}

	if repo.registry.blobDescriptorServiceFactory != nil {
		statter = repo.registry.blobDescriptorServiceFactory.BlobAccessController(statter)
	}

	return &linkedBlobStore{
		registry:             repo.registry,
		blobStore:            repo.blobStore,
		blobServer:           repo.blobServer,
		blobAccessController: statter,
		repository:           repo,
		ctx:                  ctx,

		// TODO(stevvooe): linkPath limits this blob store to only layers.
		// This instance cannot be used for manifest checks.
		linkPath:               blobLinkPath,
		linkDirectoryPathSpec:  layersPathSpec{name: repo.name.Name()},
		deleteEnabled:          repo.registry.deleteEnabled,
		resumableDigestEnabled: repo.resumableDigestEnabled,
	}
}

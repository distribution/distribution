package storage

import (
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storagedriver"
)

// Services provides various services with application-level operations for
// use across backend storage drivers.
type Services struct {
	driver     storagedriver.StorageDriver
	pathMapper *pathMapper
}

// NewServices creates a new Services object to access docker objects stored
// in the underlying driver.
func NewServices(driver storagedriver.StorageDriver) *Services {

	return &Services{
		driver: driver,
		// TODO(sday): This should be configurable.
		pathMapper: defaultPathMapper,
	}
}

// Layers returns an instance of the LayerService. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (ss *Services) Layers() LayerService {
	return &layerStore{
		driver: ss.driver,
		blobStore: &blobStore{
			driver: ss.driver,
			pm:     ss.pathMapper,
		},
		pathMapper: ss.pathMapper,
	}
}

// Manifests returns an instance of ManifestService. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (ss *Services) Manifests() ManifestService {
	// TODO(stevvooe): Lose this kludge. An intermediary object is clearly
	// missing here. This initialization is a mess.
	bs := &blobStore{
		driver: ss.driver,
		pm:     ss.pathMapper,
	}

	return &manifestStore{
		driver:     ss.driver,
		pathMapper: ss.pathMapper,
		revisionStore: &revisionStore{
			driver:     ss.driver,
			pathMapper: ss.pathMapper,
			blobStore:  bs,
		},
		tagStore: &tagStore{
			driver:     ss.driver,
			blobStore:  bs,
			pathMapper: ss.pathMapper,
		},
		blobStore:    bs,
		layerService: ss.Layers()}
}

// ManifestService provides operations on image manifests.
type ManifestService interface {
	// Tags lists the tags under the named repository.
	Tags(name string) ([]string, error)

	// Exists returns true if the manifest exists.
	Exists(name, tag string) (bool, error)

	// Get retrieves the named manifest, if it exists.
	Get(name, tag string) (*manifest.SignedManifest, error)

	// Put creates or updates the named manifest.
	Put(name, tag string, manifest *manifest.SignedManifest) error

	// Delete removes the named manifest, if it exists.
	Delete(name, tag string) error
}

// LayerService provides operations on layer files in a backend storage.
type LayerService interface {
	// Exists returns true if the layer exists.
	Exists(name string, digest digest.Digest) (bool, error)

	// Fetch the layer identifed by TarSum.
	Fetch(name string, digest digest.Digest) (Layer, error)

	// Upload begins a layer upload to repository identified by name,
	// returning a handle.
	Upload(name string) (LayerUpload, error)

	// Resume continues an in progress layer upload, returning a handle to the
	// upload. The caller should seek to the latest desired upload location
	// before proceeding.
	Resume(name, uuid string) (LayerUpload, error)
}

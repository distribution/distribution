package storage

import (
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storagedriver"
)

// Services provides various services with application-level operations for
// use across backend storage drivers.
type Services struct {
	driver           storagedriver.StorageDriver
	pathMapper       *pathMapper
	layerUploadStore layerUploadStore
}

// NewServices creates a new Services object to access docker objects stored
// in the underlying driver.
func NewServices(driver storagedriver.StorageDriver) *Services {
	layerUploadStore, err := newTemporaryLocalFSLayerUploadStore()

	if err != nil {
		// TODO(stevvooe): This failure needs to be understood in the context
		// of the lifecycle of the services object, which is uncertain at this
		// point.
		panic("unable to allocate layerUploadStore: " + err.Error())
	}

	return &Services{
		driver: driver,
		pathMapper: &pathMapper{
			// TODO(sday): This should be configurable.
			root:    "/docker/registry/",
			version: storagePathVersion,
		},
		layerUploadStore: layerUploadStore,
	}
}

// Layers returns an instance of the LayerService. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (ss *Services) Layers() LayerService {
	return &layerStore{driver: ss.driver, pathMapper: ss.pathMapper, uploadStore: ss.layerUploadStore}
}

// Manifests returns an instance of ManifestService. Instantiation is cheap and
// may be context sensitive in the future. The instance should be used similar
// to a request local.
func (ss *Services) Manifests() ManifestService {
	return &manifestStore{driver: ss.driver, pathMapper: ss.pathMapper, layerService: ss.Layers()}
}

// ManifestService provides operations on image manifests.
type ManifestService interface {
	// Tags lists the tags under the named repository.
	Tags(name string) ([]string, error)

	// Exists returns true if the layer exists.
	Exists(name, tag string) (bool, error)

	// Get retrieves the named manifest, if it exists.
	Get(name, tag string) (*SignedManifest, error)

	// Put creates or updates the named manifest.
	Put(name, tag string, manifest *SignedManifest) error

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

	// Resume continues an in progress layer upload, returning the current
	// state of the upload.
	Resume(uuid string) (LayerUpload, error)
}

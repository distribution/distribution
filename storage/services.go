package storage

import (
	"github.com/docker/docker-registry/storagedriver"
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

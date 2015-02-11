package storage

import (
	"fmt"
	"net/http"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// LayerHandler provides middleware for serving the contents of a Layer.
type LayerHandler interface {
	// Resolve returns an http.Handler which can serve the contents of a given
	// Layer if possible, or nil and an error when unsupported. This may
	// directly serve the contents of the layer or issue a redirect to another
	// URL hosting the content.
	Resolve(layer Layer) (http.Handler, error)
}

// LayerHandlerInitFunc is the type of a LayerHandler factory function and is
// used to register the contsructor for different LayerHandler backends.
type LayerHandlerInitFunc func(storageDriver storagedriver.StorageDriver, options map[string]interface{}) (LayerHandler, error)

var layerHandlers map[string]LayerHandlerInitFunc

// RegisterLayerHandler is used to register an LayerHandlerInitFunc for
// a LayerHandler backend with the given name.
func RegisterLayerHandler(name string, initFunc LayerHandlerInitFunc) error {
	if layerHandlers == nil {
		layerHandlers = make(map[string]LayerHandlerInitFunc)
	}
	if _, exists := layerHandlers[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	layerHandlers[name] = initFunc

	return nil
}

// GetLayerHandler constructs a LayerHandler
// with the given options using the named backend.
func GetLayerHandler(name string, options map[string]interface{}, storageDriver storagedriver.StorageDriver) (LayerHandler, error) {
	if layerHandlers != nil {
		if initFunc, exists := layerHandlers[name]; exists {
			return initFunc(storageDriver, options)
		}
	}

	return nil, fmt.Errorf("no layer handler registered with name: %s", name)
}

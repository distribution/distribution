package storage

import (
	"fmt"
	"net/http"
	"time"

	"github.com/docker/distribution/storagedriver"
)

// delegateLayerHandler provides a simple implementation of layerHandler that
// simply issues HTTP Temporary Redirects to the URL provided by the
// storagedriver for a given Layer.
type delegateLayerHandler struct {
	storageDriver storagedriver.StorageDriver
	pathMapper    *pathMapper
	duration      time.Duration
}

var _ LayerHandler = &delegateLayerHandler{}

func newDelegateLayerHandler(storageDriver storagedriver.StorageDriver, options map[string]interface{}) (LayerHandler, error) {
	duration := 20 * time.Minute
	d, ok := options["duration"]
	if ok {
		switch d := d.(type) {
		case time.Duration:
			duration = d
		case string:
			dur, err := time.ParseDuration(d)
			if err != nil {
				return nil, fmt.Errorf("Invalid duration: %s", err)
			}
			duration = dur
		}
	}

	return &delegateLayerHandler{storageDriver: storageDriver, pathMapper: defaultPathMapper, duration: duration}, nil
}

// Resolve returns an http.Handler which can serve the contents of the given
// Layer, or an error if not supported by the storagedriver.
func (lh *delegateLayerHandler) Resolve(layer Layer) (http.Handler, error) {
	// TODO(bbland): This is just a sanity check to ensure that the
	// storagedriver supports url generation. It would be nice if we didn't have
	// to do this twice for non-GET requests.
	layerURL, err := lh.urlFor(layer, map[string]interface{}{"method": "GET"})
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			layerURL, err = lh.urlFor(layer, map[string]interface{}{"method": r.Method})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, layerURL, http.StatusTemporaryRedirect)
	}), nil
}

// urlFor returns a download URL for the given layer, or the empty string if
// unsupported.
func (lh *delegateLayerHandler) urlFor(layer Layer, options map[string]interface{}) (string, error) {
	// Crack open the layer to get at the layerStore
	layerRd, ok := layer.(*layerReader)
	if !ok {
		// TODO(stevvooe): We probably want to find a better way to get at the
		// underlying filesystem path for a given layer. Perhaps, the layer
		// handler should have its own layer store but right now, it is not
		// request scoped.
		return "", fmt.Errorf("unsupported layer type: cannot resolve blob path: %v", layer)
	}

	if options == nil {
		options = make(map[string]interface{})
	}
	options["expiry"] = time.Now().Add(lh.duration)

	layerURL, err := lh.storageDriver.URLFor(layerRd.path, options)
	if err != nil {
		return "", err
	}

	return layerURL, nil
}

// init registers the delegate layerHandler backend.
func init() {
	RegisterLayerHandler("delegate", LayerHandlerInitFunc(newDelegateLayerHandler))
}

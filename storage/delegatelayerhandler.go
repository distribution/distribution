package storage

import (
	"net/http"
	"time"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storagedriver"
)

// delegateLayerHandler provides a simple implementation of layerHandler that
// simply issues HTTP Temporary Redirects to the URL provided by the
// storagedriver for a given Layer.
type delegateLayerHandler struct {
	storageDriver storagedriver.StorageDriver
	pathMapper    *pathMapper
}

var _ LayerHandler = &delegateLayerHandler{}

func newDelegateLayerHandler(storageDriver storagedriver.StorageDriver, options map[string]interface{}) (LayerHandler, error) {
	return &delegateLayerHandler{storageDriver: storageDriver, pathMapper: defaultPathMapper}, nil
}

// Resolve returns an http.Handler which can serve the contents of the given
// Layer, or an error if not supported by the storagedriver.
func (lh *delegateLayerHandler) Resolve(layer Layer) (http.Handler, error) {
	layerURL, err := lh.urlFor(layer)
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, layerURL, http.StatusTemporaryRedirect)
	}), nil
}

// urlFor returns a download URL for the given layer, or the empty string if
// unsupported.
func (lh *delegateLayerHandler) urlFor(layer Layer) (string, error) {
	blobPath, err := lh.resolveBlobPath(layer.Name(), layer.Digest())
	if err != nil {
		return "", err
	}

	layerURL, err := lh.storageDriver.URLFor(blobPath, map[string]interface{}{"expires": time.Now().Add(20 * time.Minute)})
	if err != nil {
		return "", err
	}

	return layerURL, nil
}

// resolveBlobPath looks up the blob location in the repositories from a
// layer/blob link file, returning blob path or an error on failure.
func (lh *delegateLayerHandler) resolveBlobPath(name string, dgst digest.Digest) (string, error) {
	pathSpec := layerLinkPathSpec{name: name, digest: dgst}
	layerLinkPath, err := lh.pathMapper.path(pathSpec)

	if err != nil {
		return "", err
	}

	layerLinkContent, err := lh.storageDriver.GetContent(layerLinkPath)
	if err != nil {
		return "", err
	}

	// NOTE(stevvooe): The content of the layer link should match the digest.
	// This layer of indirection is for name-based content protection.

	linked, err := digest.ParseDigest(string(layerLinkContent))
	if err != nil {
		return "", err
	}

	bp := blobPathSpec{digest: linked}

	return lh.pathMapper.path(bp)
}

// init registers the delegate layerHandler backend.
func init() {
	RegisterLayerHandler("delegate", LayerHandlerInitFunc(newDelegateLayerHandler))
}

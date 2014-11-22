package registry

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/docker/docker-registry/storage"
	"github.com/gorilla/handlers"
)

// imageManifestDispatcher takes the request context and builds the
// appropriate handler for handling image manifest requests.
func imageManifestDispatcher(ctx *Context, r *http.Request) http.Handler {
	imageManifestHandler := &imageManifestHandler{
		Context: ctx,
		Tag:     ctx.vars["tag"],
	}

	imageManifestHandler.log = imageManifestHandler.log.WithField("tag", imageManifestHandler.Tag)

	return handlers.MethodHandler{
		"GET":    http.HandlerFunc(imageManifestHandler.GetImageManifest),
		"PUT":    http.HandlerFunc(imageManifestHandler.PutImageManifest),
		"DELETE": http.HandlerFunc(imageManifestHandler.DeleteImageManifest),
	}
}

// imageManifestHandler handles http operations on image manifests.
type imageManifestHandler struct {
	*Context

	Tag string
}

// GetImageManifest fetches the image manifest from the storage backend, if it exists.
func (imh *imageManifestHandler) GetImageManifest(w http.ResponseWriter, r *http.Request) {

}

// PutImageManifest validates and stores and image in the registry.
func (imh *imageManifestHandler) PutImageManifest(w http.ResponseWriter, r *http.Request) {

}

// DeleteImageManifest removes the image with the given tag from the registry.
func (imh *imageManifestHandler) DeleteImageManifest(w http.ResponseWriter, r *http.Request) {

}

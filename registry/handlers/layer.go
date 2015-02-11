package handlers

import (
	"net/http"

	"github.com/docker/distribution/registry/api/v2"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage"
	"github.com/gorilla/handlers"
)

// layerDispatcher uses the request context to build a layerHandler.
func layerDispatcher(ctx *Context, r *http.Request) http.Handler {
	dgst, err := getDigest(ctx)
	if err != nil {

		if err == errDigestNotAvailable {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				ctx.Errors.Push(v2.ErrorCodeDigestInvalid, err)
			})
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx.Errors.Push(v2.ErrorCodeDigestInvalid, err)
		})
	}

	layerHandler := &layerHandler{
		Context: ctx,
		Digest:  dgst,
	}

	return handlers.MethodHandler{
		"GET":  http.HandlerFunc(layerHandler.GetLayer),
		"HEAD": http.HandlerFunc(layerHandler.GetLayer),
	}
}

// layerHandler serves http layer requests.
type layerHandler struct {
	*Context

	Digest digest.Digest
}

// GetLayer fetches the binary data from backend storage returns it in the
// response.
func (lh *layerHandler) GetLayer(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(lh).Debug("GetImageLayer")
	layers := lh.Repository.Layers()
	layer, err := layers.Fetch(lh.Digest)

	if err != nil {
		switch err := err.(type) {
		case storage.ErrUnknownLayer:
			w.WriteHeader(http.StatusNotFound)
			lh.Errors.Push(v2.ErrorCodeBlobUnknown, err.FSLayer)
		default:
			lh.Errors.Push(v2.ErrorCodeUnknown, err)
		}
		return
	}
	defer layer.Close()

	if lh.layerHandler != nil {
		handler, _ := lh.layerHandler.Resolve(layer)
		if handler != nil {
			handler.ServeHTTP(w, r)
			return
		}
	}

	http.ServeContent(w, r, layer.Digest().String(), layer.CreatedAt(), layer)
}

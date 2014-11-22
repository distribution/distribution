package registry

import (
	"net/http"

	"github.com/docker/docker-registry/digest"
	"github.com/docker/docker-registry/storage"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

// layerDispatcher uses the request context to build a layerHandler.
func layerDispatcher(ctx *Context, r *http.Request) http.Handler {
	dgst, err := digest.ParseDigest(ctx.vars["digest"])

	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx.Errors.Push(ErrorCodeInvalidDigest, err)
		})
	}

	layerHandler := &layerHandler{
		Context: ctx,
		Digest:  dgst,
	}

	layerHandler.log = layerHandler.log.WithField("digest", dgst)

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
	layers := lh.services.Layers()

	layer, err := layers.Fetch(lh.Name, lh.Digest)

	if err != nil {
		switch err {
		case storage.ErrLayerUnknown:
			w.WriteHeader(http.StatusNotFound)
			lh.Errors.Push(ErrorCodeUnknownLayer,
				map[string]interface{}{
					"unknown": storage.FSLayer{BlobSum: lh.Digest},
				})
			return
		default:
			lh.Errors.Push(ErrorCodeUnknown, err)
			return
		}
	}
	defer layer.Close()

	http.ServeContent(w, r, layer.Digest().String(), layer.CreatedAt(), layer)
}

func buildLayerURL(router *mux.Router, r *http.Request, layer storage.Layer) (string, error) {
	route := clonedRoute(router, routeNameBlob)

	layerURL, err := route.Schemes(r.URL.Scheme).Host(r.Host).
		URL("name", layer.Name(),
		"digest", layer.Digest().String())
	if err != nil {
		return "", err
	}

	return layerURL.String(), nil
}

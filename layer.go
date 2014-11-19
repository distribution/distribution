package registry

import (
	"net/http"

	"github.com/gorilla/handlers"
)

// layerDispatcher uses the request context to build a layerHandler.
func layerDispatcher(ctx *Context, r *http.Request) http.Handler {
	layerHandler := &layerHandler{
		Context: ctx,
		TarSum:  ctx.vars["tarsum"],
	}

	layerHandler.log = layerHandler.log.WithField("tarsum", layerHandler.TarSum)

	return handlers.MethodHandler{
		"GET":  http.HandlerFunc(layerHandler.GetLayer),
		"HEAD": http.HandlerFunc(layerHandler.GetLayer),
	}
}

// layerHandler serves http layer requests.
type layerHandler struct {
	*Context

	TarSum string
}

// GetLayer fetches the binary data from backend storage returns it in the
// response.
func (lh *layerHandler) GetLayer(w http.ResponseWriter, r *http.Request) {

}

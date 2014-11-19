package registry

import (
	"net/http"

	"github.com/gorilla/handlers"
)

// layerUploadDispatcher constructs and returns the layer upload handler for
// the given request context.
func layerUploadDispatcher(ctx *Context, r *http.Request) http.Handler {
	layerUploadHandler := &layerUploadHandler{
		Context: ctx,
		TarSum:  ctx.vars["tarsum"],
		UUID:    ctx.vars["uuid"],
	}

	layerUploadHandler.log = layerUploadHandler.log.WithField("tarsum", layerUploadHandler.TarSum)

	if layerUploadHandler.UUID != "" {
		layerUploadHandler.log = layerUploadHandler.log.WithField("uuid", layerUploadHandler.UUID)
	}

	return handlers.MethodHandler{
		"POST":   http.HandlerFunc(layerUploadHandler.StartLayerUpload),
		"GET":    http.HandlerFunc(layerUploadHandler.GetUploadStatus),
		"HEAD":   http.HandlerFunc(layerUploadHandler.GetUploadStatus),
		"PUT":    http.HandlerFunc(layerUploadHandler.PutLayerChunk),
		"DELETE": http.HandlerFunc(layerUploadHandler.CancelLayerUpload),
	}
}

// layerUploadHandler handles the http layer upload process.
type layerUploadHandler struct {
	*Context

	// TarSum is the unique identifier of the layer being uploaded.
	TarSum string

	// UUID identifies the upload instance for the current request.
	UUID string
}

// StartLayerUpload begins the layer upload process and allocates a server-
// side upload session.
func (luh *layerUploadHandler) StartLayerUpload(w http.ResponseWriter, r *http.Request) {

}

// GetUploadStatus returns the status of a given upload, identified by uuid.
func (luh *layerUploadHandler) GetUploadStatus(w http.ResponseWriter, r *http.Request) {

}

// PutLayerChunk receives a layer chunk during the layer upload process,
// possible completing the upload with a checksum and length.
func (luh *layerUploadHandler) PutLayerChunk(w http.ResponseWriter, r *http.Request) {

}

// CancelLayerUpload cancels an in-progress upload of a layer.
func (luh *layerUploadHandler) CancelLayerUpload(w http.ResponseWriter, r *http.Request) {

}

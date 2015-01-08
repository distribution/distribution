package registry

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/api/v2"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storage"
	"github.com/gorilla/handlers"
)

// layerUploadDispatcher constructs and returns the layer upload handler for
// the given request context.
func layerUploadDispatcher(ctx *Context, r *http.Request) http.Handler {
	luh := &layerUploadHandler{
		Context: ctx,
		UUID:    ctx.vars["uuid"],
	}

	handler := http.Handler(handlers.MethodHandler{
		"POST":   http.HandlerFunc(luh.StartLayerUpload),
		"GET":    http.HandlerFunc(luh.GetUploadStatus),
		"HEAD":   http.HandlerFunc(luh.GetUploadStatus),
		"PUT":    http.HandlerFunc(luh.PutLayerChunk),
		"DELETE": http.HandlerFunc(luh.CancelLayerUpload),
	})

	if luh.UUID != "" {
		luh.log = luh.log.WithField("uuid", luh.UUID)

		state, err := hmacKey(ctx.Config.HTTP.Secret).unpackUploadState(r.FormValue("_state"))
		if err != nil {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx.log.Infof("error resolving upload: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				luh.Errors.Push(v2.ErrorCodeBlobUploadInvalid, err)
			})
		}
		luh.State = state

		if state.UUID != luh.UUID {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx.log.Infof("mismatched uuid in upload state: %q != %q", state.UUID, luh.UUID)
				w.WriteHeader(http.StatusBadRequest)
				luh.Errors.Push(v2.ErrorCodeBlobUploadInvalid, err)
			})
		}

		layers := ctx.services.Layers()
		upload, err := layers.Resume(luh.Name, luh.UUID)
		if err != nil && err != storage.ErrLayerUploadUnknown {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx.log.Errorf("error resolving upload: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				luh.Errors.Push(v2.ErrorCodeBlobUploadUnknown, err)
			})
		}
		luh.Upload = upload

		if state.Offset > 0 {
			// Seek the layer upload to the correct spot if it's non-zero.
			// These error conditions should be rare and demonstrate really
			// problems. We basically cancel the upload and tell the client to
			// start over.
			if nn, err := upload.Seek(luh.State.Offset, os.SEEK_SET); err != nil {
				ctx.log.Infof("error seeking layer upload: %v", err)
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					luh.Errors.Push(v2.ErrorCodeBlobUploadInvalid, err)
					upload.Cancel()
				})
			} else if nn != luh.State.Offset {
				ctx.log.Infof("seek to wrong offest: %d != %d", nn, luh.State.Offset)
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					luh.Errors.Push(v2.ErrorCodeBlobUploadInvalid, err)
					upload.Cancel()
				})
			}
		}

		handler = closeResources(handler, luh.Upload)
	}

	return handler
}

// layerUploadHandler handles the http layer upload process.
type layerUploadHandler struct {
	*Context

	// UUID identifies the upload instance for the current request.
	UUID string

	Upload storage.LayerUpload

	State layerUploadState
}

// StartLayerUpload begins the layer upload process and allocates a server-
// side upload session.
func (luh *layerUploadHandler) StartLayerUpload(w http.ResponseWriter, r *http.Request) {
	layers := luh.services.Layers()
	upload, err := layers.Upload(luh.Name)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError) // Error conditions here?
		luh.Errors.Push(v2.ErrorCodeUnknown, err)
		return
	}

	luh.Upload = upload
	defer luh.Upload.Close()

	if err := luh.layerUploadResponse(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError) // Error conditions here?
		luh.Errors.Push(v2.ErrorCodeUnknown, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// GetUploadStatus returns the status of a given upload, identified by uuid.
func (luh *layerUploadHandler) GetUploadStatus(w http.ResponseWriter, r *http.Request) {
	if luh.Upload == nil {
		w.WriteHeader(http.StatusNotFound)
		luh.Errors.Push(v2.ErrorCodeBlobUploadUnknown)
	}

	if err := luh.layerUploadResponse(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError) // Error conditions here?
		luh.Errors.Push(v2.ErrorCodeUnknown, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PutLayerChunk receives a layer chunk during the layer upload process,
// possible completing the upload with a checksum and length.
func (luh *layerUploadHandler) PutLayerChunk(w http.ResponseWriter, r *http.Request) {
	if luh.Upload == nil {
		w.WriteHeader(http.StatusNotFound)
		luh.Errors.Push(v2.ErrorCodeBlobUploadUnknown)
	}

	var finished bool

	// TODO(stevvooe): This is woefully incomplete. Missing stuff:
	//
	// 1. Extract information from range header, if present.
	// 2. Check offset of current layer.
	// 3. Emit correct error responses.

	// Read in the chunk
	io.Copy(luh.Upload, r.Body)

	if err := luh.maybeCompleteUpload(w, r); err != nil {
		if err != errNotReadyToComplete {
			switch err := err.(type) {
			case storage.ErrLayerInvalidSize:
				w.WriteHeader(http.StatusBadRequest)
				luh.Errors.Push(v2.ErrorCodeSizeInvalid, err)
				return
			case storage.ErrLayerInvalidDigest:
				w.WriteHeader(http.StatusBadRequest)
				luh.Errors.Push(v2.ErrorCodeDigestInvalid, err)
				return
			default:
				w.WriteHeader(http.StatusInternalServerError)
				luh.Errors.Push(v2.ErrorCodeUnknown, err)
				return
			}
		}
	}

	if err := luh.layerUploadResponse(w, r); err != nil {
		w.WriteHeader(http.StatusInternalServerError) // Error conditions here?
		luh.Errors.Push(v2.ErrorCodeUnknown, err)
		return
	}

	if finished {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusAccepted)
	}
}

// CancelLayerUpload cancels an in-progress upload of a layer.
func (luh *layerUploadHandler) CancelLayerUpload(w http.ResponseWriter, r *http.Request) {
	if luh.Upload == nil {
		w.WriteHeader(http.StatusNotFound)
		luh.Errors.Push(v2.ErrorCodeBlobUploadUnknown)
	}

}

// layerUploadResponse provides a standard request for uploading layers and
// chunk responses. This sets the correct headers but the response status is
// left to the caller.
func (luh *layerUploadHandler) layerUploadResponse(w http.ResponseWriter, r *http.Request) error {

	offset, err := luh.Upload.Seek(0, os.SEEK_CUR)
	if err != nil {
		luh.log.Errorf("unable get current offset of layer upload: %v", err)
		return err
	}

	// TODO(stevvooe): Need a better way to manage the upload state automatically.
	luh.State.Name = luh.Name
	luh.State.UUID = luh.Upload.UUID()
	luh.State.Offset = offset
	luh.State.StartedAt = luh.Upload.StartedAt()

	token, err := hmacKey(luh.Config.HTTP.Secret).packUploadState(luh.State)
	if err != nil {
		logrus.Infof("error building upload state token: %s", err)
		return err
	}

	uploadURL, err := luh.urlBuilder.BuildBlobUploadChunkURL(
		luh.Upload.Name(), luh.Upload.UUID(),
		url.Values{
			"_state": []string{token},
		})
	if err != nil {
		logrus.Infof("error building upload url: %s", err)
		return err
	}

	w.Header().Set("Location", uploadURL)
	w.Header().Set("Content-Length", "0")
	w.Header().Set("Range", fmt.Sprintf("0-%d", luh.State.Offset))

	return nil
}

var errNotReadyToComplete = fmt.Errorf("not ready to complete upload")

// maybeCompleteUpload tries to complete the upload if the correct parameters
// are available. Returns errNotReadyToComplete if not ready to complete.
func (luh *layerUploadHandler) maybeCompleteUpload(w http.ResponseWriter, r *http.Request) error {
	// If we get a digest and length, we can finish the upload.
	dgstStr := r.FormValue("digest") // TODO(stevvooe): Support multiple digest parameters!

	if dgstStr == "" {
		return errNotReadyToComplete
	}

	dgst, err := digest.ParseDigest(dgstStr)
	if err != nil {
		return err
	}

	luh.completeUpload(w, r, dgst)
	return nil
}

// completeUpload finishes out the upload with the correct response.
func (luh *layerUploadHandler) completeUpload(w http.ResponseWriter, r *http.Request, dgst digest.Digest) {
	layer, err := luh.Upload.Finish(dgst)
	if err != nil {
		luh.Errors.Push(v2.ErrorCodeUnknown, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	layerURL, err := luh.urlBuilder.BuildBlobURL(layer.Name(), layer.Digest())
	if err != nil {
		luh.Errors.Push(v2.ErrorCodeUnknown, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Location", layerURL)
	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusCreated)
}

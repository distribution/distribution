package handlers

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/api/v2"
)

// layerUploadDispatcher constructs and returns the layer upload handler for
// the given request context.
func layerUploadHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (err error) {
	switch r.Method {
	case "POST":
		err = StartLayerUpload(ctx, w, r)
	case "GET", "HEAD":
		err = GetUploadStatus(ctx, w, r)
	case "PUT":
		err = PutLayerUploadComplete(ctx, w, r)
	case "DELETE":
		err = CancelLayerUpload(ctx, w, r)
	// TODO(stevvooe): Must implement patch support.
	// case "PATCH":
	//	PutLayerChunk(ctx, w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
	return err
}

func layerUploadInfo(ctx *Context, r *http.Request) (uuid string, upload distribution.LayerUpload, httpErr error) {
	uuid = getUploadUUID(ctx)
	if uuid != "" {
		state, err := hmacKey(ctx.Secret).unpackUploadState(r.FormValue("_state"))
		if err != nil {
			ctxu.GetLogger(ctx).Infof("error resolving upload: %v", err)
			httpErr = NewHTTPError(v2.ErrorCodeBlobUploadInvalid, err, http.StatusBadRequest)
			return
		}

		if state.Name != ctx.Repository.Name() {
			ctxu.GetLogger(ctx).Infof("mismatched repository name in upload state: %q != %q", state.Name, ctx.Repository.Name())
			httpErr = NewHTTPError(v2.ErrorCodeBlobUploadInvalid, err, http.StatusBadRequest)
			return
		}

		if state.UUID != uuid {
			ctxu.GetLogger(ctx).Infof("mismatched uuid in upload state: %q != %q", state.UUID, uuid)
			httpErr = NewHTTPError(v2.ErrorCodeBlobUploadInvalid, err, http.StatusBadRequest)
			return
		}

		layers := ctx.Repository.Layers()
		upload, err = layers.Resume(uuid)
		if err != nil {
			ctxu.GetLogger(ctx).Errorf("error resolving upload: %v", err)
			if err == distribution.ErrLayerUploadUnknown {
				httpErr = NewHTTPError(v2.ErrorCodeBlobUploadUnknown, err, http.StatusNotFound)
				return
			}

			httpErr = NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
			return
		}

		if state.Offset > 0 {
			// Seek the layer upload to the correct spot if it's non-zero.
			// These error conditions should be rare and demonstrate really
			// problems. We basically cancel the upload and tell the client to
			// start over.
			if nn, err := upload.Seek(state.Offset, os.SEEK_SET); err != nil {
				defer upload.Cancel() // Cancel calls Close
				ctxu.GetLogger(ctx).Infof("error seeking layer upload: %v", err)
				httpErr = NewHTTPError(v2.ErrorCodeBlobUploadInvalid, err, http.StatusBadRequest)
				return
			} else if nn != state.Offset {
				defer upload.Cancel()
				ctxu.GetLogger(ctx).Infof("seek to wrong offest: %d != %d", nn, state.Offset)
				httpErr = NewHTTPError(v2.ErrorCodeBlobUploadInvalid, err, http.StatusBadRequest)
				return
			}
		}
	}
	return uuid, upload, nil
}

// StartLayerUpload begins the layer upload process and allocates a server-
// side upload session.
func StartLayerUpload(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	layers := ctx.Repository.Layers()
	upload, err := layers.Upload()
	if err != nil {
		return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
	}

	defer upload.Close()

	if err := layerUploadResponse(ctx, upload, w); err != nil {
		return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
	}

	w.Header().Set("Docker-Upload-UUID", upload.UUID())
	w.WriteHeader(http.StatusAccepted)
	return nil
}

// GetUploadStatus returns the status of a given upload, identified by uuid.
func GetUploadStatus(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	uuid, upload, httpErr := layerUploadInfo(ctx, r)
	if httpErr != nil {
		return httpErr
	}

	if upload == nil {
		return NewHTTPError(v2.ErrorCodeBlobUploadUnknown, nil, http.StatusNotFound)
	}

	if err := layerUploadResponse(ctx, upload, w); err != nil {
		return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
	}

	w.Header().Set("Docker-Upload-UUID", uuid)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// PutLayerUploadComplete takes the final request of a layer upload. The final
// chunk may include all the layer data, the final chunk of layer data or no
// layer data. Any data provided is received and verified. If successful, the
// layer is linked into the blob store and 201 Created is returned with the
// canonical url of the layer.
func PutLayerUploadComplete(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	_, upload, httpErr := layerUploadInfo(ctx, r)
	if httpErr != nil {
		return httpErr
	}

	if upload == nil {
		return NewHTTPError(v2.ErrorCodeBlobUploadUnknown, nil, http.StatusNotFound)
	}

	dgstStr := r.FormValue("digest") // TODO(stevvooe): Support multiple digest parameters!

	if dgstStr == "" {
		// no digest? return error, but allow retry.
		return NewHTTPError(v2.ErrorCodeDigestInvalid, "digest missing", http.StatusBadRequest)
	}

	dgst, err := digest.ParseDigest(dgstStr)
	if err != nil {
		// no digest? return error, but allow retry.
		return NewHTTPError(v2.ErrorCodeDigestInvalid, "digest parsing failed", http.StatusNotFound)
	}

	// TODO(stevvooe): Check the incoming range header here, per the
	// specification. LayerUpload should be seeked (sought?) to that position.

	// TODO(stevvooe): Consider checking the error on this copy.
	// Theoretically, problems should be detected during verification but we
	// may miss a root cause.

	// Read in the final chunk, if any.
	io.Copy(upload, r.Body)

	layer, err := upload.Finish(dgst)
	if err != nil {
		var httpErr error

		switch err := err.(type) {
		case distribution.ErrLayerInvalidDigest:
			httpErr = NewHTTPError(v2.ErrorCodeDigestInvalid, err, http.StatusBadRequest)
		default:
			ctxu.GetLogger(ctx).Errorf("unknown error completing upload: %#v", err)
			httpErr = NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
		}

		// Clean up the backend layer data if there was an error.
		if err := upload.Cancel(); err != nil {
			// If the cleanup fails, all we can do is observe and report.
			ctxu.GetLogger(ctx).Errorf("error canceling upload after error: %v", err)
		}

		return httpErr
	}

	// Build our canonical layer url
	layerURL, err := ctx.urlBuilder.BuildBlobURL(ctx.Repository.Name(), layer.Digest())
	if err != nil {
		return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
	}

	w.Header().Set("Location", layerURL)
	w.Header().Set("Content-Length", "0")
	w.Header().Set("Docker-Content-Digest", layer.Digest().String())
	w.WriteHeader(http.StatusCreated)
	return nil
}

// CancelLayerUpload cancels an in-progress upload of a layer.
func CancelLayerUpload(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	uuid, upload, httpErr := layerUploadInfo(ctx, r)
	if httpErr != nil {
		return httpErr
	}

	if upload == nil {
		return NewHTTPError(v2.ErrorCodeBlobUploadUnknown, nil, http.StatusNotFound)
	}

	w.Header().Set("Docker-Upload-UUID", uuid)
	if err := upload.Cancel(); err != nil {
		ctxu.GetLogger(ctx).Errorf("error encountered canceling upload: %v", err)
		return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

// layerUploadResponse provides a standard request for uploading layers and
// chunk responses. This sets the correct headers but the response status is
// left to the caller.
func layerUploadResponse(ctx *Context, upload distribution.LayerUpload, w http.ResponseWriter) error {

	offset, err := upload.Seek(0, os.SEEK_CUR)
	if err != nil {
		ctxu.GetLogger(ctx).Errorf("unable get current offset of layer upload: %v", err)
		return err
	}

	state := layerUploadState{
		Name:      ctx.Repository.Name(),
		UUID:      upload.UUID(),
		Offset:    offset,
		StartedAt: upload.StartedAt(),
	}

	token, err := hmacKey(ctx.Secret).packUploadState(state)
	if err != nil {
		ctxu.GetLogger(ctx).Infof("error building upload state token: %s", err)
		return err
	}

	uploadURL, err := ctx.urlBuilder.BuildBlobUploadChunkURL(
		ctx.Repository.Name(), upload.UUID(),
		url.Values{
			"_state": []string{token},
		})
	if err != nil {
		ctxu.GetLogger(ctx).Infof("error building upload url: %s", err)
		return err
	}

	w.Header().Set("Docker-Upload-UUID", upload.UUID())
	w.Header().Set("Location", uploadURL)
	w.Header().Set("Content-Length", "0")
	w.Header().Set("Range", fmt.Sprintf("0-%d", state.Offset))

	return nil
}

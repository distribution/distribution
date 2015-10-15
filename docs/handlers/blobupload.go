package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/gorilla/handlers"
)

// blobUploadDispatcher constructs and returns the blob upload handler for the
// given request context.
func blobUploadDispatcher(ctx *Context, r *http.Request) http.Handler {
	buh := &blobUploadHandler{
		Context: ctx,
		UUID:    getUploadUUID(ctx),
	}

	handler := handlers.MethodHandler{
		"GET":  http.HandlerFunc(buh.GetUploadStatus),
		"HEAD": http.HandlerFunc(buh.GetUploadStatus),
	}

	if !ctx.readOnly {
		handler["POST"] = http.HandlerFunc(buh.StartBlobUpload)
		handler["PATCH"] = http.HandlerFunc(buh.PatchBlobData)
		handler["PUT"] = http.HandlerFunc(buh.PutBlobUploadComplete)
		handler["DELETE"] = http.HandlerFunc(buh.CancelBlobUpload)
	}

	if buh.UUID != "" {
		state, err := hmacKey(ctx.Config.HTTP.Secret).unpackUploadState(r.FormValue("_state"))
		if err != nil {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctxu.GetLogger(ctx).Infof("error resolving upload: %v", err)
				buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadInvalid.WithDetail(err))
			})
		}
		buh.State = state

		if state.Name != ctx.Repository.Name() {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctxu.GetLogger(ctx).Infof("mismatched repository name in upload state: %q != %q", state.Name, buh.Repository.Name())
				buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadInvalid.WithDetail(err))
			})
		}

		if state.UUID != buh.UUID {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctxu.GetLogger(ctx).Infof("mismatched uuid in upload state: %q != %q", state.UUID, buh.UUID)
				buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadInvalid.WithDetail(err))
			})
		}

		blobs := ctx.Repository.Blobs(buh)
		upload, err := blobs.Resume(buh, buh.UUID)
		if err != nil {
			ctxu.GetLogger(ctx).Errorf("error resolving upload: %v", err)
			if err == distribution.ErrBlobUploadUnknown {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadUnknown.WithDetail(err))
				})
			}

			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			})
		}
		buh.Upload = upload

		if state.Offset > 0 {
			// Seek the blob upload to the correct spot if it's non-zero.
			// These error conditions should be rare and demonstrate really
			// problems. We basically cancel the upload and tell the client to
			// start over.
			if nn, err := upload.Seek(buh.State.Offset, os.SEEK_SET); err != nil {
				defer upload.Close()
				ctxu.GetLogger(ctx).Infof("error seeking blob upload: %v", err)
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadInvalid.WithDetail(err))
					upload.Cancel(buh)
				})
			} else if nn != buh.State.Offset {
				defer upload.Close()
				ctxu.GetLogger(ctx).Infof("seek to wrong offest: %d != %d", nn, buh.State.Offset)
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadInvalid.WithDetail(err))
					upload.Cancel(buh)
				})
			}
		}

		return closeResources(handler, buh.Upload)
	}

	return handler
}

// blobUploadHandler handles the http blob upload process.
type blobUploadHandler struct {
	*Context

	// UUID identifies the upload instance for the current request. Using UUID
	// to key blob writers since this implementation uses UUIDs.
	UUID string

	Upload distribution.BlobWriter

	State blobUploadState
}

// StartBlobUpload begins the blob upload process and allocates a server-side
// blob writer session.
func (buh *blobUploadHandler) StartBlobUpload(w http.ResponseWriter, r *http.Request) {
	blobs := buh.Repository.Blobs(buh)
	upload, err := blobs.Create(buh)

	if err != nil {
		if err == distribution.ErrUnsupported {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeUnsupported)
		} else {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	buh.Upload = upload
	defer buh.Upload.Close()

	if err := buh.blobUploadResponse(w, r, true); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	w.Header().Set("Docker-Upload-UUID", buh.Upload.ID())
	w.WriteHeader(http.StatusAccepted)
}

// GetUploadStatus returns the status of a given upload, identified by id.
func (buh *blobUploadHandler) GetUploadStatus(w http.ResponseWriter, r *http.Request) {
	if buh.Upload == nil {
		buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadUnknown)
		return
	}

	// TODO(dmcgowan): Set last argument to false in blobUploadResponse when
	// resumable upload is supported. This will enable returning a non-zero
	// range for clients to begin uploading at an offset.
	if err := buh.blobUploadResponse(w, r, true); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	w.Header().Set("Docker-Upload-UUID", buh.UUID)
	w.WriteHeader(http.StatusNoContent)
}

// PatchBlobData writes data to an upload.
func (buh *blobUploadHandler) PatchBlobData(w http.ResponseWriter, r *http.Request) {
	if buh.Upload == nil {
		buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadUnknown)
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct != "" && ct != "application/octet-stream" {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(fmt.Errorf("Bad Content-Type")))
		// TODO(dmcgowan): encode error
		return
	}

	// TODO(dmcgowan): support Content-Range header to seek and write range

	if err := copyFullPayload(w, r, buh.Upload, buh, "blob PATCH", &buh.Errors); err != nil {
		// copyFullPayload reports the error if necessary
		return
	}

	if err := buh.blobUploadResponse(w, r, false); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// PutBlobUploadComplete takes the final request of a blob upload. The
// request may include all the blob data or no blob data. Any data
// provided is received and verified. If successful, the blob is linked
// into the blob store and 201 Created is returned with the canonical
// url of the blob.
func (buh *blobUploadHandler) PutBlobUploadComplete(w http.ResponseWriter, r *http.Request) {
	if buh.Upload == nil {
		buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadUnknown)
		return
	}

	dgstStr := r.FormValue("digest") // TODO(stevvooe): Support multiple digest parameters!

	if dgstStr == "" {
		// no digest? return error, but allow retry.
		buh.Errors = append(buh.Errors, v2.ErrorCodeDigestInvalid.WithDetail("digest missing"))
		return
	}

	dgst, err := digest.ParseDigest(dgstStr)
	if err != nil {
		// no digest? return error, but allow retry.
		buh.Errors = append(buh.Errors, v2.ErrorCodeDigestInvalid.WithDetail("digest parsing failed"))
		return
	}

	if err := copyFullPayload(w, r, buh.Upload, buh, "blob PUT", &buh.Errors); err != nil {
		// copyFullPayload reports the error if necessary
		return
	}

	desc, err := buh.Upload.Commit(buh, distribution.Descriptor{
		Digest: dgst,

		// TODO(stevvooe): This isn't wildly important yet, but we should
		// really set the length and mediatype. For now, we can let the
		// backend take care of this.
	})

	if err != nil {
		switch err := err.(type) {
		case distribution.ErrBlobInvalidDigest:
			buh.Errors = append(buh.Errors, v2.ErrorCodeDigestInvalid.WithDetail(err))
		default:
			switch err {
			case distribution.ErrUnsupported:
				buh.Errors = append(buh.Errors, errcode.ErrorCodeUnsupported)
			case distribution.ErrBlobInvalidLength, distribution.ErrBlobDigestUnsupported:
				buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadInvalid.WithDetail(err))
			default:
				ctxu.GetLogger(buh).Errorf("unknown error completing upload: %#v", err)
				buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			}

		}

		// Clean up the backend blob data if there was an error.
		if err := buh.Upload.Cancel(buh); err != nil {
			// If the cleanup fails, all we can do is observe and report.
			ctxu.GetLogger(buh).Errorf("error canceling upload after error: %v", err)
		}

		return
	}

	// Build our canonical blob url
	blobURL, err := buh.urlBuilder.BuildBlobURL(buh.Repository.Name(), desc.Digest)
	if err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	w.Header().Set("Location", blobURL)
	w.Header().Set("Content-Length", "0")
	w.Header().Set("Docker-Content-Digest", desc.Digest.String())
	w.WriteHeader(http.StatusCreated)
}

// CancelBlobUpload cancels an in-progress upload of a blob.
func (buh *blobUploadHandler) CancelBlobUpload(w http.ResponseWriter, r *http.Request) {
	if buh.Upload == nil {
		buh.Errors = append(buh.Errors, v2.ErrorCodeBlobUploadUnknown)
		return
	}

	w.Header().Set("Docker-Upload-UUID", buh.UUID)
	if err := buh.Upload.Cancel(buh); err != nil {
		ctxu.GetLogger(buh).Errorf("error encountered canceling upload: %v", err)
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
	}

	w.WriteHeader(http.StatusNoContent)
}

// blobUploadResponse provides a standard request for uploading blobs and
// chunk responses. This sets the correct headers but the response status is
// left to the caller. The fresh argument is used to ensure that new blob
// uploads always start at a 0 offset. This allows disabling resumable push by
// always returning a 0 offset on check status.
func (buh *blobUploadHandler) blobUploadResponse(w http.ResponseWriter, r *http.Request, fresh bool) error {

	var offset int64
	if !fresh {
		var err error
		offset, err = buh.Upload.Seek(0, os.SEEK_CUR)
		if err != nil {
			ctxu.GetLogger(buh).Errorf("unable get current offset of blob upload: %v", err)
			return err
		}
	}

	// TODO(stevvooe): Need a better way to manage the upload state automatically.
	buh.State.Name = buh.Repository.Name()
	buh.State.UUID = buh.Upload.ID()
	buh.State.Offset = offset
	buh.State.StartedAt = buh.Upload.StartedAt()

	token, err := hmacKey(buh.Config.HTTP.Secret).packUploadState(buh.State)
	if err != nil {
		ctxu.GetLogger(buh).Infof("error building upload state token: %s", err)
		return err
	}

	uploadURL, err := buh.urlBuilder.BuildBlobUploadChunkURL(
		buh.Repository.Name(), buh.Upload.ID(),
		url.Values{
			"_state": []string{token},
		})
	if err != nil {
		ctxu.GetLogger(buh).Infof("error building upload url: %s", err)
		return err
	}

	endRange := offset
	if endRange > 0 {
		endRange = endRange - 1
	}

	w.Header().Set("Docker-Upload-UUID", buh.UUID)
	w.Header().Set("Location", uploadURL)
	w.Header().Set("Content-Length", "0")
	w.Header().Set("Range", fmt.Sprintf("0-%d", endRange))

	return nil
}

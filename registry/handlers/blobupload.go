package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/reference"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

// blobUploadDispatcher constructs and returns the blob upload handler for the
// given request context.
func blobUploadDispatcher(ctx *Context, r *http.Request) http.Handler {
	buh := &blobUploadHandler{
		Context: ctx,
		UUID:    getUploadUUID(ctx),
	}

	handler := handlers.MethodHandler{
		http.MethodGet:  http.HandlerFunc(buh.GetUploadStatus),
		http.MethodHead: http.HandlerFunc(buh.GetUploadStatus),
	}

	if !ctx.readOnly {
		handler[http.MethodPost] = http.HandlerFunc(buh.StartBlobUpload)
		handler[http.MethodPatch] = http.HandlerFunc(buh.PatchBlobData)
		handler[http.MethodPut] = http.HandlerFunc(buh.PutBlobUploadComplete)
		handler[http.MethodDelete] = http.HandlerFunc(buh.CancelBlobUpload)
	}

	if buh.UUID != "" {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			return handler
		}
		if h := buh.ResumeBlobUpload(ctx, r); h != nil {
			return h
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
// blob writer session, optionally mounting the blob from a separate repository.
func (buh *blobUploadHandler) StartBlobUpload(w http.ResponseWriter, r *http.Request) {
	var options []distribution.BlobCreateOption

	fromRepo := r.FormValue("from")
	mountDigest := r.FormValue("mount")

	if mountDigest != "" && fromRepo != "" {
		opt, err := buh.createBlobMountOption(fromRepo, mountDigest)
		if opt != nil && err == nil {
			options = append(options, opt)
		}
	}

	blobs := buh.Repository.Blobs(buh)
	upload, err := blobs.Create(buh, options...)
	if err != nil {
		if ebm, ok := err.(distribution.ErrBlobMounted); ok {
			if err := buh.writeBlobCreatedHeaders(w, ebm.Descriptor); err != nil {
				buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			}
		} else if err == distribution.ErrUnsupported {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeUnsupported)
		} else {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	buh.Upload = upload

	if err := buh.blobUploadResponse(w, r); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	w.Header().Set("Docker-Upload-UUID", buh.Upload.ID())
	w.WriteHeader(http.StatusAccepted)
}

// GetUploadStatus returns the status of a given upload, identified by id.
func (buh *blobUploadHandler) GetUploadStatus(w http.ResponseWriter, r *http.Request) {
	if buh.Upload == nil {
		blobs := buh.Repository.Blobs(buh)
		upload, err := blobs.Resume(buh, buh.UUID)
		if err != nil {
			if err == distribution.ErrBlobUploadUnknown {
				buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadUnknown.WithDetail(err))
			} else {
				buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			}
			return
		}

		buh.Upload = upload
	}

	if err := buh.blobUploadResponse(w, r); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// PatchBlobData writes data to an upload.
func (buh *blobUploadHandler) PatchBlobData(w http.ResponseWriter, r *http.Request) {
	if buh.Upload == nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadUnknown)
		return
	}

	ct := r.Header.Get("Content-Type")
	if ct != "" && ct != "application/octet-stream" {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(fmt.Errorf("bad Content-Type")))
		// TODO(dmcgowan): encode error
		return
	}

	cr := r.Header.Get("Content-Range")
	cl := r.Header.Get("Content-Length")
	if cr != "" && cl != "" {
		start, end, err := parseContentRange(cr)
		if err != nil {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err.Error()))
			return
		}
		if start > end || start != buh.Upload.Size() {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeRangeInvalid)
			return
		}

		clInt, err := strconv.ParseInt(cl, 10, 64)
		if err != nil {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err.Error()))
			return
		}
		if clInt != (end-start)+1 {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeSizeInvalid)
			return
		}
	}

	if err := copyFullPayload(buh, w, r, buh.Upload, -1, "blob PATCH"); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err.Error()))
		return
	}

	if err := buh.blobUploadResponse(w, r); err != nil {
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
		buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadUnknown)
		return
	}
	defer buh.Upload.Close()

	dgstStr := r.FormValue("digest") // TODO(stevvooe): Support multiple digest parameters!

	if dgstStr == "" {
		// no digest? return error, but allow retry.
		buh.Errors = append(buh.Errors, errcode.ErrorCodeDigestInvalid.WithDetail("digest missing"))
		return
	}

	dgst, err := digest.Parse(dgstStr)
	if err != nil {
		// no digest? return error, but allow retry.
		buh.Errors = append(buh.Errors, errcode.ErrorCodeDigestInvalid.WithDetail("digest parsing failed"))
		return
	}

	if err := copyFullPayload(buh, w, r, buh.Upload, -1, "blob PUT"); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err.Error()))
		return
	}

	desc, err := buh.Upload.Commit(buh, distribution.Descriptor{
		Digest: dgst,

		// TODO(stevvooe): This isn't wildly important yet, but we should
		// really set the mediatype. For now, we can let the backend take care
		// of this.
	})
	if err != nil {
		switch err := err.(type) {
		case distribution.ErrBlobInvalidDigest:
			buh.Errors = append(buh.Errors, errcode.ErrorCodeDigestInvalid.WithDetail(err))
		case errcode.Error:
			buh.Errors = append(buh.Errors, err)
		default:
			switch err {
			case distribution.ErrAccessDenied:
				buh.Errors = append(buh.Errors, errcode.ErrorCodeDenied)
			case distribution.ErrUnsupported:
				buh.Errors = append(buh.Errors, errcode.ErrorCodeUnsupported)
			case distribution.ErrBlobInvalidLength, distribution.ErrBlobDigestUnsupported:
				buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadInvalid.WithDetail(err))
			default:
				dcontext.GetLogger(buh).Errorf("unknown error completing upload: %v", err)
				buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			}

		}

		// Clean up the backend blob data if there was an error.
		if err := buh.Upload.Cancel(buh); err != nil {
			// If the cleanup fails, all we can do is observe and report.
			dcontext.GetLogger(buh).Errorf("error canceling upload after error: %v", err)
		}

		return
	}
	if err := buh.writeBlobCreatedHeaders(w, desc); err != nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

// CancelBlobUpload cancels an in-progress upload of a blob.
func (buh *blobUploadHandler) CancelBlobUpload(w http.ResponseWriter, r *http.Request) {
	if buh.Upload == nil {
		buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadUnknown)
		return
	}
	defer buh.Upload.Close()

	w.Header().Set("Docker-Upload-UUID", buh.UUID)
	if err := buh.Upload.Cancel(buh); err != nil {
		dcontext.GetLogger(buh).Errorf("error encountered canceling upload: %v", err)
		buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
	}

	w.WriteHeader(http.StatusNoContent)
}

func (buh *blobUploadHandler) ResumeBlobUpload(ctx *Context, r *http.Request) http.Handler {
	state, err := hmacKey(ctx.Config.HTTP.Secret).unpackUploadState(r.FormValue("_state"))
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dcontext.GetLogger(ctx).Infof("error resolving upload: %v", err)
			buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadInvalid.WithDetail(err))
		})
	}
	buh.State = state

	if state.Name != ctx.Repository.Named().Name() {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dcontext.GetLogger(ctx).Infof("mismatched repository name in upload state: %q != %q", state.Name, buh.Repository.Named().Name())
			buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadInvalid.WithDetail(err))
		})
	}

	if state.UUID != buh.UUID {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dcontext.GetLogger(ctx).Infof("mismatched uuid in upload state: %q != %q", state.UUID, buh.UUID)
			buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadInvalid.WithDetail(err))
		})
	}

	blobs := ctx.Repository.Blobs(buh)
	upload, err := blobs.Resume(buh, buh.UUID)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("error resolving upload: %v", err)
		if err == distribution.ErrBlobUploadUnknown {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				buh.Errors = append(buh.Errors, errcode.ErrorCodeBlobUploadUnknown.WithDetail(err))
			})
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		})
	}
	buh.Upload = upload

	if size := upload.Size(); size != buh.State.Offset {
		dcontext.GetLogger(ctx).Errorf("upload resumed at wrong offset: %d != %d", size, buh.State.Offset)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buh.Errors = append(buh.Errors, errcode.ErrorCodeRangeInvalid.WithDetail(err))
		})
	}
	return nil
}

// blobUploadResponse provides a standard request for uploading blobs and
// chunk responses. This sets the correct headers but the response status is
// left to the caller.
func (buh *blobUploadHandler) blobUploadResponse(w http.ResponseWriter, r *http.Request) error {
	// TODO(stevvooe): Need a better way to manage the upload state automatically.
	buh.State.Name = buh.Repository.Named().Name()
	buh.State.UUID = buh.Upload.ID()
	buh.Upload.Close()
	buh.State.Offset = buh.Upload.Size()
	buh.State.StartedAt = buh.Upload.StartedAt()

	token, err := hmacKey(buh.Config.HTTP.Secret).packUploadState(buh.State)
	if err != nil {
		dcontext.GetLogger(buh).Infof("error building upload state token: %s", err)
		return err
	}

	uploadURL, err := buh.urlBuilder.BuildBlobUploadChunkURL(
		buh.Repository.Named(), buh.Upload.ID(),
		url.Values{
			"_state": []string{token},
		})
	if err != nil {
		dcontext.GetLogger(buh).Infof("error building upload url: %s", err)
		return err
	}

	endRange := buh.Upload.Size()
	if endRange > 0 {
		endRange = endRange - 1
	}

	w.Header().Set("Docker-Upload-UUID", buh.UUID)
	w.Header().Set("Location", uploadURL)

	w.Header().Set("Content-Length", "0")
	w.Header().Set("Range", fmt.Sprintf("0-%d", endRange))

	return nil
}

// mountBlob attempts to mount a blob from another repository by its digest. If
// successful, the blob is linked into the blob store and 201 Created is
// returned with the canonical url of the blob.
func (buh *blobUploadHandler) createBlobMountOption(fromRepo, mountDigest string) (distribution.BlobCreateOption, error) {
	dgst, err := digest.Parse(mountDigest)
	if err != nil {
		return nil, err
	}

	ref, err := reference.WithName(fromRepo)
	if err != nil {
		return nil, err
	}

	canonical, err := reference.WithDigest(ref, dgst)
	if err != nil {
		return nil, err
	}

	return storage.WithMountFrom(canonical), nil
}

// writeBlobCreatedHeaders writes the standard headers describing a newly
// created blob. A 201 Created is written as well as the canonical URL and
// blob digest.
func (buh *blobUploadHandler) writeBlobCreatedHeaders(w http.ResponseWriter, desc distribution.Descriptor) error {
	ref, err := reference.WithDigest(buh.Repository.Named(), desc.Digest)
	if err != nil {
		return err
	}
	blobURL, err := buh.urlBuilder.BuildBlobURL(ref)
	if err != nil {
		return err
	}

	w.Header().Set("Location", blobURL)
	w.Header().Set("Content-Length", "0")
	w.Header().Set("Docker-Content-Digest", desc.Digest.String())
	w.WriteHeader(http.StatusCreated)
	return nil
}

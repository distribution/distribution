package handlers

import (
	"net/http"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

// blobDispatcher uses the request context to build a blobHandler.
func blobDispatcher(ctx *Context, r *http.Request) http.Handler {
	dgst, err := getDigest(ctx)
	if err != nil {

		if err == errDigestNotAvailable {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx.Errors = append(ctx.Errors, errcode.ErrorCodeDigestInvalid.WithDetail(err))
			})
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx.Errors = append(ctx.Errors, errcode.ErrorCodeDigestInvalid.WithDetail(err))
		})
	}

	blobHandler := &blobHandler{
		Context: ctx,
		Digest:  dgst,
	}

	mhandler := handlers.MethodHandler{
		http.MethodGet:  http.HandlerFunc(blobHandler.GetBlob),
		http.MethodHead: http.HandlerFunc(blobHandler.GetBlob),
	}

	if !ctx.readOnly {
		mhandler[http.MethodDelete] = http.HandlerFunc(blobHandler.DeleteBlob)
	}

	return mhandler
}

// blobHandler serves http blob requests.
type blobHandler struct {
	*Context

	Digest digest.Digest
}

// GetBlob fetches the binary data from backend storage returns it in the
// response.
func (bh *blobHandler) GetBlob(w http.ResponseWriter, r *http.Request) {
	dcontext.GetLogger(bh).Debug("GetBlob")
	blobs := bh.Repository.Blobs(bh)
	desc, err := blobs.Stat(bh, bh.Digest)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			bh.Errors = append(bh.Errors, errcode.ErrorCodeBlobUnknown.WithDetail(bh.Digest))
		} else {
			bh.Errors = append(bh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	if err := blobs.ServeBlob(bh, w, r, desc.Digest); err != nil {
		dcontext.GetLogger(bh).Debugf("unexpected error getting blob HTTP handler: %v", err)
		bh.Errors = append(bh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

// DeleteBlob deletes a layer blob
func (bh *blobHandler) DeleteBlob(w http.ResponseWriter, r *http.Request) {
	dcontext.GetLogger(bh).Debug("DeleteBlob")

	blobs := bh.Repository.Blobs(bh)
	err := blobs.Delete(bh, bh.Digest)
	if err != nil {
		switch err {
		case distribution.ErrUnsupported:
			bh.Errors = append(bh.Errors, errcode.ErrorCodeUnsupported)
			return
		case distribution.ErrBlobUnknown:
			bh.Errors = append(bh.Errors, errcode.ErrorCodeBlobUnknown)
			return
		default:
			bh.Errors = append(bh.Errors, err)
			dcontext.GetLogger(bh).Errorf("Unknown error deleting blob: %s", err.Error())
			return
		}
	}

	w.Header().Set("Content-Length", "0")
	w.WriteHeader(http.StatusAccepted)
}

package handlers

import (
	"net/http"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/api/v2"
)

// layerDispatcher uses the request context to build a layerHandler.
func layerHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (httpErr error) {
	switch r.Method {
	case "GET", "HEAD":
		httpErr = GetLayer(ctx, w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
	return httpErr
}

// GetLayer fetches the binary data from backend storage returns it in the
// response.
func GetLayer(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	ctxu.GetLogger(ctx).Debug("GetImageLayer")
	dgst, err := getDigest(ctx)
	if err != nil {

		if err == errDigestNotAvailable {
			return NewHTTPError(v2.ErrorCodeDigestInvalid, err, http.StatusNotFound)
		}

		return NewHTTPError(v2.ErrorCodeDigestInvalid, err, http.StatusBadRequest)
	}

	layers := ctx.Repository.Layers()
	layer, err := layers.Fetch(dgst)

	if err != nil {
		switch err := err.(type) {
		case distribution.ErrUnknownLayer:
			return NewHTTPError(v2.ErrorCodeBlobUnknown, err, http.StatusNotFound)
		default:
			return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
		}
	}

	handler, err := layer.Handler(r)
	if err != nil {
		ctxu.GetLogger(ctx).Debugf("unexpected error getting layer HTTP handler: %s", err)
		return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
	}

	handler.ServeHTTP(w, r)
	return nil
}

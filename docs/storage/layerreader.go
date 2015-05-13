package storage

import (
	"fmt"
	"net/http"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage/driver"
)

// layerReader implements Layer and provides facilities for reading and
// seeking.
type layerReader struct {
	fileReader

	digest digest.Digest
}

// newLayerReader returns a new layerReader with the digest, path and length,
// eliding round trips to the storage backend.
func newLayerReader(driver driver.StorageDriver, dgst digest.Digest, path string, length int64) (*layerReader, error) {
	fr := &fileReader{
		driver: driver,
		path:   path,
		size:   length,
	}

	return &layerReader{
		fileReader: *fr,
		digest:     dgst,
	}, nil
}

var _ distribution.Layer = &layerReader{}

func (lr *layerReader) Digest() digest.Digest {
	return lr.digest
}

func (lr *layerReader) Length() int64 {
	return lr.size
}

func (lr *layerReader) CreatedAt() time.Time {
	return lr.modtime
}

// Close the layer. Should be called when the resource is no longer needed.
func (lr *layerReader) Close() error {
	return lr.closeWithErr(distribution.ErrLayerClosed)
}

func (lr *layerReader) Handler(r *http.Request) (h http.Handler, err error) {
	var handlerFunc http.HandlerFunc

	redirectURL, err := lr.fileReader.driver.URLFor(lr.ctx, lr.path, map[string]interface{}{"method": r.Method})

	switch err {
	case nil:
		handlerFunc = func(w http.ResponseWriter, r *http.Request) {
			// Redirect to storage URL.
			http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		}
	case driver.ErrUnsupportedMethod:
		handlerFunc = func(w http.ResponseWriter, r *http.Request) {
			// Fallback to serving the content directly.
			http.ServeContent(w, r, lr.digest.String(), lr.CreatedAt(), lr)
		}
	default:
		// Some unexpected error.
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If the registry is serving this content itself, check
		// the If-None-Match header and return 304 on match.  Redirected
		// storage implementations do the same.

		if etagMatch(r, lr.digest.String()) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		setCacheHeaders(w, 86400, lr.digest.String())
		w.Header().Set("Docker-Content-Digest", lr.digest.String())
		handlerFunc.ServeHTTP(w, r)
	}), nil
}

func etagMatch(r *http.Request, etag string) bool {
	for _, headerVal := range r.Header["If-None-Match"] {
		if headerVal == etag {
			return true
		}
	}
	return false
}

func setCacheHeaders(w http.ResponseWriter, cacheAge int, etag string) {
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%d", cacheAge))

}

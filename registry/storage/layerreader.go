package storage

import (
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
		w.Header().Set("Docker-Content-Digest", lr.digest.String())
		handlerFunc.ServeHTTP(w, r)
	}), nil
}

package storage

import (
	"net/http"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
)

// layerReader implements Layer and provides facilities for reading and
// seeking.
type layerReader struct {
	fileReader

	digest digest.Digest
}

var _ distribution.Layer = &layerReader{}

func (lr *layerReader) Path() string {
	return lr.path
}

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

func (lr *layerReader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Content-Digest", lr.digest.String())

	if url, err := lr.fileReader.driver.URLFor(lr.Path(), map[string]interface{}{}); err == nil {
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
	http.ServeContent(w, r, lr.digest.String(), lr.CreatedAt(), lr)
}

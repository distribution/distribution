package storage

import (
	"net/http"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
)

// LayerRead implements Layer and provides facilities for reading and
// seeking.
type layerReader struct {
	fileReader

	digest digest.Digest
}

var _ distribution.Layer = &layerReader{}

func (lrs *layerReader) Path() string {
	return lrs.path
}

func (lrs *layerReader) Digest() digest.Digest {
	return lrs.digest
}

func (lrs *layerReader) Length() int64 {
	return lrs.size
}

func (lrs *layerReader) CreatedAt() time.Time {
	return lrs.modtime
}

// Close the layer. Should be called when the resource is no longer needed.
func (lrs *layerReader) Close() error {
	return lrs.closeWithErr(distribution.ErrLayerClosed)
}

func (lrs *layerReader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Docker-Content-Digest", lrs.digest.String())

	if url, err := lrs.fileReader.driver.URLFor(lrs.Path(), map[string]interface{}{}); err == nil {
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
	http.ServeContent(w, r, lrs.Digest().String(), lrs.CreatedAt(), lrs)
}

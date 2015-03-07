package storage

import (
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
)

// layerReadSeeker implements Layer and provides facilities for reading and
// seeking.
type layerReader struct {
	fileReader

	digest digest.Digest
}

var _ distribution.Layer = &layerReader{}

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

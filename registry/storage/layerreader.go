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

	name   string // repo name of this layer
	digest digest.Digest
}

var _ distribution.Layer = &layerReader{}

func (lrs *layerReader) Name() string {
	return lrs.name
}

func (lrs *layerReader) Digest() digest.Digest {
	return lrs.digest
}

func (lrs *layerReader) CreatedAt() time.Time {
	return lrs.modtime
}

// Close the layer. Should be called when the resource is no longer needed.
func (lrs *layerReader) Close() error {
	return lrs.closeWithErr(distribution.ErrLayerClosed)
}

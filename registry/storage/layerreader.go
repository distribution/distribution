package storage

import (
	"time"

	"github.com/docker/distribution/digest"
)

// layerReadSeeker implements Layer and provides facilities for reading and
// seeking.
type layerReader struct {
	fileReader

	name   string // repo name of this layer
	digest digest.Digest
}

var _ Layer = &layerReader{}

func (lrs *layerReader) Name() string {
	return lrs.name
}

func (lrs *layerReader) Digest() digest.Digest {
	return lrs.digest
}

func (lrs *layerReader) CreatedAt() time.Time {
	return lrs.modtime
}

package storage

import (
	"fmt"
	"io"
	"time"

	"github.com/docker/distribution/digest"
)

// Layer provides a readable and seekable layer object. Typically,
// implementations are *not* goroutine safe.
type Layer interface {
	// http.ServeContent requires an efficient implementation of
	// ReadSeeker.Seek(0, os.SEEK_END).
	io.ReadSeeker
	io.Closer

	// Name returns the repository under which this layer is linked.
	Name() string // TODO(stevvooe): struggling with nomenclature: should this be "repo" or "name"?

	// Digest returns the unique digest of the blob, which is the tarsum for
	// layers.
	Digest() digest.Digest

	// CreatedAt returns the time this layer was created. Until we implement
	// Stat call on storagedriver, this just returns the zero time.
	CreatedAt() time.Time
}

// LayerUpload provides a handle for working with in-progress uploads.
// Instances can be obtained from the LayerService.Upload and
// LayerService.Resume.
type LayerUpload interface {
	io.WriteCloser

	// UUID returns the identifier for this upload.
	UUID() string

	// Name of the repository under which the layer will be linked.
	Name() string

	// Offset returns the position of the last byte written to this layer.
	Offset() int64

	// TODO(stevvooe): Consider completely removing the size check from this
	// interface. The digest check may be adequate and we are making it
	// optional in the HTTP API.

	// Finish marks the upload as completed, returning a valid handle to the
	// uploaded layer. The final size and digest are validated against the
	// contents of the uploaded layer. If the size is negative, only the
	// digest will be checked.
	Finish(size int64, digest digest.Digest) (Layer, error)

	// Cancel the layer upload process.
	Cancel() error
}

var (
	// ErrLayerExists returned when layer already exists
	ErrLayerExists = fmt.Errorf("layer exists")

	// ErrLayerTarSumVersionUnsupported when tarsum is unsupported version.
	ErrLayerTarSumVersionUnsupported = fmt.Errorf("unsupported tarsum version")

	// ErrLayerUploadUnknown returned when upload is not found.
	ErrLayerUploadUnknown = fmt.Errorf("layer upload unknown")

	// ErrLayerClosed returned when an operation is attempted on a closed
	// Layer or LayerUpload.
	ErrLayerClosed = fmt.Errorf("layer closed")
)

// ErrUnknownLayer returned when layer cannot be found.
type ErrUnknownLayer struct {
	FSLayer FSLayer
}

func (err ErrUnknownLayer) Error() string {
	return fmt.Sprintf("unknown layer %v", err.FSLayer.BlobSum)
}

// ErrLayerInvalidDigest returned when tarsum check fails.
type ErrLayerInvalidDigest struct {
	FSLayer FSLayer
}

func (err ErrLayerInvalidDigest) Error() string {
	return fmt.Sprintf("invalid digest for referenced layer: %v", err.FSLayer.BlobSum)
}

// ErrLayerInvalidSize returned when length check fails.
type ErrLayerInvalidSize struct {
	Size int64
}

func (err ErrLayerInvalidSize) Error() string {
	return fmt.Sprintf("invalid layer size: %d", err.Size)
}

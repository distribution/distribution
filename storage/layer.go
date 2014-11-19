package storage

import (
	"fmt"
	"io"
	"time"

	"github.com/docker/docker-registry/digest"
)

// LayerService provides operations on layer files in a backend storage.
type LayerService interface {
	// Exists returns true if the layer exists.
	Exists(name string, digest digest.Digest) (bool, error)

	// Fetch the layer identifed by TarSum.
	Fetch(name string, digest digest.Digest) (Layer, error)

	// Upload begins a layer upload to repository identified by name,
	// returning a handle.
	Upload(name string) (LayerUpload, error)

	// Resume continues an in progress layer upload, returning the current
	// state of the upload.
	Resume(uuid string) (LayerUpload, error)
}

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

	// Finish marks the upload as completed, returning a valid handle to the
	// uploaded layer. The final size and digest are validated against the
	// contents of the uploaded layer.
	Finish(size int64, digest digest.Digest) (Layer, error)

	// Cancel the layer upload process.
	Cancel() error
}

var (
	// ErrLayerUnknown returned when layer cannot be found.
	ErrLayerUnknown = fmt.Errorf("unknown layer")

	// ErrLayerExists returned when layer already exists
	ErrLayerExists = fmt.Errorf("layer exists")

	// ErrLayerTarSumVersionUnsupported when tarsum is unsupported version.
	ErrLayerTarSumVersionUnsupported = fmt.Errorf("unsupported tarsum version")

	// ErrLayerUploadUnknown returned when upload is not found.
	ErrLayerUploadUnknown = fmt.Errorf("layer upload unknown")

	// ErrLayerInvalidDigest returned when tarsum check fails.
	ErrLayerInvalidDigest = fmt.Errorf("invalid layer digest")

	// ErrLayerInvalidLength returned when length check fails.
	ErrLayerInvalidLength = fmt.Errorf("invalid layer length")
)

package distribution

import (
	"io"
	"time"

	"github.com/docker/distribution/digest"
	"golang.org/x/net/context"
)

// Blob describes a readable and seekable binary object. Most registry
// objects, including layers and manifests can be accessible by digest in a
// BlobService.
type Blob interface {
	// NOTE(stevvooe): http.ServeContent requires an efficient implementation
	// of ReadSeeker.Seek(0, os.SEEK_END).
	io.ReadSeeker
	io.ReaderAt
	io.WriterTo
	io.Closer

	// Descriptor identifies the metadata about the blob.
	Descriptor() Descriptor
}

// BlobWriter provides a handle for working with a blob store. Instances
// should be obtained from BlobService.Writer and BlobService.Resume. If
// supported by the store, a writer can be recovered with the id.
type BlobWriter interface {
	io.WriteSeeker
	io.WriterAt
	io.ReaderFrom
	io.Closer

	// ID returns the identifier for this writer. The ID can be used with the
	// Blob service to later resume the write.
	ID() string

	// StartedAt returns the time this blob write was started.
	StartedAt() time.Time

	// Commit completes the blob writer process. The content is verified
	// against the descriptor, which may result in an error. Depending on the
	// implementation, Size or Digest or both, if present, may be validated
	// against written data. If MediaType is present, the implementation may
	// reject the commit or assign "application/octet- stream" to the blob.
	Commit(desc Descriptor) (Blob, error)

	// Rollback cancels the blob write and frees any associated resources. Any
	// data written thus far will be lost.
	Rollback() error
}

// BlobService describes operations for getting and writing blob data.
type BlobService interface {
	// Exists returns true if the blob exists.
	Exists(ctx context.Context, dgst digest.Digest) (bool, error)

	// Get the blob identifed by digest dgst.
	Get(ctx context.Context, dgst digest.Digest) (Blob, error)

	// Writer creates a new blob writer to add a blob to this service.
	Writer(ctx context.Context) (BlobWriter, error)

	// Resume attempts to resume a write to a blob, identified by a id.
	Resume(ctx context.Context, id string) (BlobWriter, error)
}

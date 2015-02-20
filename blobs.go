package distribution

import (
	"io"
	"net/http"
	"time"

	"github.com/docker/distribution/digest"
	"golang.org/x/net/context"
)

// Blob describes a readable and seekable binary object. Most registry
// objects, including layers and manifests can be accessible by digest in a
// BlobService.
type Blob interface {
	Describable
	io.ReaderAt
	io.Closer

	Reader() (ReadSeekCloser, error)

	// Handler returns an HTTP handler which serves the blob content, either
	// by redirection or directly serving the content itself.
	Handler(r *http.Request) (http.Handler, error)
}

// BlobWriter provides a handle for working with a blob store. Instances
// should be obtained from BlobService.Writer and BlobService.Resume. If
// supported by the store, a writer can be recovered with the id.
type BlobWriter interface {
	io.WriteSeeker
	io.WriterAt
	io.ReaderFrom // TODO(stevvooe): This should be optional.
	io.Closer

	// ID returns the identifier for this writer. The ID can be used with the
	// Blob service to later resume the write.
	ID() string

	// StartedAt returns the time this blob write was started.
	StartedAt() time.Time

	// Commit completes the blob writer process. The content is verified
	// against the descriptor, which may result in an error. Depending on the
	// implementation, Size or Digest or both, if present, may be validated
	// against written data. If MediaType is not present, the implementation
	// may reject the commit or assign "application/octet-stream" to the blob.
	Commit(ctx context.Context, desc Descriptor) (Blob, error)

	// Rollback cancels the blob write and frees any associated resources. Any
	// data written thus far will be lost. Rollback implementation should
	// allow multiple calls even after a commit that result in a no-op. This
	// allows use of Rollback in a defer statement, increasing the assurance
	// that it is correctly called.
	Rollback(ctx context.Context) error
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

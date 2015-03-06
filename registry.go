package distribution

import (
	"io"
	"time"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"golang.org/x/net/context"
)

// Registry represents a collection of repositories, addressable by name.
type Registry interface {
	// Repository should return a reference to the named repository. The
	// registry may or may not have the repository but should always return a
	// reference.
	Repository(ctx context.Context, name string) (Repository, error)
}

// Repository is a named collection of manifests and layers.
type Repository interface {
	// Name returns the name of the repository.
	Name() string

	// Manifests returns a reference to this repository's manifest service.
	Manifests() ManifestService

	// Layers returns a reference to this repository's layers service.
	Layers() LayerService

	// Signatures returns a reference to this repository's signatures service.
	Signatures() SignatureService
}

// ManifestService provides operations on image manifests.
type ManifestService interface {
	// Exists returns true if the manifest exists.
	Exists(dgst digest.Digest) (bool, error)

	// Get retrieves the identified by the digest, if it exists.
	Get(dgst digest.Digest) (*manifest.SignedManifest, error)

	// Delete removes the manifest, if it exists.
	Delete(dgst digest.Digest) error

	// Put creates or updates the manifest.
	Put(manifest *manifest.SignedManifest) error

	// TODO(stevvooe): The methods after this message should be moved to a
	// discrete TagService, per active proposals.

	// Tags lists the tags under the named repository.
	Tags() ([]string, error)

	// ExistsByTag returns true if the manifest exists.
	ExistsByTag(tag string) (bool, error)

	// GetByTag retrieves the named manifest, if it exists.
	GetByTag(tag string) (*manifest.SignedManifest, error)

	// TODO(stevvooe): There are several changes that need to be done to this
	// interface:
	//
	//	1. Allow explicit tagging with Tag(digest digest.Digest, tag string)
	//	2. Support reading tags with a re-entrant reader to avoid large
	//       allocations in the registry.
	//	3. Long-term: Provide All() method that lets one scroll through all of
	//       the manifest entries.
	//	4. Long-term: break out concept of signing from manifests. This is
	//       really a part of the distribution sprint.
	//	5. Long-term: Manifest should be an interface. This code shouldn't
	//       really be concerned with the storage format.
}

// LayerService provides operations on layer files in a backend storage.
type LayerService interface {
	// Exists returns true if the layer exists.
	Exists(digest digest.Digest) (bool, error)

	// Fetch the layer identifed by TarSum.
	Fetch(digest digest.Digest) (Layer, error)

	// Upload begins a layer upload to repository identified by name,
	// returning a handle.
	Upload() (LayerUpload, error)

	// Resume continues an in progress layer upload, returning a handle to the
	// upload. The caller should seek to the latest desired upload location
	// before proceeding.
	Resume(uuid string) (LayerUpload, error)
}

// Layer provides a readable and seekable layer object. Typically,
// implementations are *not* goroutine safe.
type Layer interface {
	// http.ServeContent requires an efficient implementation of
	// ReadSeeker.Seek(0, os.SEEK_END).
	io.ReadSeeker
	io.Closer

	// Digest returns the unique digest of the blob, which is the tarsum for
	// layers.
	Digest() digest.Digest

	// CreatedAt returns the time this layer was created.
	CreatedAt() time.Time
}

// LayerUpload provides a handle for working with in-progress uploads.
// Instances can be obtained from the LayerService.Upload and
// LayerService.Resume.
type LayerUpload interface {
	io.WriteSeeker
	io.ReaderFrom
	io.Closer

	// UUID returns the identifier for this upload.
	UUID() string

	// StartedAt returns the time this layer upload was started.
	StartedAt() time.Time

	// Finish marks the upload as completed, returning a valid handle to the
	// uploaded layer. The digest is validated against the contents of the
	// uploaded layer.
	Finish(digest digest.Digest) (Layer, error)

	// Cancel the layer upload process.
	Cancel() error
}

// SignatureService provides operations on signatures.
type SignatureService interface {
	// Get retrieves all of the signature blobs for the specified digest.
	Get(dgst digest.Digest) ([][]byte, error)

	// Put stores the signature for the provided digest.
	Put(dgst digest.Digest, signatures ...[]byte) error
}

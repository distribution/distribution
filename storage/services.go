package storage

import (
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"golang.org/x/net/context"
)

// TODO(stevvooe): These types need to be moved out of the storage package.

// Registry represents a collection of repositories, addressable by name.
type Registry interface {
	// Repository should return a reference to the named repository. The
	// registry may or may not have the repository but should always return a
	// reference.
	Repository(ctx context.Context, name string) Repository
}

// Repository is a named collection of manifests and layers.
type Repository interface {
	// Name returns the name of the repository.
	Name() string

	// Manifests returns a reference to this repository's manifest service.
	Manifests() ManifestService

	// Layers returns a reference to this repository's layers service.
	Layers() LayerService
}

// ManifestService provides operations on image manifests.
type ManifestService interface {
	// Tags lists the tags under the named repository.
	Tags() ([]string, error)

	// Exists returns true if the manifest exists.
	Exists(tag string) (bool, error)

	// Get retrieves the named manifest, if it exists.
	Get(tag string) (*manifest.SignedManifest, error)

	// Put creates or updates the named manifest.
	// Put(tag string, manifest *manifest.SignedManifest) (digest.Digest, error)
	Put(tag string, manifest *manifest.SignedManifest) error

	// Delete removes the named manifest, if it exists.
	Delete(tag string) error

	// TODO(stevvooe): There are several changes that need to be done to this
	// interface:
	//
	//	1. Get(tag string) should be GetByTag(tag string)
	//	2. Put(tag string, manifest *manifest.SignedManifest) should be
	//       Put(manifest *manifest.SignedManifest). The method can read the
	//       tag on manifest to automatically tag it in the repository.
	//	3. Need a GetByDigest(dgst digest.Digest) method.
	//	4. Allow explicit tagging with Tag(digest digest.Digest, tag string)
	//	5. Support reading tags with a re-entrant reader to avoid large
	//       allocations in the registry.
	//	6. Long-term: Provide All() method that lets one scroll through all of
	//       the manifest entries.
	//	7. Long-term: break out concept of signing from manifests. This is
	//       really a part of the distribution sprint.
	//	8. Long-term: Manifest should be an interface. This code shouldn't
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

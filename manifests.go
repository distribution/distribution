package distribution

import (
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"
)

// Manifest represents a registry object specifying a set of
// references and an optional target
type Manifest interface {
	// Target returns a descriptor for the configuration of this manifest.  This
	// may return a nil descriptor if a target is not applicable for the given manifest.
	Target() Descriptor

	// References returns a list of objects which make up this manifest.
	// The references are strictly ordered from base to head. A reference
	// is anything which can be represented by a distribution.Descriptor
	References() []Descriptor

	// Payload provides the serialized format of the manifest, in addition to
	// the mediatype.
	Payload() (mediatype string, payload []byte, err error)
}

// ManifestBuilder creates a manifest allowing one to include dependencies.
// Instances can be obtained from a version-specific manifest package.  Manifest
// specific data is passed into the function which creates the builder.
type ManifestBuilder interface {
	// Build creates the manifest from his builder.
	Build() (Manifest, error)

	// References returns a list of objects which have been added to this
	// builder. The dependencies are returned in the order they were added,
	// which should be from base to head.
	References() []Descriptor

	// AppendReference includes the given object in the manifest after any
	// existing dependencies. If the add fails, such as when adding an
	// unsupported dependency, an error may be returned.
	AppendReference(dependency Descriptor) error
}

// ManifestService describes operations on image manifests.
type ManifestService interface {
	// Exists returns true if the manifest exists.
	Exists(ctx context.Context, dgst digest.Digest) (bool, error)

	// Get retrieves the manifest specified by the given digest
	Get(ctx context.Context, dgst digest.Digest) (Manifest, error)

	// Put creates or updates the given manifest returning the manifest digest
	Put(ctx context.Context, manifest Manifest) (digest.Digest, error)

	// Delete removes the manifest specified by the given digest. Deleting
	// a manifest that doesn't exist will return ErrManifestNotFound
	Delete(ctx context.Context, dgst digest.Digest) error

	// Enumerate fills 'manifests' with the manifests in this service up
	// to the size of 'manifests' and returns 'n' for the number of entries
	// which were filled.  'last' contains an offset in the manifest set
	// and can be used to resume iteration.
	Enumerate(ctx context.Context, manifests []Manifest, last Manifest) (n int, err error)
}

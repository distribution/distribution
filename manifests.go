package distribution

import (
	"github.com/docker/distribution/digest"
	"golang.org/x/net/context"
)

// Manifest specifies a registry object with a list of dependencies.
type Manifest interface {
	Blob // io access to serialized content.

	// Dependencies returns a list of object on which this manifest depends.
	// The dependencies are strictly ordered from base to head. Typically,
	// these are layers but their interpretation is application specific.
	Dependencies() []Descriptor
}

// ManifestBuilder creates a manifest allowing one to include dependencies.
// Instances can be obtained from a version-specific manifest package, likely
// from a type that provides a version specific interface.
type ManifestBuilder interface {
	// Build creates the manifest fromt his builder.
	Build() (Manifest, error)

	// Dependencies returns a list of objects which have been added to this
	// builder. The dependencies are returned in the order they were added,
	// which should be from base to head.
	Dependencies() []Descriptor

	// AddDependency includes the dependency in the manifest after any
	// existing dependencies. If the add fails, such as when adding an
	// unsupported dependency, an error may be returned.
	AddDependency(dependency Describable) error
}

// ManifestService describes operations on image manifests.
type ManifestService interface {
	// Exists returns true if the manifest exists.
	Exists(ctx context.Context, dgst digest.Digest) (bool, error)

	// Get retrieves the named manifest, if it exists.
	Get(ctx context.Context, dgst digest.Digest) (Manifest, error)

	// Put creates or updates the named manifest.
	Put(ctx context.Context, manifest Manifest) (digest.Digest, error)

	// Delete removes the named manifest, if it exists.
	Delete(ctx context.Context, dgst digest.Digest) error

	// TODO(stevvooe): Provide All() method that lets one scroll through all
	// of the manifest entries.
}

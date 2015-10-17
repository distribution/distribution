package distribution

import (
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"
)

// Manifest represents a registry object specifying a target and a set of constituents
type Manifest interface {
	// Target returns a descriptor for the configuration of this manifest.  This
	// may return a nil descriptor if none exists for this manifest.
	Target() Descriptor

	// Constituents returns a list of objects which make up this manifest.
	// The dependencies are strictly ordered from base to head. A constituent
	// is anything which can be represented by a distribution.Descriptor
	Constituents() []Descriptor

	// Payload provides the serialized format of the manifest, in addition to
	// the mediatype. // TODO(richardscothern): make mediatype its
	// own function?
	Payload() (mediatype string, payload []byte, err error)

	// Reference returns the tag for this manifest.  This is for convenience
	// and is not necessarily part of the manifest schema
	Tag() reference.Tagged
}

// ManifestBuilder creates a manifest allowing one to include dependencies.
// Instances can be obtained from a version-specific manifest package.  Manifest
// specific data is passed into the function which creates the builder.
type ManifestBuilder interface {
	// Build creates the manifest from his builder.
	Build() (Manifest, error)

	// Constituents returns a list of objects which have been added to this
	// builder. The dependencies are returned in the order they were added,
	// which should be from base to head.
	Constituents() []Descriptor

	// AddConstituent includes the given object in the manifest after any
	// existing dependencies. If the add fails, such as when adding an
	// unsupported dependency, an error may be returned.  Constituent isn't
	// a great name but correctly describes history/layer pairs (schema v1),
	// Manifests (schema v2 manifest list) and layers (schema v2 image manifest)
	AddConstituent(dependency Descriptor) error
}

type OnManifestFunc func(Manifest)

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

	// Foreach allows iterating through all Manifests in the service
	Foreach(ctx context.Context, f OnManifestFunc) error
}

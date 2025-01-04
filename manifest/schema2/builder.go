package schema2

import (
	"context"

	"github.com/distribution/distribution/v3"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Builder is a type for constructing manifests.
type Builder struct {
	// configDescriptor is used to describe configuration
	configDescriptor v1.Descriptor

	// configJSON references
	configJSON []byte

	// dependencies is a list of descriptors that gets built by successive
	// calls to AppendReference. In case of image configuration these are layers.
	dependencies []v1.Descriptor
}

// NewManifestBuilder is used to build new manifests for the current schema
// version. It takes a BlobService so it can publish the configuration blob
// as part of the Build process.
func NewManifestBuilder(configDescriptor v1.Descriptor, configJSON []byte) *Builder {
	mb := &Builder{
		configDescriptor: configDescriptor,
		configJSON:       make([]byte, len(configJSON)),
	}
	copy(mb.configJSON, configJSON)

	return mb
}

// Build produces a final manifest from the given references.
func (mb *Builder) Build(ctx context.Context) (distribution.Manifest, error) {
	m := Manifest{
		Versioned: specs.Versioned{SchemaVersion: defaultSchemaVersion},
		MediaType: defaultMediaType,
		Layers:    make([]v1.Descriptor, len(mb.dependencies)),
	}
	copy(m.Layers, mb.dependencies)

	m.Config = mb.configDescriptor

	return FromStruct(m)
}

// AppendReference adds a reference to the current ManifestBuilder.
func (mb *Builder) AppendReference(ref v1.Descriptor) error {
	mb.dependencies = append(mb.dependencies, ref)
	return nil
}

// References returns the current references added to this builder.
func (mb *Builder) References() []v1.Descriptor {
	return mb.dependencies
}

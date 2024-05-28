package schema2

import (
	"context"

	"github.com/distribution/distribution/v3"
)

// builder is a type for constructing manifests.
type builder struct {
	// configDescriptor is used to describe configuration
	configDescriptor distribution.Descriptor

	// configJSON references
	configJSON []byte

	// dependencies is a list of descriptors that gets built by successive
	// calls to AppendReference. In case of image configuration these are layers.
	dependencies []distribution.Descriptor
}

// NewManifestBuilder is used to build new manifests for the current schema
// version. It takes a BlobService so it can publish the configuration blob
// as part of the Build process.
func NewManifestBuilder(configDescriptor distribution.Descriptor, configJSON []byte) distribution.ManifestBuilder {
	mb := &builder{
		configDescriptor: configDescriptor,
		configJSON:       make([]byte, len(configJSON)),
	}
	copy(mb.configJSON, configJSON)

	return mb
}

// Build produces a final manifest from the given references.
func (mb *builder) Build(ctx context.Context) (distribution.Manifest, error) {
	m := Manifest{
		Versioned: SchemaVersion,
		Layers:    make([]distribution.Descriptor, len(mb.dependencies)),
	}
	copy(m.Layers, mb.dependencies)

	m.Config = mb.configDescriptor

	return FromStruct(m)
}

// AppendReference adds a reference to the current ManifestBuilder.
func (mb *builder) AppendReference(d distribution.Describable) error {
	mb.dependencies = append(mb.dependencies, d.Descriptor())
	return nil
}

// References returns the current references added to this builder.
func (mb *builder) References() []distribution.Descriptor {
	return mb.dependencies
}

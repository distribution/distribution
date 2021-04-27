package schema2

import (
	"context"
	"errors"

	"github.com/distribution/distribution/v3"
	"github.com/opencontainers/go-digest"
)

// builder is a type for constructing manifests.
type builder struct {
	// bs is a BlobService used to publish the configuration blob.
	bs distribution.BlobService

	// configMediaType is media type used to describe configuration
	configMediaType string

	// configJSON references
	configJSON []byte

	// layers is a list of descriptors that gets built by successive
	// calls to AppendReference. In case of image configuration these are layers.
	layers []distribution.Descriptor
}

// NewManifestBuilder is used to build new manifests for the current schema
// version. It takes a BlobService so it can publish the configuration blob
// as part of the Build process.
func NewManifestBuilder(bs distribution.BlobService, configMediaType string, configJSON []byte) distribution.ManifestBuilder {
	mb := &builder{
		bs:              bs,
		configMediaType: configMediaType,
		configJSON:      make([]byte, len(configJSON)),
	}
	copy(mb.configJSON, configJSON)

	return mb
}

// Build produces a final manifest from the given references.
func (mb *builder) Build(ctx context.Context) (distribution.Manifest, error) {
	m := Manifest{
		Versioned: SchemaVersion,
		Layers:    make([]distribution.Descriptor, len(mb.layers)),
	}
	copy(m.Layers, mb.layers)

	configDigest := digest.FromBytes(mb.configJSON)

	var err error
	m.Config, err = mb.bs.Stat(ctx, configDigest)
	switch err {
	case nil:
		// Override MediaType, since Put always replaces the specified media
		// type with application/octet-stream in the descriptor it returns.
		m.Config.MediaType = mb.configMediaType
		return FromStruct(m)
	case distribution.ErrBlobUnknown:
		// nop
	default:
		return nil, err
	}

	// Add config to the blob store
	m.Config, err = mb.bs.Put(ctx, mb.configMediaType, mb.configJSON)
	// Override MediaType, since Put always replaces the specified media
	// type with application/octet-stream in the descriptor it returns.
	m.Config.MediaType = mb.configMediaType
	if err != nil {
		return nil, err
	}

	return FromStruct(m)
}

func (mb *builder) AppendReference(d distribution.Describable) error {
	return mb.AppendBlobReference(d)
}

// AppendReference adds a reference to the current ManifestBuilder.
func (mb *builder) AppendBlobReference(d distribution.Describable) error {
	mb.layers = append(mb.layers, d.Descriptor())
	return nil
}

// AppendManifestReference adds a reference to the current ManifestBuilder
func (mb *builder) AppendManifestReference(d distribution.Describable) error {
	return errors.New("cannot add manifest reference to schema2 manifest")
}

// References returns the current references added to this builder.
func (mb *builder) References() []distribution.Descriptor {
	return mb.BlobReferences()
}

// BlobReferences returns the current blob references added to this builder.
func (mb *builder) BlobReferences() []distribution.Descriptor {
	return mb.layers
}

// ManifestReferences returns the current manifest references added to this builder.
func (mb *builder) ManifestReferences() []distribution.Descriptor {
	return nil
}

package schema2

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// MediaTypeManifest specifies the mediaType for the current version.
	MediaTypeManifest = "application/vnd.docker.distribution.manifest.v2+json"

	// MediaTypeImageConfig specifies the mediaType for the image configuration.
	MediaTypeImageConfig = "application/vnd.docker.container.image.v1+json"

	// MediaTypePluginConfig specifies the mediaType for plugin configuration.
	MediaTypePluginConfig = "application/vnd.docker.plugin.v1+json"

	// MediaTypeLayer is the mediaType used for layers referenced by the
	// manifest.
	MediaTypeLayer = "application/vnd.docker.image.rootfs.diff.tar.gzip"

	// MediaTypeForeignLayer is the mediaType used for layers that must be
	// downloaded from foreign URLs.
	MediaTypeForeignLayer = "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip"

	// MediaTypeUncompressedLayer is the mediaType used for layers which
	// are not compressed.
	MediaTypeUncompressedLayer = "application/vnd.docker.image.rootfs.diff.tar"
)

const (
	defaultSchemaVersion = 2
	defaultMediaType     = MediaTypeManifest
)

// SchemaVersion provides a pre-initialized version structure for this
// packages version of the manifest.
//
// Deprecated: use [specs.Versioned] and set MediaType on the manifest
// to [MediaTypeManifest].
//
//nolint:staticcheck // ignore SA1019: manifest.Versioned is deprecated:
var SchemaVersion = manifest.Versioned{
	SchemaVersion: defaultSchemaVersion,
	MediaType:     defaultMediaType,
}

func init() {
	if err := distribution.RegisterManifestSchema(defaultMediaType, unmarshalSchema2); err != nil {
		panic(fmt.Sprintf("Unable to register manifest: %s", err))
	}
}

func unmarshalSchema2(b []byte) (distribution.Manifest, v1.Descriptor, error) {
	m := &DeserializedManifest{}
	if err := m.UnmarshalJSON(b); err != nil {
		return nil, v1.Descriptor{}, err
	}

	return m, v1.Descriptor{
		Digest:    digest.FromBytes(b),
		Size:      int64(len(b)),
		MediaType: defaultMediaType,
	}, nil
}

// Manifest defines a schema2 manifest.
type Manifest struct {
	specs.Versioned

	// MediaType is the media type of this schema.
	MediaType string `json:"mediaType,omitempty"`

	// Config references the image configuration as a blob.
	Config v1.Descriptor `json:"config"`

	// Layers lists descriptors for the layers referenced by the
	// configuration.
	Layers []v1.Descriptor `json:"layers"`
}

// References returns the descriptors of this manifests references.
func (m Manifest) References() []v1.Descriptor {
	references := make([]v1.Descriptor, 0, 1+len(m.Layers))
	references = append(references, m.Config)
	references = append(references, m.Layers...)
	return references
}

// Target returns the target of this manifest.
func (m Manifest) Target() v1.Descriptor {
	return m.Config
}

// DeserializedManifest wraps Manifest with a copy of the original JSON.
// It satisfies the distribution.Manifest interface.
type DeserializedManifest struct {
	Manifest

	// canonical is the canonical byte representation of the Manifest.
	canonical []byte
}

// FromStruct takes a Manifest structure, marshals it to JSON, and returns a
// DeserializedManifest which contains the manifest and its JSON representation.
func FromStruct(m Manifest) (*DeserializedManifest, error) {
	var deserialized DeserializedManifest
	deserialized.Manifest = m

	var err error
	deserialized.canonical, err = json.MarshalIndent(&m, "", "   ")
	return &deserialized, err
}

// UnmarshalJSON populates a new Manifest struct from JSON data.
func (m *DeserializedManifest) UnmarshalJSON(b []byte) error {
	m.canonical = make([]byte, len(b))
	// store manifest in canonical
	copy(m.canonical, b)

	// Unmarshal canonical JSON into Manifest object
	var mfst Manifest
	if err := json.Unmarshal(m.canonical, &mfst); err != nil {
		return err
	}

	if mfst.MediaType != defaultMediaType {
		return fmt.Errorf("mediaType in manifest should be '%s' not '%s'", defaultMediaType, mfst.MediaType)
	}

	m.Manifest = mfst

	return nil
}

// MarshalJSON returns the contents of canonical. If canonical is empty,
// marshals the inner contents.
func (m *DeserializedManifest) MarshalJSON() ([]byte, error) {
	if len(m.canonical) > 0 {
		return m.canonical, nil
	}

	return nil, errors.New("JSON representation not initialized in DeserializedManifest")
}

// Payload returns the raw content of the manifest. The contents can be used to
// calculate the content identifier.
func (m DeserializedManifest) Payload() (string, []byte, error) {
	return m.MediaType, m.canonical, nil
}

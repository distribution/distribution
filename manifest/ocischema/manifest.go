package ocischema

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

// SchemaVersion provides a pre-initialized version structure for OCI Image
// Manifests.
//
// Deprecated: use [specs.Versioned] and set MediaType on the manifest
// to [v1.MediaTypeImageManifest].
//
//nolint:staticcheck // ignore SA1019: manifest.Versioned is deprecated:
var SchemaVersion = manifest.Versioned{
	SchemaVersion: 2,
	MediaType:     v1.MediaTypeImageManifest,
}

func init() {
	if err := distribution.RegisterManifestSchema(v1.MediaTypeImageManifest, unmarshalOCISchema); err != nil {
		panic(fmt.Sprintf("Unable to register manifest: %s", err))
	}
}

func unmarshalOCISchema(b []byte) (distribution.Manifest, v1.Descriptor, error) {
	if err := validateManifest(b); err != nil {
		return nil, v1.Descriptor{}, err
	}

	m := &DeserializedManifest{}
	if err := m.UnmarshalJSON(b); err != nil {
		return nil, v1.Descriptor{}, err
	}

	return m, v1.Descriptor{
		MediaType:   v1.MediaTypeImageManifest,
		Digest:      digest.FromBytes(b),
		Size:        int64(len(b)),
		Annotations: m.Annotations,
	}, nil
}

// Manifest defines a ocischema manifest.
type Manifest struct {
	specs.Versioned

	// MediaType is the media type of this schema.
	MediaType string `json:"mediaType,omitempty"`

	// Config references the image configuration as a blob.
	Config v1.Descriptor `json:"config"`

	// Layers lists descriptors for the layers referenced by the
	// configuration.
	Layers []v1.Descriptor `json:"layers"`

	// Annotations contains arbitrary metadata for the image manifest.
	Annotations map[string]string `json:"annotations,omitempty"`
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

	if mfst.MediaType != "" && mfst.MediaType != v1.MediaTypeImageManifest {
		return fmt.Errorf("if present, mediaType in manifest should be '%s' not '%s'",
			v1.MediaTypeImageManifest, mfst.MediaType)
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
func (m *DeserializedManifest) Payload() (string, []byte, error) {
	return v1.MediaTypeImageManifest, m.canonical, nil
}

// validateManifest returns an error if the byte slice is invalid JSON or if it
// contains fields that belong to a index
func validateManifest(b []byte) error {
	var doc struct {
		Manifests interface{} `json:"manifests,omitempty"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	if doc.Manifests != nil {
		return errors.New("ocimanifest: expected manifest but found index")
	}
	return nil
}

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

// IndexSchemaVersion provides a pre-initialized version structure for OCI Image
// Indices.
//
// Deprecated: use [specs.Versioned] and set MediaType on the manifest
// to [v1.MediaTypeImageIndex].
//
//nolint:staticcheck // ignore SA1019: manifest.Versioned is deprecated:
var IndexSchemaVersion = manifest.Versioned{
	SchemaVersion: 2,
	MediaType:     v1.MediaTypeImageIndex,
}

func init() {
	if err := distribution.RegisterManifestSchema(v1.MediaTypeImageIndex, unmarshalImageIndex); err != nil {
		panic(fmt.Sprintf("Unable to register OCI Image Index: %s", err))
	}
}

func unmarshalImageIndex(b []byte) (distribution.Manifest, v1.Descriptor, error) {
	if err := validateIndex(b); err != nil {
		return nil, v1.Descriptor{}, err
	}

	m := &DeserializedImageIndex{}
	if err := m.UnmarshalJSON(b); err != nil {
		return nil, v1.Descriptor{}, err
	}

	if m.MediaType != "" && m.MediaType != v1.MediaTypeImageIndex {
		return nil, v1.Descriptor{}, fmt.Errorf("if present, mediaType in image index should be '%s' not '%s'", v1.MediaTypeImageIndex, m.MediaType)
	}

	return m, v1.Descriptor{
		MediaType:   v1.MediaTypeImageIndex,
		Digest:      digest.FromBytes(b),
		Size:        int64(len(b)),
		Annotations: m.Annotations,
	}, nil
}

// ImageIndex references manifests for various platforms.
type ImageIndex struct {
	specs.Versioned

	// MediaType is the media type of this schema.
	MediaType string `json:"mediaType,omitempty"`

	// Manifests references a list of manifests
	Manifests []v1.Descriptor `json:"manifests"`

	// Annotations is an optional field that contains arbitrary metadata for the
	// image index
	Annotations map[string]string `json:"annotations,omitempty"`
}

// References returns the distribution descriptors for the referenced image
// manifests.
func (ii ImageIndex) References() []v1.Descriptor {
	return ii.Manifests
}

// DeserializedImageIndex wraps ManifestList with a copy of the original
// JSON.
type DeserializedImageIndex struct {
	ImageIndex

	// canonical is the canonical byte representation of the Manifest.
	canonical []byte
}

// FromDescriptors takes a slice of descriptors and a map of annotations, and
// returns a DeserializedManifestList which contains the resulting manifest list
// and its JSON representation. If annotations is nil or empty then the
// annotations property will be omitted from the JSON representation.
func FromDescriptors(descriptors []v1.Descriptor, annotations map[string]string) (*DeserializedImageIndex, error) {
	return fromDescriptorsWithMediaType(descriptors, annotations, v1.MediaTypeImageIndex)
}

// fromDescriptorsWithMediaType is for testing purposes, it's useful to be able to specify the media type explicitly
func fromDescriptorsWithMediaType(descriptors []v1.Descriptor, annotations map[string]string, mediaType string) (_ *DeserializedImageIndex, err error) {
	m := ImageIndex{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   mediaType,
		Annotations: annotations,
	}

	m.Manifests = make([]v1.Descriptor, len(descriptors))
	copy(m.Manifests, descriptors)

	deserialized := DeserializedImageIndex{
		ImageIndex: m,
	}

	deserialized.canonical, err = json.MarshalIndent(&m, "", "   ")
	return &deserialized, err
}

// UnmarshalJSON populates a new ManifestList struct from JSON data.
func (m *DeserializedImageIndex) UnmarshalJSON(b []byte) error {
	m.canonical = make([]byte, len(b))
	// store manifest list in canonical
	copy(m.canonical, b)

	// Unmarshal canonical JSON into ManifestList object
	var manifestList ImageIndex
	if err := json.Unmarshal(m.canonical, &manifestList); err != nil {
		return err
	}

	m.ImageIndex = manifestList

	return nil
}

// MarshalJSON returns the contents of canonical. If canonical is empty,
// marshals the inner contents.
func (m *DeserializedImageIndex) MarshalJSON() ([]byte, error) {
	if len(m.canonical) > 0 {
		return m.canonical, nil
	}

	return nil, errors.New("JSON representation not initialized in DeserializedImageIndex")
}

// Payload returns the raw content of the manifest list. The contents can be
// used to calculate the content identifier.
func (m DeserializedImageIndex) Payload() (string, []byte, error) {
	mediaType := m.MediaType
	if m.MediaType == "" {
		mediaType = v1.MediaTypeImageIndex
	}

	return mediaType, m.canonical, nil
}

// validateIndex returns an error if the byte slice is invalid JSON or if it
// contains fields that belong to a manifest
func validateIndex(b []byte) error {
	var doc struct {
		Config interface{} `json:"config,omitempty"`
		Layers interface{} `json:"layers,omitempty"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	if doc.Config != nil || doc.Layers != nil {
		return errors.New("index: expected index but found manifest")
	}
	return nil
}

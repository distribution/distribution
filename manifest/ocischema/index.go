package ocischema

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// OCISchemaVersion provides a pre-initialized version structure for this
// packages OCIschema version of the manifest.
var OCISchemaVersion = manifest.Versioned{
	SchemaVersion: 2,
	MediaType:     v1.MediaTypeImageIndex,
}

func init() {
	imageIndexFunc := func(b []byte) (distribution.Manifest, distribution.Descriptor, error) {
		if err := validateIndex(b); err != nil {
			return nil, distribution.Descriptor{}, err
		}
		m := new(DeserializedImageIndex)
		err := m.UnmarshalJSON(b)
		if err != nil {
			return nil, distribution.Descriptor{}, err
		}

		if m.MediaType != "" && m.MediaType != v1.MediaTypeImageIndex {
			err = fmt.Errorf("if present, mediaType in image index should be '%s' not '%s'",
				v1.MediaTypeImageIndex, m.MediaType)

			return nil, distribution.Descriptor{}, err
		}

		dgst := digest.FromBytes(b)
		return m, distribution.Descriptor{Digest: dgst, Size: int64(len(b)), MediaType: v1.MediaTypeImageIndex}, err
	}
	err := distribution.RegisterManifestSchema(v1.MediaTypeImageIndex, imageIndexFunc)
	if err != nil {
		panic(fmt.Sprintf("Unable to register OCI Image Index: %s", err))
	}
}

// A ManifestDescriptor references a platform-specific manifest.
type ManifestDescriptor struct {
	distribution.Descriptor

	// Platform specifies which platform the manifest pointed to by the
	// descriptor runs on.
	Platform *v1.Platform `json:"platform,omitempty"`
}

// ImageIndex references manifests for various platforms.
type ImageIndex struct {
	manifest.Versioned

	// Manifests references a list of manifests
	Manifests []ManifestDescriptor `json:"manifests"`

	// Annotations is an optional field that contains arbitrary metadata for the
	// image index
	Annotations map[string]string `json:"annotations,omitempty"`
}

// References returns the distribution descriptors for the referenced image
// manifests.
func (ii ImageIndex) References() []distribution.Descriptor {
	dependencies := make([]distribution.Descriptor, len(ii.Manifests))
	for i := range ii.Manifests {
		dependencies[i] = ii.Manifests[i].Descriptor
		dependencies[i].Platform = ii.Manifests[i].Platform
	}

	return dependencies
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
func FromDescriptors(descriptors []ManifestDescriptor, annotations map[string]string) (*DeserializedImageIndex, error) {
	return fromDescriptorsWithMediaType(descriptors, annotations, v1.MediaTypeImageIndex)
}

// fromDescriptorsWithMediaType is for testing purposes, it's useful to be able to specify the media type explicitly
func fromDescriptorsWithMediaType(descriptors []ManifestDescriptor, annotations map[string]string, mediaType string) (*DeserializedImageIndex, error) {
	m := ImageIndex{
		Versioned: manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     mediaType,
		},
		Annotations: annotations,
	}

	m.Manifests = make([]ManifestDescriptor, len(descriptors))
	copy(m.Manifests, descriptors)

	deserialized := DeserializedImageIndex{
		ImageIndex: m,
	}

	var err error
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
	var mediaType string
	if m.MediaType == "" {
		mediaType = v1.MediaTypeImageIndex
	} else {
		mediaType = m.MediaType
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

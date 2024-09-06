package manifestlist

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
	// MediaTypeManifestList specifies the mediaType for manifest lists.
	MediaTypeManifestList = "application/vnd.docker.distribution.manifest.list.v2+json"
)

// SchemaVersion provides a pre-initialized version structure for this
// packages version of the manifest.
//
// Deprecated: use [specs.Versioned] and set MediaType on the manifest
// to [MediaTypeManifestList].
//
//nolint:staticcheck // ignore SA1019: manifest.Versioned is deprecated:
var SchemaVersion = manifest.Versioned{
	SchemaVersion: 2,
	MediaType:     MediaTypeManifestList,
}

func init() {
	if err := distribution.RegisterManifestSchema(MediaTypeManifestList, unmarshalManifestList); err != nil {
		panic(fmt.Sprintf("Unable to register manifest: %s", err))
	}
}

func unmarshalManifestList(b []byte) (distribution.Manifest, v1.Descriptor, error) {
	m := &DeserializedManifestList{}
	if err := m.UnmarshalJSON(b); err != nil {
		return nil, v1.Descriptor{}, err
	}

	if m.MediaType != MediaTypeManifestList {
		return nil, v1.Descriptor{}, fmt.Errorf("mediaType in manifest list should be '%s' not '%s'", MediaTypeManifestList, m.MediaType)
	}

	return m, v1.Descriptor{
		Digest:    digest.FromBytes(b),
		Size:      int64(len(b)),
		MediaType: MediaTypeManifestList,
	}, nil
}

// PlatformSpec specifies a platform where a particular image manifest is
// applicable.
type PlatformSpec struct {
	// Architecture field specifies the CPU architecture, for example
	// `amd64` or `ppc64`.
	Architecture string `json:"architecture"`

	// OS specifies the operating system, for example `linux` or `windows`.
	OS string `json:"os"`

	// OSVersion is an optional field specifying the operating system
	// version, for example `10.0.10586`.
	OSVersion string `json:"os.version,omitempty"`

	// OSFeatures is an optional field specifying an array of strings,
	// each listing a required OS feature (for example on Windows `win32k`).
	OSFeatures []string `json:"os.features,omitempty"`

	// Variant is an optional field specifying a variant of the CPU, for
	// example `ppc64le` to specify a little-endian version of a PowerPC CPU.
	Variant string `json:"variant,omitempty"`

	// Features is an optional field specifying an array of strings, each
	// listing a required CPU feature (for example `sse4` or `aes`).
	Features []string `json:"features,omitempty"`
}

// A ManifestDescriptor references a platform-specific manifest.
type ManifestDescriptor struct {
	v1.Descriptor

	// Platform specifies which platform the manifest pointed to by the
	// descriptor runs on.
	Platform PlatformSpec `json:"platform"`
}

// ManifestList references manifests for various platforms.
type ManifestList struct {
	specs.Versioned

	// MediaType is the media type of this schema.
	MediaType string `json:"mediaType,omitempty"`

	// Manifests references a list of manifests
	Manifests []ManifestDescriptor `json:"manifests"`
}

// References returns the distribution descriptors for the referenced image
// manifests.
func (m ManifestList) References() []v1.Descriptor {
	dependencies := make([]v1.Descriptor, len(m.Manifests))
	for i := range m.Manifests {
		dependencies[i] = m.Manifests[i].Descriptor
		dependencies[i].Platform = &v1.Platform{
			Architecture: m.Manifests[i].Platform.Architecture,
			OS:           m.Manifests[i].Platform.OS,
			OSVersion:    m.Manifests[i].Platform.OSVersion,
			OSFeatures:   m.Manifests[i].Platform.OSFeatures,
			Variant:      m.Manifests[i].Platform.Variant,
		}
	}

	return dependencies
}

// DeserializedManifestList wraps ManifestList with a copy of the original
// JSON.
type DeserializedManifestList struct {
	ManifestList

	// canonical is the canonical byte representation of the Manifest.
	canonical []byte
}

// FromDescriptors takes a slice of descriptors, and returns a
// DeserializedManifestList which contains the resulting manifest list
// and its JSON representation.
func FromDescriptors(descriptors []ManifestDescriptor) (*DeserializedManifestList, error) {
	return fromDescriptorsWithMediaType(descriptors, MediaTypeManifestList)
}

// fromDescriptorsWithMediaType is for testing purposes, it's useful to be able to specify the media type explicitly
func fromDescriptorsWithMediaType(descriptors []ManifestDescriptor, mediaType string) (*DeserializedManifestList, error) {
	m := ManifestList{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: mediaType,
	}

	m.Manifests = make([]ManifestDescriptor, len(descriptors))
	copy(m.Manifests, descriptors)

	deserialized := DeserializedManifestList{
		ManifestList: m,
	}

	var err error
	deserialized.canonical, err = json.MarshalIndent(&m, "", "   ")
	return &deserialized, err
}

// UnmarshalJSON populates a new ManifestList struct from JSON data.
func (m *DeserializedManifestList) UnmarshalJSON(b []byte) error {
	m.canonical = make([]byte, len(b))
	// store manifest list in canonical
	copy(m.canonical, b)

	// Unmarshal canonical JSON into ManifestList object
	var manifestList ManifestList
	if err := json.Unmarshal(m.canonical, &manifestList); err != nil {
		return err
	}

	m.ManifestList = manifestList

	return nil
}

// MarshalJSON returns the contents of canonical. If canonical is empty,
// marshals the inner contents.
func (m *DeserializedManifestList) MarshalJSON() ([]byte, error) {
	if len(m.canonical) > 0 {
		return m.canonical, nil
	}

	return nil, errors.New("JSON representation not initialized in DeserializedManifestList")
}

// Payload returns the raw content of the manifest list. The contents can be
// used to calculate the content identifier.
func (m DeserializedManifestList) Payload() (string, []byte, error) {
	var mediaType string
	if m.MediaType == "" {
		mediaType = v1.MediaTypeImageIndex
	} else {
		mediaType = m.MediaType
	}

	return mediaType, m.canonical, nil
}

// validateManifestList returns an error if the byte slice is invalid JSON or if it
// contains fields that belong to a manifest
func validateManifestList(b []byte) error {
	var doc struct {
		Config interface{} `json:"config,omitempty"`
		Layers interface{} `json:"layers,omitempty"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return err
	}
	if doc.Config != nil || doc.Layers != nil {
		return errors.New("manifestlist: expected list but found manifest")
	}
	return nil
}

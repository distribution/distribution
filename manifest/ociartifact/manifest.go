package ociartifact

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func init() {
	artifactFunc := func(b []byte) (distribution.Manifest, distribution.Descriptor, error) {
		m := &DeserializedManifest{}
		err := m.UnmarshalJSON(b)
		if err != nil {
			return nil, distribution.Descriptor{}, err
		}

		dgst := digest.FromBytes(b)
		return m, distribution.Descriptor{Digest: dgst, Size: int64(len(b)), MediaType: v1.MediaTypeArtifactManifest}, nil
	}
	if err := distribution.RegisterManifestSchema(v1.MediaTypeArtifactManifest, artifactFunc); err != nil {
		panic(fmt.Sprintf("Unable to register manifest: %s", err))
	}
}

// Manifest defines an oci artifact manifest.
type Manifest struct {
	manifest.Unversioned

	// ArtifactType is the media type of the artifact referenced by this
	// artifact manifest.
	ArtifactType string `json:"artifactType,omitempty"`

	// Blobs lists the descriptors for the files making up the artifact
	// referenced by this this artifact manifest.
	Blobs []distribution.Descriptor `json:"blobs,omitempty"`

	// Subject is the descriptor of a manifest referred to by this artifact.
	Subject *distribution.Descriptor `json:"subject,omitempty"`

	// Annotations contain arbitrary metadata for the image manifest
	Annotations map[string]string `json:"annotations,omitempty"`
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

// DeserializedManifest wraps Manifest with a copy of the original JSON.
// It satisfies the distribution.Manifest interface.
type DeserializedManifest struct {
	Manifest

	// canonical is the canonical byte representation of the Manifest.
	canonical []byte
}

func (m *DeserializedManifest) UnmarshalJSON(b []byte) error {
	m.canonical = make([]byte, len(b))
	copy(m.canonical, b)

	var manifest Manifest
	if err := json.Unmarshal(m.canonical, &manifest); err != nil {
		return err
	}

	if manifest.MediaType != v1.MediaTypeArtifactManifest {
		return fmt.Errorf("mediaType in manifest must be '%q' not '%s'",
			v1.MediaTypeArtifactManifest, manifest.MediaType)
	}

	// The subject if specified must be a must be a manifest. This is validated
	// here rather then in the storage manifest Put handler because the subject
	// does not have to exist, so there is nothing to validate in the manifest
	// store. If a non-compliant client provided the digest of a blob then the
	// registry would still indicate that the referred manifest does not exist.
	if manifest.Subject != nil {
		if !distribution.ManifestMediaTypeSupported(manifest.Subject.MediaType) {
			return fmt.Errorf("subject.mediaType must be a manifest, not '%s'", manifest.Subject.MediaType)
		}
	}

	m.Manifest = manifest
	return nil
}

// MarshalJSON returns the contents of canonical. If canonical is empty,
// returns an error.
func (m *DeserializedManifest) MarshalJSON() ([]byte, error) {
	if len(m.canonical) > 0 {
		return m.canonical, nil
	}

	return nil, errors.New("JSON representation not initialized in DeserializedManifest")
}

// Payload returns the raw content of the manifest. The contents can be used to
// calculate the content identifier.
func (m *DeserializedManifest) Payload() (string, []byte, error) {
	return v1.MediaTypeArtifactManifest, m.canonical, nil
}

// References returns the descriptors of this manifest's blobs only.
func (m *DeserializedManifest) References() []distribution.Descriptor {
	return m.Blobs
}

// Subject returns a pointer to the subject of this manifest or nil if there is
// none
func (m *DeserializedManifest) Subject() *distribution.Descriptor {
	return m.Manifest.Subject
}

// Type returns the artifactType of this manifest if there is one, otherwise it
// returns empty string.
func (m *DeserializedManifest) Type() string {
	return m.Manifest.ArtifactType
}

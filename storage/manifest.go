package storage

import (
	"encoding/json"
	"fmt"

	"github.com/docker/libtrust"

	"github.com/docker/docker-registry/digest"
)

var (
	// ErrManifestUnknown is returned if the manifest is not known by the
	// registry.
	ErrManifestUnknown = fmt.Errorf("unknown manifest")

	// ErrManifestUnverified is returned when the registry is unable to verify
	// the manifest.
	ErrManifestUnverified = fmt.Errorf("unverified manifest")
)

// Versioned provides a struct with just the manifest schemaVersion. Incoming
// content with unknown schema version can be decoded against this struct to
// check the version.
type Versioned struct {
	// SchemaVersion is the image manifest schema that this image follows
	SchemaVersion int `json:"schemaVersion"`
}

// Manifest provides the base accessible fields for working with V2 image
// format in the registry.
type Manifest struct {
	Versioned

	// Name is the name of the image's repository
	Name string `json:"name"`

	// Tag is the tag of the image specified by this manifest
	Tag string `json:"tag"`

	// Architecture is the host architecture on which this image is intended to
	// run
	Architecture string `json:"architecture"`

	// FSLayers is a list of filesystem layer blobSums contained in this image
	FSLayers []FSLayer `json:"fsLayers"`

	// History is a list of unstructured historical data for v1 compatibility
	History []ManifestHistory `json:"history"`
}

// Sign signs the manifest with the provided private key, returning a
// SignedManifest. This typically won't be used within the registry, except
// for testing.
func (m *Manifest) Sign(pk libtrust.PrivateKey) (*SignedManifest, error) {
	p, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	js, err := libtrust.NewJSONSignature(p)
	if err != nil {
		return nil, err
	}

	if err := js.Sign(pk); err != nil {
		return nil, err
	}

	pretty, err := js.PrettySignature("signatures")
	if err != nil {
		return nil, err
	}

	return &SignedManifest{
		Manifest: *m,
		Raw:      pretty,
	}, nil
}

// SignedManifest provides an envelope for
type SignedManifest struct {
	Manifest

	// Raw is the byte representation of the ImageManifest, used for signature
	// verification. The manifest byte representation cannot change or it will
	// have to be re-signed.
	Raw []byte `json:"-"`
}

// UnmarshalJSON populates a new ImageManifest struct from JSON data.
func (m *SignedManifest) UnmarshalJSON(b []byte) error {
	var manifest Manifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	m.Manifest = manifest
	m.Raw = b

	return nil
}

// MarshalJSON returns the contents of raw. If Raw is nil, marshals the inner
// contents.
func (m *SignedManifest) MarshalJSON() ([]byte, error) {
	if len(m.Raw) > 0 {
		return m.Raw, nil
	}

	// If the raw data is not available, just dump the inner content.
	return json.Marshal(&m.Manifest)
}

// FSLayer is a container struct for BlobSums defined in an image manifest
type FSLayer struct {
	// BlobSum is the tarsum of the referenced filesystem image layer
	BlobSum digest.Digest `json:"blobSum"`
}

// ManifestHistory stores unstructured v1 compatibility information
type ManifestHistory struct {
	// V1Compatibility is the raw v1 compatibility information
	V1Compatibility string `json:"v1Compatibility"`
}

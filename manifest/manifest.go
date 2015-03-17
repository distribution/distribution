package manifest

import (
	"encoding/json"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/libtrust"
)

// TODO(stevvooe): When we rev the manifest format, the contents of this
// package should me moved to manifest/v1.

const (
	// ManifestMediaType specifies the mediaType for the current version. Note
	// that for schema version 1, the the media is optionally
	// "application/json".
	ManifestMediaType = "application/vnd.docker.distribution.manifest.v1+json"
)

// Versioned provides a struct with just the manifest schemaVersion. Incoming
// content with unknown schema version can be decoded against this struct to
// check the version.
type Versioned struct {
	// SchemaVersion is the image manifest schema that this image follows
	SchemaVersion int `json:"schemaVersion"`
}

// FSLayer is a container struct for BlobSums defined in an image manifest
type FSLayer struct {
	// BlobSum is the tarsum of the referenced filesystem image layer
	BlobSum digest.Digest `json:"blobSum"`
}

// History stores unstructured v1 compatibility information
type History struct {
	// V1Compatibility is the raw v1 compatibility information
	V1Compatibility string `json:"v1Compatibility"`
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
	History []History `json:"history"`
}

func (m Manifest) Dependencies() []distribution.Descriptor {
	// Bah, this format is junk:
	//	1. We don't know the size, so it won't be specified in the descriptor.
	//       Conversions to new manifest types will have to include layer
	//       size.
	//  2. FSLayers is in the wrong order. Must iterate over it backwards

	dependencies := make([]distribution.Descriptor, len(m.FSLayers))
	for i := len(m.FSLayers) - 1; i >= 0; i-- {
		fsLayer := m.FSLayers[i]
		dependencies[len(m.FSLayers)-i] = distribution.Descriptor{
			MediaType: "application/vnd.docker.container.image.rootfs.diff+x-gtar",
			Digest:    fsLayer.BlobSum,
		}
	}

	return dependencies
}

// SignedManifest provides an envelope for a signed image manifest, including
// the format sensitive raw bytes. It contains fields to
type SignedManifest struct {
	Manifest

	// Blob provides access to the serialized manifest payload that is
	// targeted by the signatures. Note that if signatures are required as
	// part of an API response, Raw should be used instead.
	distribution.Blob `json:"-"`

	// Raw is the byte representation of the ImageManifest, used for signature
	// verification. The value of Raw must be used directly during
	// serialization, or the signature check will fail. The manifest byte
	// representation cannot change or it will have to be re-signed. This
	// differs from the contents of blob in that it includes the signatures.
	Raw []byte `json:"-"`

	// Signatures provides access to the signatures of the targeted blob.
	// Please use this field sparingly.
	Signatures [][]byte `json:"-"`
}

var _ distribution.Manifest = &SignedManifest{}

// UnmarshalJSON populates a new ImageManifest struct from JSON data.
func (sm *SignedManifest) UnmarshalJSON(b []byte) error {
	var manifest Manifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	jsig, err := libtrust.ParsePrettySignature(b, "signatures")
	if err != nil {
		return err
	}

	p, err := jsig.Payload()
	if err != nil {
		return err
	}

	// create blob from the payload.
	blob, err := distribution.NewBlobFromBytes(ManifestMediaType, p)
	if err != nil {
		return err
	}

	sigs, err := jsig.Signatures()
	if err != nil {
		return err
	}

	sm.Raw = make([]byte, len(b), len(b))
	copy(sm.Raw, b)
	sm.Signatures = sigs
	sm.Blob = blob
	sm.Manifest = manifest

	return nil
}

// MarshalJSON returns the contents of raw. If Raw is nil, marshals the inner
// contents. Applications requiring a marshaled signed manifest should simply
// use Raw directly, since the the content produced by json.Marshal will be
// compacted and will fail signature checks.
func (sm *SignedManifest) MarshalJSON() ([]byte, error) {
	if len(sm.Raw) > 0 {
		return sm.Raw, nil
	}

	// If the raw data is not available, just dump the inner content.
	return json.Marshal(&sm.Manifest)
}

package storage

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Sirupsen/logrus"

	"github.com/docker/libtrust"

	"github.com/docker/docker-registry/digest"
)

// ErrUnknownManifest is returned if the manifest is not known by the
// registry.
type ErrUnknownManifest struct {
	Name string
	Tag  string
}

func (err ErrUnknownManifest) Error() string {
	return fmt.Sprintf("unknown manifest name=%s tag=%s", err.Name, err.Tag)
}

// ErrManifestUnverified is returned when the registry is unable to verify
// the manifest.
type ErrManifestUnverified struct{}

func (ErrManifestUnverified) Error() string {
	return fmt.Sprintf("unverified manifest")
}

// ErrManifestVerification provides a type to collect errors encountered
// during manifest verification. Currently, it accepts errors of all types,
// but it may be narrowed to those involving manifest verification.
type ErrManifestVerification []error

func (errs ErrManifestVerification) Error() string {
	var parts []string
	for _, err := range errs {
		parts = append(parts, err.Error())
	}

	return fmt.Sprintf("errors verifying manifest: %v", strings.Join(parts, ","))
}

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
	p, err := json.MarshalIndent(m, "", "   ")
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

// SignWithChain signs the manifest with the given private key and x509 chain.
// The public key of the first element in the chain must be the public key
// corresponding with the sign key.
func (m *Manifest) SignWithChain(key libtrust.PrivateKey, chain []*x509.Certificate) (*SignedManifest, error) {
	p, err := json.MarshalIndent(m, "", "   ")
	if err != nil {
		return nil, err
	}

	js, err := libtrust.NewJSONSignature(p)
	if err != nil {
		return nil, err
	}

	if err := js.SignWithChain(key, chain); err != nil {
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

// SignedManifest provides an envelope for a signed image manifest, including
// the format sensitive raw bytes. It contains fields to
type SignedManifest struct {
	Manifest

	// Raw is the byte representation of the ImageManifest, used for signature
	// verification. The manifest byte representation cannot change or it will
	// have to be re-signed.
	Raw []byte `json:"-"`
}

// Verify verifies the signature of the signed manifest returning the public
// keys used during signing.
func (sm *SignedManifest) Verify() ([]libtrust.PublicKey, error) {
	js, err := libtrust.ParsePrettySignature(sm.Raw, "signatures")
	if err != nil {
		logrus.WithField("err", err).Debugf("(*SignedManifest).Verify")
		return nil, err
	}

	return js.Verify()
}

// VerifyChains verifies the signature of the signed manifest against the
// certificate pool returning the list of verified chains. Signatures without
// an x509 chain are not checked.
func (sm *SignedManifest) VerifyChains(ca *x509.CertPool) ([][]*x509.Certificate, error) {
	js, err := libtrust.ParsePrettySignature(sm.Raw, "signatures")
	if err != nil {
		return nil, err
	}

	return js.VerifyChains(ca)
}

// UnmarshalJSON populates a new ImageManifest struct from JSON data.
func (sm *SignedManifest) UnmarshalJSON(b []byte) error {
	var manifest Manifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return err
	}

	sm.Manifest = manifest
	sm.Raw = make([]byte, len(b), len(b))
	copy(sm.Raw, b)

	return nil
}

// MarshalJSON returns the contents of raw. If Raw is nil, marshals the inner
// contents.
func (sm *SignedManifest) MarshalJSON() ([]byte, error) {
	if len(sm.Raw) > 0 {
		return sm.Raw, nil
	}

	// If the raw data is not available, just dump the inner content.
	return json.Marshal(&sm.Manifest)
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

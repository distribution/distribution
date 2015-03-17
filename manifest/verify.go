package manifest

import (
	"crypto/x509"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/libtrust"
)

// Verify verifies the signature of the signed manifest returning the public
// keys used during signing.
func Verify(m distribution.Manifest) ([]libtrust.PublicKey, error) {
	sm, ok := m.(*SignedManifest)
	if !ok {
		// TODO(stevvooe): This is currently an old restriction. We'll
		// refactor this to move away from this concept.
		return nil, fmt.Errorf("verification only available for *SignedManifest")
	}

	js, err := libtrust.ParsePrettySignature(sm.Raw, "signatures")
	if err != nil {
		logrus.WithField("err", err).Debugf("manifest.Verify")
		return nil, err
	}

	return js.Verify()
}

// VerifyChains verifies the signature of the signed manifest against the
// certificate pool returning the list of verified chains. Signatures without
// an x509 chain are not checked.
func VerifyChains(m distribution.Manifest, ca *x509.CertPool) ([][]*x509.Certificate, error) {
	sm, ok := m.(*SignedManifest)
	if !ok {
		// TODO(stevvooe): This is currently an old restriction. We'll
		// refactor this to move away from this concept.
		return nil, fmt.Errorf("verification only available for *SignedManifest")
	}

	js, err := libtrust.ParsePrettySignature(sm.Raw, "signatures")
	if err != nil {
		return nil, err
	}

	return js.VerifyChains(ca)
}

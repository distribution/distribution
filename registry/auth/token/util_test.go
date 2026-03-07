package token

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

func TestEd25519JWKThumbprint(t *testing.T) {
	// Test vector from RFC 8037 Appendix A.2:
	// https://datatracker.ietf.org/doc/html/rfc8037#appendix-A.2
	examplePubKeyBase64 := "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo"
	// Canonical thumbprint from RFC 8037 Appendix A.3:
	// https://datatracker.ietf.org/doc/html/rfc8037#appendix-A.3
	expected := "kPrK_qmxVWaYVA9wwBF6Iuo3vVzz7TxHCTwXBygrS4k"

	pubBytes, err := base64.RawURLEncoding.DecodeString(examplePubKeyBase64)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := ed25519.PublicKey(pubBytes)

	got := GetJWKThumbprint(publicKey)
	if got != expected {
		t.Errorf("GetJWKThumbprint(ed25519) = %q, expected %q", got, expected)
	}
}

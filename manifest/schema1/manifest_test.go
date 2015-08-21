package schema1

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/docker/libtrust"
)

type testEnv struct {
	name, tag string
	manifest  *Manifest
	signed    *SignedManifest
	pk        libtrust.PrivateKey
}

func TestManifestMarshaling(t *testing.T) {
	env := genEnv(t)

	// Check that the Raw field is the same as json.MarshalIndent with these
	// parameters.
	p, err := json.MarshalIndent(env.signed, "", "   ")
	if err != nil {
		t.Fatalf("error marshaling manifest: %v", err)
	}

	if !bytes.Equal(p, env.signed.Raw) {
		t.Fatalf("manifest bytes not equal: %q != %q", string(env.signed.Raw), string(p))
	}
}

func TestManifestUnmarshaling(t *testing.T) {
	env := genEnv(t)

	var signed SignedManifest
	if err := json.Unmarshal(env.signed.Raw, &signed); err != nil {
		t.Fatalf("error unmarshaling signed manifest: %v", err)
	}

	if !reflect.DeepEqual(&signed, env.signed) {
		t.Fatalf("manifests are different after unmarshaling: %v != %v", signed, env.signed)
	}
}

func TestManifestVerification(t *testing.T) {
	env := genEnv(t)

	publicKeys, err := Verify(env.signed)
	if err != nil {
		t.Fatalf("error verifying manifest: %v", err)
	}

	if len(publicKeys) == 0 {
		t.Fatalf("no public keys found in signature")
	}

	var found bool
	publicKey := env.pk.PublicKey()
	// ensure that one of the extracted public keys matches the private key.
	for _, candidate := range publicKeys {
		if candidate.KeyID() == publicKey.KeyID() {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected public key, %v, not found in verified keys: %v", publicKey, publicKeys)
	}
}

func genEnv(t *testing.T) *testEnv {
	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("error generating test key: %v", err)
	}

	name, tag := "foo/bar", "test"

	m := Manifest{
		Versioned: SchemaVersion,
		Name:      name,
		Tag:       tag,
		FSLayers: []FSLayer{
			{
				BlobSum: "asdf",
			},
			{
				BlobSum: "qwer",
			},
		},
	}

	sm, err := Sign(&m, pk)
	if err != nil {
		t.Fatalf("error signing manifest: %v", err)
	}

	return &testEnv{
		name:     name,
		tag:      tag,
		manifest: &m,
		signed:   sm,
		pk:       pk,
	}
}

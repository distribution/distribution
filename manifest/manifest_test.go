package manifest

import (
	"bytes"
	"encoding/json"
	"reflect"
	"runtime"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/libtrust"
)

type testEnv struct {
	name, tag string
	manifest  distribution.Manifest
	pk        libtrust.PrivateKey
}

func TestManifestMarshaling(t *testing.T) {
	env := genEnv(t)

	// Check that the Raw field is the same as json.MarshalIndent with these
	// parameters.
	p, err := json.Marshal(env.manifest)
	if err != nil {
		t.Fatalf("error marshaling manifest: %v", err)
	}

	raw := env.manifest.(*SignedManifest).Raw
	if !bytes.Equal(p, raw) {
		t.Fatalf("manifest bytes not equal: %q != %q", string(raw), string(p))
	}
}

func TestManifestUnmarshaling(t *testing.T) {
	env := genEnv(t)

	var signed SignedManifest
	if err := json.Unmarshal(env.manifest.(*SignedManifest).Raw, &signed); err != nil {
		t.Fatalf("error unmarshaling signed manifest: %v", err)
	}

	if signed.Descriptor() != env.manifest.Descriptor() {
		t.Fatalf("blobs must be the same: %#v != %#v", signed.Descriptor(), env.manifest.Descriptor())
	}

	if !reflect.DeepEqual(signed.Manifest, env.manifest.(*SignedManifest).Manifest) {
		t.Fatalf("inner manifests must be equal: %#v != %#v", signed.Manifest, env.manifest.(*SignedManifest).Manifest)
	}

	if !reflect.DeepEqual(&signed, env.manifest) {
		t.Fatalf("manifests are different after unmarshaling: %#v != %#v", signed, env.manifest)
	}
}

func TestManifestVerification(t *testing.T) {
	env := genEnv(t)

	publicKeys, err := Verify(env.manifest)
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

func TestManifestInterface(t *testing.T) {
	env := genEnv(t)

	fsLayers := env.manifest.(*SignedManifest).FSLayers

	for i, dep := range env.manifest.Dependencies() {
		if dep.Digest != fsLayers[len(fsLayers)-i].BlobSum {
			t.Fatalf("depedendencies returned in incorrect order")
		}
	}
}

func genEnv(t *testing.T) *testEnv {
	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("error generating test key: %v", err)
	}

	name, tag := "foo/bar", "test"
	mb := NewManifestBuilder(pk, name, tag, runtime.GOARCH)

	mb.AddDependency(Dependency{Digest: "qwer"})
	mb.AddDependency(Dependency{Digest: "asdf"})

	m, err := mb.Build()
	if err != nil {
		t.Fatalf("unexpected error building manifest: %v", err)
	}

	return &testEnv{
		name:     name,
		tag:      tag,
		manifest: m,
		pk:       pk,
	}
}

package storage

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/docker/distribution/registry/storage/cache"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/testutil"
	"github.com/docker/libtrust"
	"golang.org/x/net/context"
)

type manifestStoreTestEnv struct {
	ctx        context.Context
	driver     driver.StorageDriver
	registry   distribution.Namespace
	repository distribution.Repository
	name       string
	tag        string
}

func newManifestStoreTestEnv(t *testing.T, name, tag string) *manifestStoreTestEnv {
	ctx := context.Background()
	driver := inmemory.New()
	registry := NewRegistryWithDriver(driver, cache.NewInMemoryLayerInfoCache())

	repo, err := registry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	return &manifestStoreTestEnv{
		ctx:        ctx,
		driver:     driver,
		registry:   registry,
		repository: repo,
		name:       name,
		tag:        tag,
	}
}

func TestManifestStorage(t *testing.T) {
	env := newManifestStoreTestEnv(t, "foo/bar", "thetag")
	ms := env.repository.Manifests()

	exists, err := ms.ExistsByTag(env.tag)
	if err != nil {
		t.Fatalf("unexpected error checking manifest existence: %v", err)
	}

	if exists {
		t.Fatalf("manifest should not exist")
	}

	if _, err := ms.GetByTag(env.tag); true {
		switch err.(type) {
		case distribution.ErrManifestUnknown:
			break
		default:
			t.Fatalf("expected manifest unknown error: %#v", err)
		}
	}

	m := manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: env.name,
		Tag:  env.tag,
	}

	// Build up some test layers and add them to the manifest, saving the
	// readseekers for upload later.
	testLayers := map[digest.Digest]io.ReadSeeker{}
	for i := 0; i < 2; i++ {
		rs, ds, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("unexpected error generating test layer file")
		}
		dgst := digest.Digest(ds)

		testLayers[digest.Digest(dgst)] = rs
		m.FSLayers = append(m.FSLayers, manifest.FSLayer{
			BlobSum: dgst,
		})
	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	sm, err := manifest.Sign(&m, pk)
	if err != nil {
		t.Fatalf("error signing manifest: %v", err)
	}

	err = ms.Put(sm)
	if err == nil {
		t.Fatalf("expected errors putting manifest")
	}

	// TODO(stevvooe): We expect errors describing all of the missing layers.

	// Now, upload the layers that were missing!
	for dgst, rs := range testLayers {
		upload, err := env.repository.Layers().Upload()
		if err != nil {
			t.Fatalf("unexpected error creating test upload: %v", err)
		}

		if _, err := io.Copy(upload, rs); err != nil {
			t.Fatalf("unexpected error copying to upload: %v", err)
		}

		if _, err := upload.Finish(dgst); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
	}

	if err = ms.Put(sm); err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	exists, err = ms.ExistsByTag(env.tag)
	if err != nil {
		t.Fatalf("unexpected error checking manifest existence: %v", err)
	}

	if !exists {
		t.Fatalf("manifest should exist")
	}

	fetchedManifest, err := ms.GetByTag(env.tag)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	if !reflect.DeepEqual(fetchedManifest, sm) {
		t.Fatalf("fetched manifest not equal: %#v != %#v", fetchedManifest, sm)
	}

	fetchedJWS, err := libtrust.ParsePrettySignature(fetchedManifest.Raw, "signatures")
	if err != nil {
		t.Fatalf("unexpected error parsing jws: %v", err)
	}

	payload, err := fetchedJWS.Payload()
	if err != nil {
		t.Fatalf("unexpected error extracting payload: %v", err)
	}

	// Now that we have a payload, take a moment to check that the manifest is
	// return by the payload digest.
	dgst, err := digest.FromBytes(payload)
	if err != nil {
		t.Fatalf("error getting manifest digest: %v", err)
	}

	exists, err = ms.Exists(dgst)
	if err != nil {
		t.Fatalf("error checking manifest existence by digest: %v", err)
	}

	if !exists {
		t.Fatalf("manifest %s should exist", dgst)
	}

	fetchedByDigest, err := ms.Get(dgst)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest by digest: %v", err)
	}

	if !reflect.DeepEqual(fetchedByDigest, fetchedManifest) {
		t.Fatalf("fetched manifest not equal: %#v != %#v", fetchedByDigest, fetchedManifest)
	}

	sigs, err := fetchedJWS.Signatures()
	if err != nil {
		t.Fatalf("unable to extract signatures: %v", err)
	}

	if len(sigs) != 1 {
		t.Fatalf("unexpected number of signatures: %d != %d", len(sigs), 1)
	}

	// Grabs the tags and check that this tagged manifest is present
	tags, err := ms.Tags()
	if err != nil {
		t.Fatalf("unexpected error fetching tags: %v", err)
	}

	if len(tags) != 1 {
		t.Fatalf("unexpected tags returned: %v", tags)
	}

	if tags[0] != env.tag {
		t.Fatalf("unexpected tag found in tags: %v != %v", tags, []string{env.tag})
	}

	// Now, push the same manifest with a different key
	pk2, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	sm2, err := manifest.Sign(&m, pk2)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}

	jws2, err := libtrust.ParsePrettySignature(sm2.Raw, "signatures")
	if err != nil {
		t.Fatalf("error parsing signature: %v", err)
	}

	sigs2, err := jws2.Signatures()
	if err != nil {
		t.Fatalf("unable to extract signatures: %v", err)
	}

	if len(sigs2) != 1 {
		t.Fatalf("unexpected number of signatures: %d != %d", len(sigs2), 1)
	}

	if err = ms.Put(sm2); err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	fetched, err := ms.GetByTag(env.tag)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	if _, err := manifest.Verify(fetched); err != nil {
		t.Fatalf("unexpected error verifying manifest: %v", err)
	}

	// Assemble our payload and two signatures to get what we expect!
	expectedJWS, err := libtrust.NewJSONSignature(payload, sigs[0], sigs2[0])
	if err != nil {
		t.Fatalf("unexpected error merging jws: %v", err)
	}

	expectedSigs, err := expectedJWS.Signatures()
	if err != nil {
		t.Fatalf("unexpected error getting expected signatures: %v", err)
	}

	receivedJWS, err := libtrust.ParsePrettySignature(fetched.Raw, "signatures")
	if err != nil {
		t.Fatalf("unexpected error parsing jws: %v", err)
	}

	receivedPayload, err := receivedJWS.Payload()
	if err != nil {
		t.Fatalf("unexpected error extracting received payload: %v", err)
	}

	if !bytes.Equal(receivedPayload, payload) {
		t.Fatalf("payloads are not equal")
	}

	receivedSigs, err := receivedJWS.Signatures()
	if err != nil {
		t.Fatalf("error getting signatures: %v", err)
	}

	for i, sig := range receivedSigs {
		if !bytes.Equal(sig, expectedSigs[i]) {
			t.Fatalf("mismatched signatures from remote: %v != %v", string(sig), string(expectedSigs[i]))
		}
	}

	// TODO(stevvooe): Currently, deletes are not supported due to some
	// complexity around managing tag indexes. We'll add this support back in
	// when the manifest format has settled. For now, we expect an error for
	// all deletes.
	if err := ms.Delete(dgst); err == nil {
		t.Fatalf("unexpected an error deleting manifest by digest: %v", err)
	}
}

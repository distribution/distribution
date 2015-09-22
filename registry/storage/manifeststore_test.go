package storage

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/testutil"
	"github.com/docker/libtrust"
)

type manifestStoreTestEnv struct {
	ctx        context.Context
	driver     driver.StorageDriver
	registry   distribution.Namespace
	repository distribution.Repository
	name       reference.Named
	tags       []string
}

func newManifestStoreTestEnv(t *testing.T, name reference.Named, tags ...string) *manifestStoreTestEnv {
	ctx := context.Background()
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(
		memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

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
		tags:       tags,
	}
}

func TestManifestStorage(t *testing.T) {
	repoName, _ := reference.ParseNamed("foo/bar")
	env := newManifestStoreTestEnv(t, repoName, "thetag")
	ctx := context.Background()
	ms, err := env.repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	m := schema1.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: env.name.Name(),
		Tag:  env.tags[0],
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
		m.FSLayers = append(m.FSLayers, schema1.FSLayer{
			BlobSum: dgst,
		})
		m.History = append(m.History, schema1.History{
			V1Compatibility: "",
		})

	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	sm, merr := schema1.Sign(&m, pk)
	if merr != nil {
		t.Fatalf("error signing manifest: %v", err)
	}

	_, err = ms.Put(ctx, sm)
	if err == nil {
		t.Fatalf("expected errors putting manifest with full verification")
	}

	switch err := err.(type) {
	case distribution.ErrManifestVerification:
		if len(err) != 2 {
			t.Fatalf("expected 2 verification errors: %#v", err)
		}

		for _, err := range err {
			if _, ok := err.(distribution.ErrManifestBlobUnknown); !ok {
				t.Fatalf("unexpected error type: %v", err)
			}
		}
	default:
		t.Fatalf("unexpected error verifying manifest: %v", err)
	}

	// Now, upload the layers that were missing!
	for dgst, rs := range testLayers {
		wr, err := env.repository.Blobs(env.ctx).Create(env.ctx)
		if err != nil {
			t.Fatalf("unexpected error creating test upload: %v", err)
		}

		if _, err := io.Copy(wr, rs); err != nil {
			t.Fatalf("unexpected error copying to upload: %v", err)
		}

		if _, err := wr.Commit(env.ctx, distribution.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
	}

	var manifestDigest digest.Digest
	if manifestDigest, err = ms.Put(ctx, sm); err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	exists, err := ms.Exists(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("unexpected error checking manifest existence: %#v", err)
	}

	if !exists {
		t.Fatalf("manifest should exist")
	}

	fromStore, err := ms.Get(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	fetchedManifest, ok := fromStore.(*schema1.SignedManifest)
	if !ok {
		t.Fatalf("unexpected manifest type from signedstore")
	}

	if !reflect.DeepEqual(fetchedManifest, sm) {
		t.Fatalf("fetched manifest not equal: %#v != %#v", fetchedManifest, sm)
	}

	_, pl, err := fetchedManifest.Payload()
	if err != nil {
		t.Fatalf("error getting payload %#v", err)
	}

	fetchedJWS, err := libtrust.ParsePrettySignature(pl, "signatures")
	if err != nil {
		t.Fatalf("unexpected error parsing jws: %v", err)
	}

	payload, err := fetchedJWS.Payload()
	if err != nil {
		t.Fatalf("unexpected error extracting payload: %v", err)
	}

	// Now that we have a payload, take a moment to check that the manifest is
	// return by the payload digest.

	dgst := digest.FromBytes(payload)
	exists, err = ms.Exists(ctx, dgst)
	if err != nil {
		t.Fatalf("error checking manifest existence by digest: %v", err)
	}

	if !exists {
		t.Fatalf("manifest %s should exist", dgst)
	}

	fetchedByDigest, err := ms.Get(ctx, dgst)
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

	// Now, push the same manifest with a different key
	pk2, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	sm2, err := schema1.Sign(&m, pk2)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}
	_, pl, err = sm2.Payload()
	if err != nil {
		t.Fatalf("error getting payload %#v", err)
	}

	jws2, err := libtrust.ParsePrettySignature(pl, "signatures")
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

	if manifestDigest, err = ms.Put(ctx, sm2); err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	fromStore, err = ms.Get(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	fetched, ok := fromStore.(*schema1.SignedManifest)
	if !ok {
		t.Fatalf("unexpected type from signed manifeststore : %T", fetched)
	}

	if _, err := schema1.Verify(fetched); err != nil {
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

	_, pl, err = fetched.Payload()
	if err != nil {
		t.Fatalf("error getting payload %#v", err)
	}

	receivedJWS, err := libtrust.ParsePrettySignature(pl, "signatures")
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

	// Test deleting manifests
	err = ms.Delete(ctx, dgst)
	if err != nil {
		t.Fatalf("unexpected an error deleting manifest by digest: %v", err)
	}

	exists, err = ms.Exists(ctx, dgst)
	if err != nil {
		t.Fatalf("Error querying manifest existence")
	}
	if exists {
		t.Errorf("Deleted manifest should not exist")
	}

	deletedManifest, err := ms.Get(ctx, dgst)
	if err == nil {
		t.Errorf("Unexpected success getting deleted manifest")
	}
	switch err.(type) {
	case distribution.ErrManifestUnknownRevision:
		break
	default:
		t.Errorf("Unexpected error getting deleted manifest: %s", reflect.ValueOf(err).Type())
	}

	if deletedManifest != nil {
		t.Errorf("Deleted manifest get returned non-nil")
	}

	// Re-upload should restore manifest to a good state
	_, err = ms.Put(ctx, sm)
	if err != nil {
		t.Errorf("Error re-uploading deleted manifest")
	}

	exists, err = ms.Exists(ctx, dgst)
	if err != nil {
		t.Fatalf("Error querying manifest existence")
	}
	if !exists {
		t.Errorf("Restored manifest should exist")
	}

	deletedManifest, err = ms.Get(ctx, dgst)
	if err != nil {
		t.Errorf("Unexpected error getting manifest")
	}
	if deletedManifest == nil {
		t.Errorf("Deleted manifest get returned non-nil")
	}

	r, err := NewRegistry(ctx, env.driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repo, err := r.Repository(ctx, env.name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	ms, err = repo.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = ms.Delete(ctx, dgst)
	if err == nil {
		t.Errorf("Unexpected success deleting while disabled")
	}
}

// TestLinkPathFuncs ensures that the link path functions behavior are locked
// down and implemented as expected.
func TestLinkPathFuncs(t *testing.T) {
	for _, testcase := range []struct {
		repo       string
		digest     digest.Digest
		linkPathFn linkPathFunc
		expected   string
	}{
		{
			repo:       "foo/bar",
			digest:     "sha256:deadbeaf98fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			linkPathFn: blobLinkPath,
			expected:   "/docker/registry/v2/repositories/foo/bar/_layers/sha256/deadbeaf98fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855/link",
		},
		{
			repo:       "foo/bar",
			digest:     "sha256:deadbeaf98fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			linkPathFn: manifestRevisionLinkPath,
			expected:   "/docker/registry/v2/repositories/foo/bar/_manifests/revisions/sha256/deadbeaf98fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855/link",
		},
	} {
		p, err := testcase.linkPathFn(testcase.repo, testcase.digest)
		if err != nil {
			t.Fatalf("unexpected error calling linkPathFn(pm, %q, %q): %v", testcase.repo, testcase.digest, err)
		}

		if p != testcase.expected {
			t.Fatalf("incorrect path returned: %q != %q", p, testcase.expected)
		}
	}

}

func makeManifestStorageForEnumeration(t *testing.T, repoName string) (*manifestStoreTestEnv, map[string]digest.Digest) {
	tags := []string{"without", "distribution", "no", "fun", "with", "docker"}
	imageName, err := reference.ParseNamed(repoName)
	if err != nil {
		t.Fatalf("failed to parse repoName %q: %v", repoName, err)
	}
	env := newManifestStoreTestEnv(t, imageName, tags...)
	ms, err := env.repository.Manifests(env.ctx)
	if err != nil {
		t.Fatal(err)
	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	res := make(map[string]digest.Digest)

	for _, tag := range tags {
		m := schema1.Manifest{
			Versioned: manifest.Versioned{
				SchemaVersion: 1,
			},
			Name: repoName,
			Tag:  tag,
		}

		sm, merr := schema1.Sign(&m, pk)
		if merr != nil {
			t.Fatalf("error signing manifest: %v", err)
		}

		dgst, err := ms.Put(env.ctx, sm)
		if err != nil {
			t.Fatalf("expected errors putting manifest with full verification")
		}

		exists, err := ms.Exists(env.ctx, dgst)
		if err != nil {
			t.Fatalf("unexpected error checking manifest existence: %v", err)
		}
		if !exists {
			t.Fatalf("manifest revision for %s:%s must exist", repoName, tag)
		}

		t.Logf("stored manifest %s:%s with digest %q", repoName, tag, dgst)

		res[tag] = dgst
	}

	return env, res
}

func testEnumerateManifestRevisions(t *testing.T, env *manifestStoreTestEnv, ms distribution.ManifestService, validDigests []digest.Digest, orphanedDigests []digest.Digest) {
	alltagset := make(map[string]struct{})
	for _, t := range env.tags {
		alltagset[t] = struct{}{}
	}

	validset := make(map[digest.Digest]struct{})
	for _, dgst := range validDigests {
		validset[dgst] = struct{}{}
	}
	orphanedset := make(map[digest.Digest]struct{})
	for _, dgst := range orphanedDigests {
		orphanedset[dgst] = struct{}{}
	}

	manifests := []digest.Digest{}
	tags := make(map[string]struct{})
	dgsts := make(map[digest.Digest]struct{})

	err := EnumerateManifestRevisions(ms, env.ctx, func(dgst digest.Digest, m distribution.Manifest, err error) error {
		manifests = append(manifests, dgst)
		if m == nil {
			if _, exists := orphanedset[dgst]; !exists {
				t.Errorf("Got unexpected orphaned manifest %q", dgst.String())
			}
			if _, ok := err.(distribution.ErrManifestUnknownRevision); !ok {
				t.Errorf("Got unexpected error for orphaned manifest %q: %v", dgst.String(), err)
			}

		} else {
			switch mt := m.(type) {
			case *schema1.SignedManifest:
				if _, exists := alltagset[mt.Tag]; !exists {
					t.Fatalf("Received manifest with unexpected tag %q", mt.Tag)
				}
				if _, exists := tags[mt.Tag]; exists {
					t.Errorf("Received duplicate manifest %s:%s", mt.Name, mt.Tag)
				}
				tags[mt.Tag] = struct{}{}

				dgst = digest.FromBytes(mt.Canonical)
				if _, exists := validset[dgst]; !exists {
					t.Errorf("Got unexpected valid manifest %q", dgst.String())
				}

			default:
				t.Errorf("Got unexpected manifest type %T", m)
			}
		}

		if _, exists := dgsts[dgst]; exists {
			t.Errorf("Received duplicate manifest reference %q", dgst.String())
		} else {
			dgsts[dgst] = struct{}{}
		}
		return nil
	})
	if err != io.EOF {
		t.Errorf("Unexpected error enumerating manifest revisions: %v", err)
	}
	if len(manifests) != len(validDigests)+len(orphanedDigests) {
		t.Errorf("Unexpected number of enumerated revisions (%d != %d)", len(manifests), len(env.tags))
	}
}

func TestManifestStorageEnumerateValid(t *testing.T) {
	env, dgstForTag := makeManifestStorageForEnumeration(t, "foo/valid")

	// valid revisions are enumerated by default
	ms, err := env.repository.Manifests(env.ctx)
	if err != nil {
		t.Fatal(err)
	}

	digests := make([]digest.Digest, 0, len(env.tags))
	for _, t := range env.tags {
		digests = append(digests, dgstForTag[t])
	}
	testEnumerateManifestRevisions(t, env, ms, digests, []digest.Digest{})

	for i := 0; i < len(digests); i++ {
		err := ms.Delete(env.ctx, digests[i])
		if err != nil {
			t.Fatalf("Failed to delete manifest %s@%s: %v", env.name, digests[i].String(), err)
		}
		t.Logf("deleted manifest %s@%s", env.name, digests[i])
		testEnumerateManifestRevisions(t, env, ms, digests[i+1:], []digest.Digest{})
	}
}

func TestManifestStorageEnumerateOrphaned(t *testing.T) {
	env, dgstForTag := makeManifestStorageForEnumeration(t, "foo/orphaned")

	ms, err := env.repository.Manifests(env.ctx, EnumerateOrphanedManifestRevisions())
	if err != nil {
		t.Fatal(err)
	}

	digests := make([]digest.Digest, 0, len(env.tags))
	for _, t := range env.tags {
		digests = append(digests, dgstForTag[t])
	}
	testEnumerateManifestRevisions(t, env, ms, []digest.Digest{}, digests[:0])

	for i := 0; i < len(digests); i++ {
		err := ms.Delete(env.ctx, digests[i])
		if err != nil {
			t.Fatalf("Failed to delete manifest %s@%s", env.name, digests[i].String())
		}
		t.Logf("deleted manifest %s@%s", env.name, digests[i])
		testEnumerateManifestRevisions(t, env, ms, []digest.Digest{}, digests[:i+1])
	}
}

func TestManifestStorageEnumerateAll(t *testing.T) {
	env, dgstForTag := makeManifestStorageForEnumeration(t, "foo/all")

	ms, err := env.repository.Manifests(env.ctx, EnumerateAllManifestRevisions())
	if err != nil {
		t.Fatal(err)
	}

	digests := make([]digest.Digest, 0, len(env.tags))
	for _, t := range env.tags {
		digests = append(digests, dgstForTag[t])
	}
	testEnumerateManifestRevisions(t, env, ms, digests, digests[:0])

	for i := 0; i < len(digests); i++ {
		err := ms.Delete(env.ctx, digests[i])
		if err != nil {
			t.Fatalf("Failed to delete manifest %s@%s", env.name, env.tags[i])
		}
		t.Logf("deleted manifest %s@%s", env.name, digests[i])
		testEnumerateManifestRevisions(t, env, ms, digests[i+1:], digests[:i+1])
	}
}

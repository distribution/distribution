package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type manifestStoreTestEnv struct {
	ctx        context.Context
	driver     driver.StorageDriver
	registry   distribution.Namespace
	repository distribution.Repository
	name       reference.Named
	tag        string
}

func newManifestStoreTestEnv(t *testing.T, name reference.Named, tag string, options ...RegistryOption) *manifestStoreTestEnv {
	ctx := context.Background()
	drvr := inmemory.New()
	registry, err := NewRegistry(ctx, drvr, options...)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

	repo, err := registry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	return &manifestStoreTestEnv{
		ctx:        ctx,
		driver:     drvr,
		registry:   registry,
		repository: repo,
		name:       name,
		tag:        tag,
	}
}

func TestManifestStorage(t *testing.T) {
	testManifestStorage(t, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete, EnableRedirect, EnableValidateImageIndexImagesExist)
}

func testManifestStorage(t *testing.T, options ...RegistryOption) {
	repoName, _ := reference.WithName("foo/bar")
	env := newManifestStoreTestEnv(t, repoName, "thetag", options...)
	ctx := context.Background()
	ms, err := env.repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Push a config, and reference it in the manifest
	sampleConfig := []byte(`{
		"architecture": "amd64",
		"history": [
		  {
		    "created": "2015-10-31T22:22:54.690851953Z",
		    "created_by": "/bin/sh -c #(nop) ADD file:a3bc1e842b69636f9df5256c49c5374fb4eef1e281fe3f282c65fb853ee171c5 in /"
		  },
		],
		"rootfs": {
		  "diff_ids": [
		    "sha256:c6f988f4874bb0add23a778f753c65efe992244e148a1d2ec2a8b664fb66bbd1",
		  ],
		  "type": "layers"
		}
	}`)

	// Build a manifest and store it and its layers in the registry

	blobStore := env.repository.Blobs(ctx)
	d, err := blobStore.Put(ctx, schema2.MediaTypeImageConfig, sampleConfig)
	if err != nil {
		t.Fatal(err)
	}
	builder := schema2.NewManifestBuilder(d, sampleConfig)

	m := &schema2.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: schema2.MediaTypeManifest,
		Config: v1.Descriptor{
			Digest:    digest.FromBytes(sampleConfig),
			Size:      int64(len(sampleConfig)),
			MediaType: schema2.MediaTypeImageConfig,
		},
		Layers: []v1.Descriptor{},
	}

	// Build up some test layers and add them to the manifest, saving the
	// readseekers for upload later.
	testLayers := map[digest.Digest]io.ReadSeeker{}
	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatal("unexpected error generating test layer file")
		}

		testLayers[dgst] = rs
		layer := v1.Descriptor{
			Digest:    dgst,
			Size:      6323,
			MediaType: schema2.MediaTypeLayer,
		}
		m.Layers = append(m.Layers, layer)
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

		if _, err := wr.Commit(env.ctx, v1.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
		if err := builder.AppendReference(v1.Descriptor{Digest: dgst, MediaType: schema2.MediaTypeLayer}); err != nil {
			t.Fatalf("unexpected error appending references: %v", err)
		}
	}

	sm, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("%s: unexpected error generating manifest: %v", repoName, err)
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
		t.Fatal("manifest should exist")
	}

	fromStore, err := ms.Get(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	fetchedManifest, ok := fromStore.(*schema2.DeserializedManifest)
	if !ok {
		t.Fatal("unexpected manifest type from signedstore")
	}
	_, pl, err := fetchedManifest.Payload()
	if err != nil {
		t.Fatalf("could not get manifest payload: %v", err)
	}

	// Now that we have a payload, take a moment to check that the manifest is
	// return by the payload digest.

	dgst := digest.FromBytes(pl)
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

	byDigestManifest, ok := fetchedByDigest.(*schema2.DeserializedManifest)
	if !ok {
		t.Fatal("unexpected manifest type from signedstore")
	}

	_, byDigestCanonical, err := byDigestManifest.Payload()
	if err != nil {
		t.Fatalf("could not get manifest payload: %v", err)
	}

	if !bytes.Equal(byDigestCanonical, pl) {
		t.Fatalf("fetched manifest not equal: %q != %q", byDigestCanonical, pl)
	}

	fromStore, err = ms.Get(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	fetched, ok := fromStore.(*schema2.DeserializedManifest)
	if !ok {
		t.Fatalf("unexpected type from signed manifeststore : %T", fetched)
	}

	_, receivedPL, err := fetched.Payload()
	if err != nil {
		t.Fatalf("error getting payload %#v", err)
	}

	if !bytes.Equal(receivedPL, pl) {
		t.Fatal("payloads are not equal")
	}

	// Test deleting manifests
	err = ms.Delete(ctx, dgst)
	if err != nil {
		t.Fatalf("unexpected an error deleting manifest by digest: %v", err)
	}

	exists, err = ms.Exists(ctx, dgst)
	if err != nil {
		t.Fatal("Error querying manifest existence")
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
		t.Fatal("Error querying manifest existence")
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

	r, err := NewRegistry(ctx, env.driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableRedirect)
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

func TestOCIManifestStorage(t *testing.T) {
	testOCIManifestStorage(t, "includeMediaTypes=true", true)
	testOCIManifestStorage(t, "includeMediaTypes=false", false)
}

func testOCIManifestStorage(t *testing.T, testname string, includeMediaTypes bool) {
	var imageMediaType string
	var indexMediaType string
	if includeMediaTypes {
		imageMediaType = v1.MediaTypeImageManifest
		indexMediaType = v1.MediaTypeImageIndex
	} else {
		imageMediaType = ""
		indexMediaType = ""
	}

	repoName, _ := reference.WithName("foo/bar")
	env := newManifestStoreTestEnv(t, repoName, "thetag",
		BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)),
		EnableDelete, EnableRedirect, EnableValidateImageIndexImagesExist)

	ctx := context.Background()
	ms, err := env.repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Build a manifest and store its layers in the registry

	blobStore := env.repository.Blobs(ctx)
	mfst, err := createRandomImage(t, testname, imageMediaType, blobStore)
	if err != nil {
		t.Fatalf("%s: unexpected error generating random image: %v", testname, err)
	}

	// create an image index

	platformSpec := &v1.Platform{
		Architecture: "atari2600",
		OS:           "CP/M",
	}

	mfstDescriptors := []v1.Descriptor{
		createOciManifestDescriptor(t, testname, mfst, platformSpec),
	}

	imageIndex, err := ociIndexFromDesriptorsWithMediaType(mfstDescriptors, indexMediaType)
	if err != nil {
		t.Fatalf("%s: unexpected error creating image index: %v", testname, err)
	}

	_, err = ms.Put(ctx, imageIndex)
	if err == nil {
		t.Fatalf("%s: expected error putting image index without child manifests in the registry: %v", testname, err)
	}

	// Test for proper handling of SchemaVersion for the image

	if mfst.(*ocischema.DeserializedManifest).Manifest.SchemaVersion != 2 {
		t.Fatalf("%s: unexpected error generating default version for oci manifest", testname)
	}
	mfst.(*ocischema.DeserializedManifest).Manifest.SchemaVersion = 0

	var manifestDigest digest.Digest
	if manifestDigest, err = ms.Put(ctx, mfst); err != nil {
		if err.Error() != "unrecognized manifest schema version 0" {
			t.Fatalf("%s: unexpected error putting manifest: %v", testname, err)
		}
		mfst.(*ocischema.DeserializedManifest).Manifest.SchemaVersion = 2
		if manifestDigest, err = ms.Put(ctx, mfst); err != nil {
			t.Fatalf("%s: unexpected error putting manifest: %v", testname, err)
		}
	}

	// We can now push the index

	var indexDigest digest.Digest
	if indexDigest, err = ms.Put(ctx, imageIndex); err != nil {
		t.Fatalf("%s: unexpected error putting image index: %v", testname, err)
	}

	// Now check that we can retrieve the manifest

	fromStore, err := ms.Get(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("%s: unexpected error fetching manifest: %v", testname, err)
	}

	fetchedManifest, ok := fromStore.(*ocischema.DeserializedManifest)
	if !ok {
		t.Fatalf("%s: unexpected type for fetched manifest", testname)
	}

	if fetchedManifest.MediaType != imageMediaType {
		t.Fatalf("%s: unexpected MediaType for result, %s", testname, fetchedManifest.MediaType)
	}

	if fetchedManifest.SchemaVersion != 2 {
		t.Fatalf("%s: unexpected schema version for result, %d", testname, fetchedManifest.SchemaVersion)
	}

	payloadMediaType, _, err := fromStore.Payload()
	if err != nil {
		t.Fatalf("%s: error getting payload %v", testname, err)
	}

	if payloadMediaType != v1.MediaTypeImageManifest {
		t.Fatalf("%s: unexpected MediaType for manifest payload, %s", testname, payloadMediaType)
	}

	// and the image index

	fromStore, err = ms.Get(ctx, indexDigest)
	if err != nil {
		t.Fatalf("%s: unexpected error fetching image index: %v", testname, err)
	}

	fetchedIndex, ok := fromStore.(*ocischema.DeserializedImageIndex)
	if !ok {
		t.Fatalf("%s: unexpected type for fetched manifest", testname)
	}

	if fetchedIndex.MediaType != indexMediaType {
		t.Fatalf("%s: unexpected MediaType for result, %s", testname, fetchedIndex.MediaType)
	}

	payloadMediaType, _, err = fromStore.Payload()
	if err != nil {
		t.Fatalf("%s: error getting payload %v", testname, err)
	}

	if payloadMediaType != v1.MediaTypeImageIndex {
		t.Fatalf("%s: unexpected MediaType for index payload, %s", testname, payloadMediaType)
	}
}

func TestIndexManifestStorageWithoutImageCheck(t *testing.T) {
	imageMediaType := v1.MediaTypeImageManifest
	indexMediaType := v1.MediaTypeImageIndex

	repoName, _ := reference.WithName("foo/bar")
	env := newManifestStoreTestEnv(t, repoName, "thetag",
		BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)),
		EnableDelete, EnableRedirect)

	ctx := context.Background()
	ms, err := env.repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Build a manifest and store its layers in the registry

	blobStore := env.repository.Blobs(ctx)
	manifest, err := createRandomImage(t, t.Name(), imageMediaType, blobStore)
	if err != nil {
		t.Fatalf("unexpected error generating random image: %v", err)
	}

	// create an image index

	ociPlatformSpec := &v1.Platform{
		Architecture: "atari2600",
		OS:           "CP/M",
	}

	ociManifestDescriptors := []v1.Descriptor{
		createOciManifestDescriptor(t, t.Name(), manifest, ociPlatformSpec),
	}

	imageIndex, err := ociIndexFromDesriptorsWithMediaType(ociManifestDescriptors, indexMediaType)
	if err != nil {
		t.Fatalf("unexpected error creating image index: %v", err)
	}

	// We should be able to put the index without having put the image

	_, err = ms.Put(ctx, imageIndex)
	if err != nil {
		t.Fatalf("unexpected error putting sparse OCI image index: %v", err)
	}

	// same for a manifest list

	listPlatformSpec := &manifestlist.PlatformSpec{
		Architecture: "atari2600",
		OS:           "CP/M",
	}

	listManifestDescriptors := []manifestlist.ManifestDescriptor{
		createManifestListDescriptor(t, t.Name(), manifest, listPlatformSpec),
	}

	list, err := manifestlist.FromDescriptors(listManifestDescriptors)
	if err != nil {
		t.Fatalf("unexpected error creating manifest list: %v", err)
	}

	// We should be able to put the list without having put the image

	_, err = ms.Put(ctx, list)
	if err != nil {
		t.Fatalf("unexpected error putting sparse manifest list: %v", err)
	}
}

func TestIndexManifestStorageWithSelectivePlatforms(t *testing.T) {
	imageMediaType := v1.MediaTypeImageManifest
	indexMediaType := v1.MediaTypeImageIndex

	repoName, _ := reference.WithName("foo/bar")
	env := newManifestStoreTestEnv(t, repoName, "thetag",
		BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)),
		EnableDelete, EnableRedirect, EnableValidateImageIndexImagesExist,
		AddValidateImageIndexImagesExistPlatform("amd64", "linux"))

	ctx := context.Background()
	ms, err := env.repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Build a manifests their layers in the registry

	blobStore := env.repository.Blobs(ctx)
	amdManifest, err := createRandomImage(t, t.Name(), imageMediaType, blobStore)
	if err != nil {
		t.Fatalf("%s: unexpected error generating random image: %v", t.Name(), err)
	}
	armManifest, err := createRandomImage(t, t.Name(), imageMediaType, blobStore)
	if err != nil {
		t.Fatalf("%s: unexpected error generating random image: %v", t.Name(), err)
	}
	atariManifest, err := createRandomImage(t, t.Name(), imageMediaType, blobStore)
	if err != nil {
		t.Fatalf("%s: unexpected error generating random image: %v", t.Name(), err)
	}

	// create an image index

	amdPlatformSpec := &v1.Platform{
		Architecture: "amd64",
		OS:           "linux",
	}
	armPlatformSpec := &v1.Platform{
		Architecture: "arm",
		OS:           "plan9",
	}
	atariPlatformSpec := &v1.Platform{
		Architecture: "atari2600",
		OS:           "CP/M",
	}

	manifestDescriptors := []v1.Descriptor{
		createOciManifestDescriptor(t, t.Name(), amdManifest, amdPlatformSpec),
		createOciManifestDescriptor(t, t.Name(), armManifest, armPlatformSpec),
		createOciManifestDescriptor(t, t.Name(), atariManifest, atariPlatformSpec),
	}

	imageIndex, err := ociIndexFromDesriptorsWithMediaType(manifestDescriptors, indexMediaType)
	if err != nil {
		t.Fatalf("unexpected error creating image index: %v", err)
	}

	// Test we can't push with no image manifests existing in the registry

	_, err = ms.Put(ctx, imageIndex)
	if err == nil {
		t.Fatalf("expected error putting image index without existing images: %v", err)
	}

	// Test we can't push with a manifest but not the right one

	_, err = ms.Put(ctx, atariManifest)
	if err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	_, err = ms.Put(ctx, imageIndex)
	if err == nil {
		t.Fatalf("expected error putting image index without correct existing images: %v", err)
	}

	// Test we can push with the right manifest

	_, err = ms.Put(ctx, amdManifest)
	if err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	_, err = ms.Put(ctx, imageIndex)
	if err != nil {
		t.Fatalf("unexpected error putting image index: %v", err)
	}
}

// createRandomImage builds an image manifest and store it and its layers in the registry
func createRandomImage(t *testing.T, testname string, imageMediaType string, blobStore distribution.BlobStore) (distribution.Manifest, error) {
	builder := ocischema.NewManifestBuilder(blobStore, []byte{}, map[string]string{})
	err := builder.SetMediaType(imageMediaType)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Add some layers
	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("%s: unexpected error generating test layer file", testname)
		}

		wr, err := blobStore.Create(ctx)
		if err != nil {
			t.Fatalf("%s: unexpected error creating test upload: %v", testname, err)
		}

		if _, err := io.Copy(wr, rs); err != nil {
			t.Fatalf("%s: unexpected error copying to upload: %v", testname, err)
		}

		if _, err := wr.Commit(ctx, v1.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("%s: unexpected error finishing upload: %v", testname, err)
		}

		if err := builder.AppendReference(v1.Descriptor{Digest: dgst, MediaType: v1.MediaTypeImageLayer}); err != nil {
			t.Fatalf("%s unexpected error appending references: %v", testname, err)
		}
	}

	return builder.Build(ctx)
}

// createOciManifestDescriptor builds a manifest descriptor from a manifest and a platform descriptor
func createOciManifestDescriptor(t *testing.T, testname string, manifest distribution.Manifest, platformSpec *v1.Platform) v1.Descriptor {
	manifestMediaType, manifestPayload, err := manifest.Payload()
	if err != nil {
		t.Fatalf("%s: unexpected error getting manifest payload: %v", testname, err)
	}
	manifestDigest := digest.FromBytes(manifestPayload)

	return v1.Descriptor{
		Digest:    manifestDigest,
		Size:      int64(len(manifestPayload)),
		MediaType: manifestMediaType,
		Platform: &v1.Platform{
			Architecture: platformSpec.Architecture,
			OS:           platformSpec.OS,
		},
	}
}

// createManifestListDescriptor builds a manifest descriptor from a manifest and a platform descriptor
func createManifestListDescriptor(t *testing.T, testname string, manifest distribution.Manifest, platformSpec *manifestlist.PlatformSpec) manifestlist.ManifestDescriptor {
	manifestMediaType, manifestPayload, err := manifest.Payload()
	if err != nil {
		t.Fatalf("%s: unexpected error getting manifest payload: %v", testname, err)
	}
	manifestDigest := digest.FromBytes(manifestPayload)

	return manifestlist.ManifestDescriptor{
		Descriptor: v1.Descriptor{
			Digest:    manifestDigest,
			Size:      int64(len(manifestPayload)),
			MediaType: manifestMediaType,
		},
		Platform: manifestlist.PlatformSpec{
			Architecture: platformSpec.Architecture,
			OS:           platformSpec.OS,
		},
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

func ociIndexFromDesriptorsWithMediaType(descriptors []v1.Descriptor, mediaType string) (*ocischema.DeserializedImageIndex, error) {
	manifest, err := ocischema.FromDescriptors(descriptors, nil)
	if err != nil {
		return nil, err
	}
	manifest.ImageIndex.MediaType = mediaType

	rawManifest, err := json.Marshal(manifest.ImageIndex)
	if err != nil {
		return nil, err
	}

	var d ocischema.DeserializedImageIndex
	if err := d.UnmarshalJSON(rawManifest); err != nil {
		return nil, err
	}

	return &d, nil
}

func TestManifestStorageCache(t *testing.T) {
	repoName, _ := reference.WithName("foo/bar")
	cacheProvider := memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)
	repositoryScopedCacheProvider, err := cacheProvider.RepositoryScoped(repoName.String())
	if err != nil {
		t.Fatalf("unexpected error getting repository scoped cache: %v", err)
	}
	env := newManifestStoreTestEnv(t, repoName, "thetag", BlobDescriptorCacheProvider(cacheProvider), EnableDelete)
	ctx := context.Background()
	ms, err := env.repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Push a config, and reference it in the manifest
	sampleConfig := []byte(`{
		"architecture": "amd64",
		"history": [
		  {
		    "created": "2015-10-31T22:22:54.690851953Z",
		    "created_by": "/bin/sh -c #(nop) ADD file:a3bc1e842b69636f9df5256c49c5374fb4eef1e281fe3f282c65fb853ee171c5 in /"
		  },
		],
		"rootfs": {
		  "diff_ids": [
		    "sha256:c6f988f4874bb0add23a778f753c65efe992244e148a1d2ec2a8b664fb66bbd1",
		  ],
		  "type": "layers"
		}
	}`)

	// Build a manifest and store it and its layers in the registry

	blobStore := env.repository.Blobs(ctx)
	d, err := blobStore.Put(ctx, schema2.MediaTypeImageConfig, sampleConfig)
	if err != nil {
		t.Fatal(err)
	}
	builder := schema2.NewManifestBuilder(d, sampleConfig)

	m := &schema2.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: schema2.MediaTypeManifest,
		Config: v1.Descriptor{
			Digest:    digest.FromBytes(sampleConfig),
			Size:      int64(len(sampleConfig)),
			MediaType: schema2.MediaTypeImageConfig,
		},
		Layers: []v1.Descriptor{},
	}

	// Build up some test layers and add them to the manifest, saving the
	// readseekers for upload later.
	testLayers := map[digest.Digest]io.ReadSeeker{}
	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatal("unexpected error generating test layer file")
		}

		testLayers[dgst] = rs
		layer := v1.Descriptor{
			Digest:    dgst,
			Size:      6323,
			MediaType: schema2.MediaTypeLayer,
		}
		m.Layers = append(m.Layers, layer)
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

		if _, err := wr.Commit(env.ctx, v1.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
		if err := builder.AppendReference(v1.Descriptor{Digest: dgst, MediaType: schema2.MediaTypeLayer}); err != nil {
			t.Fatalf("unexpected error appending references: %v", err)
		}
	}

	sm, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("%s: unexpected error generating manifest: %v", repoName, err)
	}

	var manifestDigest digest.Digest
	if manifestDigest, err = ms.Put(ctx, sm); err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	_, err = repositoryScopedCacheProvider.Stat(ctx, manifestDigest)
	if err != nil {
		t.Errorf("Manifest should be cached")
	}

	exists, err := ms.Exists(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("unexpected error checking manifest existence: %#v", err)
	}

	if !exists {
		t.Fatal("manifest should exist")
	}

	// Test deleting manifests
	err = ms.Delete(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("unexpected an error deleting manifest by digest: %v", err)
	}

	exists, err = ms.Exists(ctx, manifestDigest)
	if err != nil {
		t.Fatal("Error querying manifest existence")
	}
	if exists {
		t.Errorf("Deleted manifest should not exist")
	}

	deletedManifest, err := ms.Get(ctx, manifestDigest)
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

	// Test cache is cleared after manifest delete
	_, err = repositoryScopedCacheProvider.Stat(ctx, manifestDigest)
	if err == nil {
		t.Errorf("Unexpected success getting deleted manifest")
	}
	if !errors.Is(err, distribution.ErrBlobUnknown) {
		t.Errorf("Unexpected error getting cached manifest: %v", err)
	}
}

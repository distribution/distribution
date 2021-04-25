package storage

import (
	"io"
	"path"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/docker/libtrust"
	"github.com/opencontainers/go-digest"
)

type image struct {
	manifest       distribution.Manifest
	manifestDigest digest.Digest
	layers         map[digest.Digest]io.ReadSeeker
}

func createRegistry(t *testing.T, driver driver.StorageDriver, options ...RegistryOption) distribution.Namespace {
	ctx := context.Background()
	k, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	options = append([]RegistryOption{EnableDelete, Schema1SigningKey(k), EnableSchema1}, options...)
	registry, err := NewRegistry(ctx, driver, options...)
	if err != nil {
		t.Fatalf("Failed to construct namespace")
	}
	return registry
}

func makeRepository(t *testing.T, registry distribution.Namespace, name string) distribution.Repository {
	ctx := context.Background()

	// Initialize a dummy repository
	named, err := reference.WithName(name)
	if err != nil {
		t.Fatalf("Failed to parse name %s:  %v", name, err)
	}

	repo, err := registry.Repository(ctx, named)
	if err != nil {
		t.Fatalf("Failed to construct repository: %v", err)
	}
	return repo
}

func makeManifestService(t *testing.T, repository distribution.Repository) distribution.ManifestService {
	ctx := context.Background()

	manifestService, err := repository.Manifests(ctx)
	if err != nil {
		t.Fatalf("Failed to construct manifest store: %v", err)
	}
	return manifestService
}

func allManifests(t *testing.T, manifestService distribution.ManifestService) map[digest.Digest]struct{} {
	ctx := context.Background()
	allManMap := make(map[digest.Digest]struct{})
	manifestEnumerator, ok := manifestService.(distribution.ManifestEnumerator)
	if !ok {
		t.Fatalf("unable to convert ManifestService into ManifestEnumerator")
	}
	err := manifestEnumerator.Enumerate(ctx, func(dgst digest.Digest) error {
		allManMap[dgst] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Error getting all manifests: %v", err)
	}
	return allManMap
}

func allBlobs(t *testing.T, registry distribution.Namespace) map[digest.Digest]struct{} {
	ctx := context.Background()
	blobService := registry.Blobs()
	allBlobsMap := make(map[digest.Digest]struct{})
	err := blobService.Enumerate(ctx, func(dgst digest.Digest) error {
		allBlobsMap[dgst] = struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Error getting all blobs: %v", err)
	}
	return allBlobsMap
}

func uploadImage(t *testing.T, repository distribution.Repository, im image) digest.Digest {
	// upload layers
	err := testutil.UploadBlobs(repository, im.layers)
	if err != nil {
		t.Fatalf("layer upload failed: %v", err)
	}

	// upload manifest
	ctx := context.Background()
	manifestService := makeManifestService(t, repository)
	manifestDigest, err := manifestService.Put(ctx, im.manifest)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	return manifestDigest
}

func uploadRandomSchema1Image(t *testing.T, repository distribution.Repository) image {
	randomLayers, err := testutil.CreateRandomLayers(2)
	if err != nil {
		t.Fatalf("%v", err)
	}

	digests := []digest.Digest{}
	for digest := range randomLayers {
		digests = append(digests, digest)
	}

	manifest, err := testutil.MakeSchema1Manifest(digests)
	if err != nil {
		t.Fatalf("%v", err)
	}

	manifestDigest := uploadImage(t, repository, image{manifest: manifest, layers: randomLayers})
	return image{
		manifest:       manifest,
		manifestDigest: manifestDigest,
		layers:         randomLayers,
	}
}

func uploadRandomSchema2Image(t *testing.T, repository distribution.Repository) image {
	randomLayers, err := testutil.CreateRandomLayers(2)
	if err != nil {
		t.Fatalf("%v", err)
	}

	digests := []digest.Digest{}
	for digest := range randomLayers {
		digests = append(digests, digest)
	}

	manifest, err := testutil.MakeSchema2Manifest(repository, digests)
	if err != nil {
		t.Fatalf("%v", err)
	}

	manifestDigest := uploadImage(t, repository, image{manifest: manifest, layers: randomLayers})
	return image{
		manifest:       manifest,
		manifestDigest: manifestDigest,
		layers:         randomLayers,
	}
}

func TestNoDeletionNoEffect(t *testing.T) {
	ctx := context.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "palailogos")
	manifestService, _ := repo.Manifests(ctx)

	image1 := uploadRandomSchema1Image(t, repo)
	image2 := uploadRandomSchema1Image(t, repo)
	uploadRandomSchema2Image(t, repo)

	// construct manifestlist for fun.
	blobstatter := registry.BlobStatter()
	manifestList, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		image1.manifestDigest, image2.manifestDigest})
	if err != nil {
		t.Fatalf("Failed to make manifest list: %v", err)
	}

	_, err = manifestService.Put(ctx, manifestList)
	if err != nil {
		t.Fatalf("Failed to add manifest list: %v", err)
	}

	before := allBlobs(t, registry)

	// Run GC
	err = MarkAndSweep(context.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: false,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after := allBlobs(t, registry)
	if len(before) != len(after) {
		t.Fatalf("Garbage collection affected storage: %d != %d", len(before), len(after))
	}
}

func TestDeleteManifestIfTagNotFound(t *testing.T) {
	ctx := context.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "deletemanifests")
	manifestService, _ := repo.Manifests(ctx)

	// Create random layers
	randomLayers, err := testutil.CreateRandomLayers(12)
	if err != nil {
		t.Fatalf("failed to make layers: %v", err)
	}
	layerDigests := make([]digest.Digest, 0, len(randomLayers))
	for digest, _ := range randomLayers {
		layerDigests = append(layerDigests, digest)
	}

	// Upload all layers
	err = testutil.UploadBlobs(repo, randomLayers)
	if err != nil {
		t.Fatalf("failed to upload layers: %v", err)
	}

	putManifest := func(manifest distribution.Manifest, tag string) digest.Digest {
		digest, err := manifestService.Put(ctx, manifest)
		if err != nil {
			t.Fatalf("manifest upload failed: %v", err)
		}
		if tag != "" {
			repo.Tags(ctx).Tag(ctx, tag, distribution.Descriptor{Digest: digest})
		}
		return digest
	}

	// Construct and tag manifests
	manifest1, err := testutil.MakeSchema1Manifest(layerDigests[3:7])
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}
	manifest1Digest := putManifest(manifest1, "test")

	manifest2, err := testutil.MakeSchema1Manifest(layerDigests[0:4])
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}
	manifest2Digest := putManifest(manifest2, "test")

	subManifest1, err := testutil.MakeSchema1Manifest(layerDigests[6:8])
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}
	subManifest1Digest := putManifest(subManifest1, "")

	subManifest2, err := testutil.MakeSchema1Manifest(layerDigests[6:8])
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}
	subManifest2Digest := putManifest(subManifest2, "")

	subManifest3, err := testutil.MakeSchema1Manifest(layerDigests[10:12])
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}
	subManifest3Digest := putManifest(subManifest3, "")

	blobstatter := registry.BlobStatter()
	manifestList1, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		subManifest1Digest, subManifest2Digest})
	manifestList1Digest := putManifest(manifestList1, "mytag")

	manifestList2, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		subManifest3Digest, manifest1Digest})
	manifestList2Digest := putManifest(manifestList2, "")

	before1 := allBlobs(t, registry)
	before2 := allManifests(t, manifestService)

	// run GC with dry-run (should not remove anything)
	err = MarkAndSweep(context.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         true,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}
	afterDry1 := allBlobs(t, registry)
	afterDry2 := allManifests(t, manifestService)
	if len(before1) != len(afterDry1) {
		t.Fatalf("Garbage collection affected blobs storage: %d != %d", len(before1), len(afterDry1))
	}
	if len(before2) != len(afterDry2) {
		t.Fatalf("Garbage collection affected manifest storage: %d != %d", len(before2), len(afterDry2))
	}

	// Run GC (removes everything because no manifests with tags exist)
	err = MarkAndSweep(context.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after1 := allBlobs(t, registry)
	after2 := allManifests(t, manifestService)
	println(len(before1), len(after1), len(before2), len(after2))
	if len(before1) == len(after1) {
		t.Fatalf("Garbage collection did not affect blobs storage: %d == %d", len(before1), len(after1))
	}
	if len(before2) == len(after2) {
		t.Fatalf("Garbage collection did not affect manifest storage: %d == %d", len(before2), len(after2))
	}

	assertManifestExists := func(digest digest.Digest, expected bool, name string) {
		actual, err := manifestService.Exists(ctx, digest)
		if err != nil {
			t.Fatalf("manifest exist check failed for %s: %v", name, err)
		}
		if expected && !actual {
			t.Fatalf("expected manifest to still exist for %s", name)
		}
		if !expected && actual {
			t.Fatalf("expected manifest to no longer exist %s", name)
		}
	}

	assertManifestExists(manifest1Digest, false, "manifest1")
	assertManifestExists(manifest2Digest, true, "manifest2")
	assertManifestExists(subManifest1Digest, true, "subManifest1")
	assertManifestExists(subManifest2Digest, true, "subManifest2")
	assertManifestExists(subManifest3Digest, false, "subManifest3")
	assertManifestExists(manifestList1Digest, true, "manifestList1")
	assertManifestExists(manifestList2Digest, false, "manifestList2")
}

func TestGCWithMissingManifests(t *testing.T) {
	ctx := context.Background()
	d := inmemory.New()

	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "testrepo")
	uploadRandomSchema1Image(t, repo)

	// Simulate a missing _manifests directory
	revPath, err := pathFor(manifestRevisionsPathSpec{"testrepo"})
	if err != nil {
		t.Fatal(err)
	}

	_manifestsPath := path.Dir(revPath)
	err = d.Delete(ctx, _manifestsPath)
	if err != nil {
		t.Fatal(err)
	}

	err = MarkAndSweep(context.Background(), d, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: false,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	blobs := allBlobs(t, registry)
	if len(blobs) > 0 {
		t.Errorf("unexpected blobs after gc")
	}
}

func TestDeletionHasEffect(t *testing.T) {
	ctx := context.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "komnenos")
	manifests, _ := repo.Manifests(ctx)

	image1 := uploadRandomSchema1Image(t, repo)
	image2 := uploadRandomSchema1Image(t, repo)
	image3 := uploadRandomSchema2Image(t, repo)

	manifests.Delete(ctx, image2.manifestDigest)
	manifests.Delete(ctx, image3.manifestDigest)

	// Run GC
	err := MarkAndSweep(context.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: false,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	blobs := allBlobs(t, registry)

	// check that the image1 manifest and all the layers are still in blobs
	if _, ok := blobs[image1.manifestDigest]; !ok {
		t.Fatalf("First manifest is missing")
	}

	for layer := range image1.layers {
		if _, ok := blobs[layer]; !ok {
			t.Fatalf("manifest 1 layer is missing: %v", layer)
		}
	}

	// check that image2 and image3 layers are not still around
	for layer := range image2.layers {
		if _, ok := blobs[layer]; ok {
			t.Fatalf("manifest 2 layer is present: %v", layer)
		}
	}

	for layer := range image3.layers {
		if _, ok := blobs[layer]; ok {
			t.Fatalf("manifest 3 layer is present: %v", layer)
		}
	}
}

func getAnyKey(digests map[digest.Digest]io.ReadSeeker) (d digest.Digest) {
	for d = range digests {
		break
	}
	return
}

func getKeys(digests map[digest.Digest]io.ReadSeeker) (ds []digest.Digest) {
	for d := range digests {
		ds = append(ds, d)
	}
	return
}

func TestDeletionWithSharedLayer(t *testing.T) {
	ctx := context.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "tzimiskes")

	// Create random layers
	randomLayers1, err := testutil.CreateRandomLayers(3)
	if err != nil {
		t.Fatalf("failed to make layers: %v", err)
	}

	randomLayers2, err := testutil.CreateRandomLayers(3)
	if err != nil {
		t.Fatalf("failed to make layers: %v", err)
	}

	// Upload all layers
	err = testutil.UploadBlobs(repo, randomLayers1)
	if err != nil {
		t.Fatalf("failed to upload layers: %v", err)
	}

	err = testutil.UploadBlobs(repo, randomLayers2)
	if err != nil {
		t.Fatalf("failed to upload layers: %v", err)
	}

	// Construct manifests
	manifest1, err := testutil.MakeSchema1Manifest(getKeys(randomLayers1))
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}

	sharedKey := getAnyKey(randomLayers1)
	manifest2, err := testutil.MakeSchema2Manifest(repo, append(getKeys(randomLayers2), sharedKey))
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}

	manifestService := makeManifestService(t, repo)

	// Upload manifests
	_, err = manifestService.Put(ctx, manifest1)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	manifestDigest2, err := manifestService.Put(ctx, manifest2)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	// delete
	err = manifestService.Delete(ctx, manifestDigest2)
	if err != nil {
		t.Fatalf("manifest deletion failed: %v", err)
	}

	// check that all of the layers in layer 1 are still there
	blobs := allBlobs(t, registry)
	for dgst := range randomLayers1 {
		if _, ok := blobs[dgst]; !ok {
			t.Fatalf("random layer 1 blob missing: %v", dgst)
		}
	}
}

func TestOrphanBlobDeleted(t *testing.T) {
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "michael_z_doukas")

	digests, err := testutil.CreateRandomLayers(1)
	if err != nil {
		t.Fatalf("Failed to create random digest: %v", err)
	}

	if err = testutil.UploadBlobs(repo, digests); err != nil {
		t.Fatalf("Failed to upload blob: %v", err)
	}

	// formality to create the necessary directories
	uploadRandomSchema2Image(t, repo)

	// Run GC
	err = MarkAndSweep(context.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: false,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	blobs := allBlobs(t, registry)

	// check that orphan blob layers are not still around
	for dgst := range digests {
		if _, ok := blobs[dgst]; ok {
			t.Fatalf("Orphan layer is present: %v", dgst)
		}
	}
}

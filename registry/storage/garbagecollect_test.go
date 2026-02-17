package storage

import (
	"encoding/json"
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type image struct {
	manifest       distribution.Manifest
	manifestDigest digest.Digest
	layers         map[digest.Digest]io.ReadSeeker
}

func createRegistry(t *testing.T, driver driver.StorageDriver, options ...RegistryOption) distribution.Namespace {
	ctx := dcontext.Background()
	options = append(options, EnableDelete)
	registry, err := NewRegistry(ctx, driver, options...)
	if err != nil {
		t.Fatal("Failed to construct namespace")
	}
	return registry
}

func makeRepository(t *testing.T, registry distribution.Namespace, name string) distribution.Repository {
	ctx := dcontext.Background()

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
	ctx := dcontext.Background()

	manifestService, err := repository.Manifests(ctx)
	if err != nil {
		t.Fatalf("Failed to construct manifest store: %v", err)
	}
	return manifestService
}

func allManifests(t *testing.T, manifestService distribution.ManifestService) map[digest.Digest]struct{} {
	ctx := dcontext.Background()
	allManMap := make(map[digest.Digest]struct{})
	manifestEnumerator, ok := manifestService.(distribution.ManifestEnumerator)
	if !ok {
		t.Fatal("unable to convert ManifestService into ManifestEnumerator")
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
	ctx := dcontext.Background()
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
	ctx := dcontext.Background()
	manifestService := makeManifestService(t, repository)
	manifestDigest, err := manifestService.Put(ctx, im.manifest)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	return manifestDigest
}

func uploadRandomSchema2Image(t *testing.T, repository distribution.Repository) image {
	randomLayers, err := testutil.CreateRandomLayers(2)
	if err != nil {
		t.Fatalf("%v", err)
	}

	digests := make([]digest.Digest, 0, len(randomLayers))
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

func uploadRandomOCIImage(t *testing.T, repository distribution.Repository) image {
	randomLayers, err := testutil.CreateRandomLayers(2)
	if err != nil {
		t.Fatalf("%v", err)
	}

	digests := make([]digest.Digest, 0, len(randomLayers))
	for digest := range randomLayers {
		digests = append(digests, digest)
	}
	manifest, err := testutil.MakeOCIManifest(repository, digests)
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
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "palailogos")
	manifestService, _ := repo.Manifests(ctx)

	image1 := uploadRandomSchema2Image(t, repo)
	image2 := uploadRandomSchema2Image(t, repo)
	uploadRandomSchema2Image(t, repo)

	// construct manifestlist for fun.
	blobstatter := registry.BlobStatter()
	manifestList, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		image1.manifestDigest, image2.manifestDigest,
	})
	if err != nil {
		t.Fatalf("Failed to make manifest list: %v", err)
	}

	_, err = manifestService.Put(ctx, manifestList)
	if err != nil {
		t.Fatalf("Failed to add manifest list: %v", err)
	}

	before := allBlobs(t, registry)

	// Run GC
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
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
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "deletemanifests")
	manifestService, _ := repo.Manifests(ctx)

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
	manifest1, err := testutil.MakeSchema2Manifest(repo, getKeys(randomLayers1))
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}

	manifest2, err := testutil.MakeSchema2Manifest(repo, getKeys(randomLayers2))
	if err != nil {
		t.Fatalf("failed to make manifest: %v", err)
	}

	_, err = manifestService.Put(ctx, manifest1)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	_, err = manifestService.Put(ctx, manifest2)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	manifestEnumerator, _ := manifestService.(distribution.ManifestEnumerator)
	err = manifestEnumerator.Enumerate(ctx, func(dgst digest.Digest) error {
		return repo.Tags(ctx).Tag(ctx, "test", v1.Descriptor{Digest: dgst})
	})
	if err != nil {
		t.Fatalf("manifest enumeration failed: %v", err)
	}

	before1 := allBlobs(t, registry)
	before2 := allManifests(t, manifestService)

	// run GC with dry-run (should not remove anything)
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
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
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after1 := allBlobs(t, registry)
	after2 := allManifests(t, manifestService)
	if len(before1) == len(after1) {
		t.Fatalf("Garbage collection affected blobs storage: %d == %d", len(before1), len(after1))
	}
	if len(before2) == len(after2) {
		t.Fatalf("Garbage collection affected manifest storage: %d == %d", len(before2), len(after2))
	}
}

func TestDeleteManifestIndexWithDanglingReferences(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "deletemanifests")
	manifestService, _ := repo.Manifests(ctx)

	image1 := uploadRandomOCIImage(t, repo)
	image2 := uploadRandomOCIImage(t, repo)

	ii, _ := ocischema.FromDescriptors([]v1.Descriptor{
		{Digest: image1.manifestDigest}, {Digest: image2.manifestDigest},
	}, map[string]string{})

	id, err := manifestService.Put(ctx, ii)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	err = repo.Tags(ctx).Tag(ctx, "test", v1.Descriptor{Digest: id})
	if err != nil {
		t.Fatalf("Failed to delete tag: %v", err)
	}

	// delete image2 => ii has a dangling reference
	err = manifestService.Delete(ctx, image2.manifestDigest)
	if err != nil {
		t.Fatalf("Failed to delete image: %v", err)
	}

	before1 := allBlobs(t, registry)
	before2 := allManifests(t, manifestService)

	// run GC (should not remove anything because of tag)
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after1 := allBlobs(t, registry)
	after2 := allManifests(t, manifestService)
	if len(before1) == len(after1) {
		t.Fatalf("Garbage collection did not affect blobs storage: %d == %d", len(before1), len(after1))
	}
	if len(before2) != len(after2) {
		t.Fatalf("Garbage collection affected manifest storage: %d != %d", len(before2), len(after2))
	}
}

func TestDeleteManifestIndexIfTagNotFound(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "deletemanifests")
	manifestService, _ := repo.Manifests(ctx)

	image1 := uploadRandomOCIImage(t, repo)
	image2 := uploadRandomOCIImage(t, repo)

	ii, _ := ocischema.FromDescriptors([]v1.Descriptor{
		{Digest: image1.manifestDigest}, {Digest: image2.manifestDigest},
	}, map[string]string{})

	d4, err := manifestService.Put(ctx, ii)
	if err != nil {
		t.Fatalf("manifest upload failed: %v", err)
	}

	err = repo.Tags(ctx).Tag(ctx, "test", v1.Descriptor{Digest: d4})
	if err != nil {
		t.Fatalf("Failed to delete tag: %v", err)
	}

	before1 := allBlobs(t, registry)
	before2 := allManifests(t, manifestService)

	// run GC (should not remove anything because of tag)
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}
	beforeUntag1 := allBlobs(t, registry)
	beforeUntag2 := allManifests(t, manifestService)
	if len(before1) != len(beforeUntag1) {
		t.Fatalf("Garbage collection affected blobs storage: %d != %d", len(before1), len(beforeUntag1))
	}
	if len(before2) != len(beforeUntag2) {
		t.Fatalf("Garbage collection affected manifest storage: %d != %d", len(before2), len(beforeUntag2))
	}

	err = repo.Tags(ctx).Untag(ctx, "test")
	if err != nil {
		t.Fatalf("Failed to delete tag: %v", err)
	}

	// Run GC (removes everything because no manifests with tags exist)
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after1 := allBlobs(t, registry)
	after2 := allManifests(t, manifestService)
	if len(beforeUntag1) == len(after1) {
		t.Fatalf("Garbage collection did not affect blobs storage: %d == %d", len(beforeUntag1), len(after1))
	}
	if len(beforeUntag2) == len(after2) {
		t.Fatalf("Garbage collection did not affect manifest storage: %d == %d", len(beforeUntag2), len(after2))
	}
}

func TestGCWithUnusedLayerLinkPath(t *testing.T) {
	ctx := dcontext.Background()
	d := inmemory.New()

	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "unusedlayerlink")
	image := uploadRandomSchema2Image(t, repo)

	for dgst := range image.layers {
		layerLinkPath, err := pathFor(layerLinkPathSpec{name: "unusedlayerlink", digest: dgst})
		if err != nil {
			t.Fatal(err)
		}
		fileInfo, err := d.Stat(ctx, layerLinkPath)
		if err != nil {
			t.Fatal(err)
		}
		if fileInfo == nil {
			t.Fatalf("layer link path %s not found", layerLinkPath)
		}
	}

	err := MarkAndSweep(dcontext.Background(), d, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("got error: %v, expected nil", err)
	}
	for dgst := range image.layers {
		layerLinkPath, err := pathFor(layerLinkPathSpec{name: "unusedlayerlink", digest: dgst})
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Stat(ctx, layerLinkPath)
		if _, ok := err.(driver.PathNotFoundError); !ok {
			t.Fatalf("layer link path %s should be not found", layerLinkPath)
		}
	}
}

func TestGCWithUnknownRepository(t *testing.T) {
	ctx := dcontext.Background()
	d := inmemory.New()

	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "nonexistentrepo")
	image := uploadRandomSchema2Image(t, repo)

	err := repo.Tags(ctx).Tag(ctx, "image", v1.Descriptor{Digest: image.manifestDigest})
	if err != nil {
		t.Fatalf("Failed to tag descriptor: %v", err)
	}

	// Simulate a missing _manifests/tags directory
	manifestTagsPath, err := pathFor(manifestTagsPathSpec{"nonexistentrepo"})
	if err != nil {
		t.Fatal(err)
	}

	err = d.Delete(ctx, manifestTagsPath)
	if err != nil {
		t.Fatal(err)
	}

	err = MarkAndSweep(dcontext.Background(), d, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("got error: %v, expected nil", err)
	}
}

func TestGCWithMissingManifests(t *testing.T) {
	ctx := dcontext.Background()
	d := inmemory.New()

	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "testrepo")
	uploadRandomSchema2Image(t, repo)

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

	err = MarkAndSweep(dcontext.Background(), d, registry, GCOpts{
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
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "komnenos")
	manifests, _ := repo.Manifests(ctx)

	image1 := uploadRandomSchema2Image(t, repo)
	image2 := uploadRandomSchema2Image(t, repo)
	image3 := uploadRandomSchema2Image(t, repo)

	if err := manifests.Delete(ctx, image2.manifestDigest); err != nil {
		t.Fatalf("failed deleting manifest digest: %v", err)
	}

	if err := manifests.Delete(ctx, image3.manifestDigest); err != nil {
		t.Fatalf("failed deleting manifest digest: %v", err)
	}

	// Run GC
	err := MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: false,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	blobs := allBlobs(t, registry)

	// check that the image1 manifest and all the layers are still in blobs
	if _, ok := blobs[image1.manifestDigest]; !ok {
		t.Fatal("First manifest is missing")
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
	ctx := dcontext.Background()
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
	manifest1, err := testutil.MakeSchema2Manifest(repo, getKeys(randomLayers1))
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
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
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

func TestTaggedManifestlistWithUntaggedManifest(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "foo/taggedlist/untaggedmanifest")
	manifestService, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}

	image1 := uploadRandomSchema2Image(t, repo)
	image2 := uploadRandomSchema2Image(t, repo)

	// construct a manifestlist to reference manifests that is not tagged.
	blobstatter := registry.BlobStatter()
	manifestList, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		image1.manifestDigest, image2.manifestDigest,
	})
	if err != nil {
		t.Fatalf("Failed to make manifest list: %v", err)
	}

	dgst, err := manifestService.Put(ctx, manifestList)
	if err != nil {
		t.Fatalf("Failed to add manifest list: %v", err)
	}

	err = repo.Tags(ctx).Tag(ctx, "test", v1.Descriptor{Digest: dgst})
	if err != nil {
		t.Fatalf("Failed to delete tag: %v", err)
	}

	before := allBlobs(t, registry)

	// Run GC
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after := allBlobs(t, registry)
	if len(before) != len(after) {
		t.Fatalf("Garbage collection affected storage: %d != %d", len(before), len(after))
	}

	if _, ok := after[image1.manifestDigest]; !ok {
		t.Fatal("First manifest is missing")
	}

	if _, ok := after[image2.manifestDigest]; !ok {
		t.Fatal("Second manifest is missing")
	}

	if _, ok := after[dgst]; !ok {
		t.Fatal("Manifest list is missing")
	}
}

func TestUnTaggedManifestlistWithUntaggedManifest(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "foo/untaggedlist/untaggedmanifest")
	manifestService, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}

	image1 := uploadRandomSchema2Image(t, repo)
	image2 := uploadRandomSchema2Image(t, repo)

	// construct a manifestlist to reference manifests that is not tagged.
	blobstatter := registry.BlobStatter()
	manifestList, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		image1.manifestDigest, image2.manifestDigest,
	})
	if err != nil {
		t.Fatalf("Failed to make manifest list: %v", err)
	}

	_, err = manifestService.Put(ctx, manifestList)
	if err != nil {
		t.Fatalf("Failed to add manifest list: %v", err)
	}

	// Run GC
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after := allBlobs(t, registry)
	if len(after) != 0 {
		t.Fatalf("Garbage collection affected storage: %d != %d", len(after), 0)
	}

}

func TestUnTaggedManifestlistWithTaggedManifest(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "foo/untaggedlist/taggedmanifest")
	manifestService, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}

	image1 := uploadRandomSchema2Image(t, repo)
	image2 := uploadRandomSchema2Image(t, repo)

	err = repo.Tags(ctx).Tag(ctx, "image1", v1.Descriptor{Digest: image1.manifestDigest})
	if err != nil {
		t.Fatalf("Failed to delete tag: %v", err)
	}

	err = repo.Tags(ctx).Tag(ctx, "image2", v1.Descriptor{Digest: image2.manifestDigest})
	if err != nil {
		t.Fatalf("Failed to delete tag: %v", err)
	}

	// construct a manifestlist to reference manifests that is tagged.
	blobstatter := registry.BlobStatter()
	manifestList, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		image1.manifestDigest, image2.manifestDigest,
	})
	if err != nil {
		t.Fatalf("Failed to make manifest list: %v", err)
	}

	dgst, err := manifestService.Put(ctx, manifestList)
	if err != nil {
		t.Fatalf("Failed to add manifest list: %v", err)
	}

	// Run GC
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after := allBlobs(t, registry)
	afterManifests := allManifests(t, manifestService)

	if _, ok := after[dgst]; ok {
		t.Fatal("Untagged manifestlist still exists")
	}

	if _, ok := afterManifests[image1.manifestDigest]; !ok {
		t.Fatal("First manifest is missing")
	}

	if _, ok := afterManifests[image2.manifestDigest]; !ok {
		t.Fatal("Second manifest is missing")
	}
}

func TestTaggedManifestlistWithDeletedReference(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "foo/untaggedlist/deleteref")
	manifestService, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}

	image1 := uploadRandomSchema2Image(t, repo)
	image2 := uploadRandomSchema2Image(t, repo)

	// construct a manifestlist to reference manifests that is deleted.
	blobstatter := registry.BlobStatter()
	manifestList, err := testutil.MakeManifestList(blobstatter, []digest.Digest{
		image1.manifestDigest, image2.manifestDigest,
	})
	if err != nil {
		t.Fatalf("Failed to make manifest list: %v", err)
	}

	_, err = manifestService.Put(ctx, manifestList)
	if err != nil {
		t.Fatalf("Failed to add manifest list: %v", err)
	}

	err = manifestService.Delete(ctx, image1.manifestDigest)
	if err != nil {
		t.Fatalf("Failed to delete image: %v", err)
	}

	err = manifestService.Delete(ctx, image2.manifestDigest)
	if err != nil {
		t.Fatalf("Failed to delete image: %v", err)
	}

	// Run GC
	err = MarkAndSweep(dcontext.Background(), inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		RemoveUntagged: true,
	})
	if err != nil {
		t.Fatalf("Failed mark and sweep: %v", err)
	}

	after := allBlobs(t, registry)
	if len(after) != 0 {
		t.Fatalf("Garbage collection affected storage: %d != %d", len(after), 0)
	}
}

func TestTwoPassGarbageCollect(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "twopass")

	// Create and upload two images, delete one
	image1 := uploadRandomSchema2Image(t, repo)
	image2 := uploadRandomSchema2Image(t, repo)

	manifestService := makeManifestService(t, repo)
	err := manifestService.Delete(ctx, image2.manifestDigest)
	if err != nil {
		t.Fatalf("Failed to delete manifest: %v", err)
	}

	before := allBlobs(t, registry)

	// Create temp checkpoint dir
	checkpointDir := t.TempDir()

	// Phase 1: Mark-only phase
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		MarkOnly:      true,
		Quiet:         true,
	})
	if err != nil {
		t.Fatalf("Failed mark-only phase: %v", err)
	}

	// Verify checkpoint file was created (on filesystem, not in inmemory driver)
	checkpointPath := checkpointDir + "/candidates.json"
	if _, err := os.Stat(checkpointPath); err != nil {
		t.Fatalf("Checkpoint file not created: %v", err)
	}

	// Verify no blobs deleted yet
	afterMark := allBlobs(t, registry)
	if len(before) != len(afterMark) {
		t.Fatalf("Mark-only phase deleted blobs: %d != %d", len(before), len(afterMark))
	}

	// Phase 2: Sweep phase
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		SweepOnly:     true,
		Quiet:         true,
	})
	if err != nil {
		t.Fatalf("Failed sweep phase: %v", err)
	}

	// Verify checkpoint file was automatically deleted after successful sweep
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Fatalf("Checkpoint file should be deleted after sweep: %v", err)
	}

	// Verify image2 layers were deleted but image1 layers remain
	afterSweep := allBlobs(t, registry)
	if len(afterSweep) >= len(before) {
		t.Fatalf("Sweep phase did not delete blobs: %d >= %d", len(afterSweep), len(before))
	}

	// Verify image1 is still intact
	if _, ok := afterSweep[image1.manifestDigest]; !ok {
		t.Fatal("Image1 manifest was deleted")
	}
	for layer := range image1.layers {
		if _, ok := afterSweep[layer]; !ok {
			t.Fatalf("Image1 layer missing: %v", layer)
		}
	}

	// Verify image2 layers were deleted
	for layer := range image2.layers {
		if _, ok := afterSweep[layer]; ok {
			t.Fatalf("Image2 layer still exists: %v", layer)
		}
	}
}

func TestTwoPassWithNewReferences(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "twopass_newrefs")

	// Upload initial image and delete it (should be GC candidate)
	image1 := uploadRandomSchema2Image(t, repo)
	manifestService := makeManifestService(t, repo)
	err := manifestService.Delete(ctx, image1.manifestDigest)
	if err != nil {
		t.Fatalf("Failed to delete manifest: %v", err)
	}

	checkpointDir := t.TempDir()

	// Phase 1: Mark-only phase (image1 is candidate for deletion)
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		MarkOnly:      true,
		Quiet:         true,
	})
	if err != nil {
		t.Fatalf("Failed mark-only phase: %v", err)
	}

	// Simulate new push AFTER mark phase: upload new image with fresh layers
	// (we can't reuse image1.layers because the ReadSeeker has been consumed)
	newLayers, err := testutil.CreateRandomLayers(2)
	if err != nil {
		t.Fatalf("Failed to create layers: %v", err)
	}

	// Track one of the new layers to verify it's protected
	newLayerDigest := getAnyKey(newLayers)

	err = testutil.UploadBlobs(repo, newLayers)
	if err != nil {
		t.Fatalf("Failed to upload new image: %v", err)
	}

	newManifest, err := testutil.MakeSchema2Manifest(repo, getKeys(newLayers))
	if err != nil {
		t.Fatalf("Failed to make manifest: %v", err)
	}

	newManifestDigest, err := manifestService.Put(ctx, newManifest)
	if err != nil {
		t.Fatalf("Failed to put manifest: %v", err)
	}

	// Phase 2: Sweep phase (should re-mark and protect the new image)
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		SweepOnly:     true,
		Quiet:         true,
	})
	if err != nil {
		t.Fatalf("Failed sweep phase: %v", err)
	}

	// Verify checkpoint file was automatically deleted after successful sweep
	checkpointPath := checkpointDir + "/candidates.json"
	if _, err := os.Stat(checkpointPath); !os.IsNotExist(err) {
		t.Fatalf("Checkpoint file should be deleted after sweep: %v", err)
	}

	// Verify new image and its layers were NOT deleted (protected by re-mark)
	afterSweep := allBlobs(t, registry)
	if _, ok := afterSweep[newManifestDigest]; !ok {
		t.Fatalf("New manifest was deleted: %v", newManifestDigest)
	}
	if _, ok := afterSweep[newLayerDigest]; !ok {
		t.Fatalf("New layer was deleted despite being pushed between mark and sweep: %v", newLayerDigest)
	}
}

func TestGarbageCollectConcurrency(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)

	// Create multiple repositories with images
	for i := 0; i < 10; i++ {
		repoName := "concurrent_" + string(rune('a'+i))
		repo := makeRepository(t, registry, repoName)
		uploadRandomSchema2Image(t, repo)
		image := uploadRandomSchema2Image(t, repo)

		// Delete one image per repo
		manifestService := makeManifestService(t, repo)
		err := manifestService.Delete(ctx, image.manifestDigest)
		if err != nil {
			t.Fatalf("Failed to delete manifest: %v", err)
		}
	}

	beforeSerial := allBlobs(t, registry)

	// Run GC with 1 worker (serial)
	err := MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		MaxConcurrency: 1,
		Quiet:          true,
	})
	if err != nil {
		t.Fatalf("Failed serial GC: %v", err)
	}

	afterSerial := allBlobs(t, registry)

	// Reset registry state
	inmemoryDriver = inmemory.New()
	registry = createRegistry(t, inmemoryDriver)

	// Recreate same data
	for i := 0; i < 10; i++ {
		repoName := "concurrent_" + string(rune('a'+i))
		repo := makeRepository(t, registry, repoName)
		uploadRandomSchema2Image(t, repo)
		image := uploadRandomSchema2Image(t, repo)

		manifestService := makeManifestService(t, repo)
		err := manifestService.Delete(ctx, image.manifestDigest)
		if err != nil {
			t.Fatalf("Failed to delete manifest: %v", err)
		}
	}

	beforeParallel := allBlobs(t, registry)

	// Run GC with 8 workers (parallel)
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:         false,
		MaxConcurrency: 8,
		Quiet:          true,
	})
	if err != nil {
		t.Fatalf("Failed parallel GC: %v", err)
	}

	afterParallel := allBlobs(t, registry)

	// Verify same blobs were deleted in both modes
	if len(beforeSerial) != len(beforeParallel) {
		t.Fatalf("Initial state differs: %d != %d", len(beforeSerial), len(beforeParallel))
	}

	if len(afterSerial) != len(afterParallel) {
		t.Fatalf("Results differ between serial and parallel: %d != %d", len(afterSerial), len(afterParallel))
	}
}

func TestDistributedLock(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "locktest")
	uploadRandomSchema2Image(t, repo)

	checkpointDir := t.TempDir()

	// Start first GC with mark-only (acquires lock)
	err := MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		MarkOnly:      true,
		Quiet:         true,
	})
	if err != nil {
		t.Fatalf("First GC failed: %v", err)
	}

	// Lock should be released after mark-only completes
	// Try to run second GC immediately (should succeed)
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		MarkOnly:      true,
		Quiet:         true,
	})
	if err != nil {
		t.Fatalf("Second GC failed (lock should be released): %v", err)
	}
}

func TestCheckpointExpiration(t *testing.T) {
	ctx := dcontext.Background()
	inmemoryDriver := inmemory.New()

	registry := createRegistry(t, inmemoryDriver)
	repo := makeRepository(t, registry, "expiration")

	// Upload and delete an image
	image := uploadRandomSchema2Image(t, repo)
	manifestService := makeManifestService(t, repo)
	err := manifestService.Delete(ctx, image.manifestDigest)
	if err != nil {
		t.Fatalf("Failed to delete manifest: %v", err)
	}

	checkpointDir := t.TempDir()

	// Run mark-only to create checkpoint
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		MarkOnly:      true,
		Quiet:         true,
	})
	if err != nil {
		t.Fatalf("Mark phase failed: %v", err)
	}

	// Manually modify checkpoint timestamp to be 8 days old (expired)
	checkpointPath := checkpointDir + "/candidates.json"
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatalf("Failed to read checkpoint: %v", err)
	}

	// Unmarshal, modify timestamp, and marshal back
	var checkpoint CheckpointState
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		t.Fatalf("Failed to unmarshal checkpoint: %v", err)
	}

	// Set timestamp to 8 days ago (expired)
	checkpoint.Timestamp = time.Now().Add(-8 * 24 * time.Hour)

	// Write modified checkpoint back
	modifiedData, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal checkpoint: %v", err)
	}

	err = os.WriteFile(checkpointPath, modifiedData, 0644)
	if err != nil {
		t.Fatalf("Failed to write modified checkpoint: %v", err)
	}

	// Try to run sweep with expired checkpoint (should fail)
	err = MarkAndSweep(ctx, inmemoryDriver, registry, GCOpts{
		DryRun:        false,
		CheckpointDir: checkpointDir,
		SweepOnly:     true,
		Quiet:         true,
	})
	if err == nil {
		t.Fatal("Sweep phase should have failed with expired checkpoint")
	}
	if !strings.Contains(err.Error(), "checkpoint is too old") && !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Expected expiration error, got: %v", err)
	}
}

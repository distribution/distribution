package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type image struct {
	manifest       distribution.Manifest
	manifestDigest digest.Digest
	layers         map[digest.Digest]io.ReadSeeker
}

func createRegistry(t testing.TB, driver driver.StorageDriver, options ...RegistryOption) distribution.Namespace {
	ctx := dcontext.Background()
	options = append(options, EnableDelete)
	registry, err := NewRegistry(ctx, driver, options...)
	if err != nil {
		t.Fatal("Failed to construct namespace")
	}
	return registry
}

func makeRepository(t testing.TB, registry distribution.Namespace, name string) distribution.Repository {
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

func makeManifestService(t testing.TB, repository distribution.Repository) distribution.ManifestService {
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

// gcTestDriver observes lookups and injects storage failures during marking.
type gcTestDriver struct {
	driver.StorageDriver
	misses           atomic.Int64
	partialRevision  string
	enumerationError error
	deletes          atomic.Int64
	lookupPath       string
	lookupError      error
}

func (d *gcTestDriver) GetContent(ctx context.Context, p string) ([]byte, error) {
	if p == d.lookupPath {
		return nil, d.lookupError
	}
	content, err := d.StorageDriver.GetContent(ctx, p)
	var notFound driver.PathNotFoundError
	if strings.Contains(p, "/_manifests/revisions/") && errors.As(err, &notFound) {
		d.misses.Add(1)
	}
	return content, err
}

func (d *gcTestDriver) Walk(ctx context.Context, p string, fn driver.WalkFn, options ...func(*driver.WalkOptions)) error {
	if d.partialRevision != "" && strings.HasSuffix(p, "/_manifests/revisions") {
		info, err := d.StorageDriver.Stat(ctx, d.partialRevision)
		if err != nil {
			return err
		}
		if err := fn(info); err != nil {
			return err
		}
		return d.enumerationError
	}
	return d.StorageDriver.Walk(ctx, p, fn, options...)
}

func (d *gcTestDriver) Delete(ctx context.Context, p string) error {
	d.deletes.Add(1)
	return d.StorageDriver.Delete(ctx, p)
}

func TestGCWithoutBlobManifestLookups(t *testing.T) {
	for _, backend := range []string{"inmemory", "filesystem"} {
		t.Run(backend, func(t *testing.T) {
			for _, removeUntagged := range []bool{false, true} {
				t.Run(fmt.Sprint(removeUntagged), func(t *testing.T) {
					ctx := dcontext.Background()
					var backendDriver driver.StorageDriver = inmemory.New()
					if backend == "filesystem" {
						var err error
						backendDriver, err = filesystem.FromParameters(map[string]any{"rootdirectory": t.TempDir()})
						if err != nil {
							t.Fatal(err)
						}
					}
					d := &gcTestDriver{StorageDriver: backendDriver}
					registry, _ := gcTaggedIndex(t, d)
					d.misses.Store(0)
					if err := MarkAndSweep(ctx, d, registry, GCOpts{RemoveUntagged: removeUntagged, Quiet: true}); err != nil {
						t.Fatal(err)
					}
					if misses := d.misses.Load(); misses != 0 {
						t.Errorf("non-manifest revision lookups: got %d misses, want 0", misses)
					}
				})
			}
		})
	}
}

func BenchmarkGarbageCollect(b *testing.B) {
	ctx := dcontext.Background()
	d, err := filesystem.FromParameters(map[string]any{"rootdirectory": b.TempDir()})
	if err != nil {
		b.Fatal(err)
	}
	registry := createRegistry(b, d)
	repo := makeRepository(b, registry, "gc/benchmark")
	manifests := makeManifestService(b, repo)
	for image := range 20 {
		layers := make([]digest.Digest, 0, 10)
		for layer := range 10 {
			desc, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageLayer, fmt.Appendf(nil, "image-%d-layer-%d", image, layer))
			if err != nil {
				b.Fatal(err)
			}
			layers = append(layers, desc.Digest)
		}
		manifest, err := testutil.MakeOCIManifest(repo, layers)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := manifests.Put(ctx, manifest); err != nil {
			b.Fatal(err)
		}
	}
	for b.Loop() {
		if err := MarkAndSweep(ctx, d, registry, GCOpts{}); err != nil {
			b.Fatal(err)
		}
	}
}

func gcTaggedIndex(t *testing.T, d driver.StorageDriver) (distribution.Namespace, digest.Digest) {
	t.Helper()
	ctx := dcontext.Background()
	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "gc/references")
	im := uploadRandomOCIImage(t, repo)
	manifests := makeManifestService(t, repo)
	index, err := testutil.MakeManifestList(registry.BlobStatter(), []digest.Digest{im.manifestDigest})
	if err != nil {
		t.Fatal(err)
	}
	dgst, err := manifests.Put(ctx, index)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Tags(ctx).Tag(ctx, "latest", v1.Descriptor{Digest: dgst}); err != nil {
		t.Fatal(err)
	}
	return registry, dgst
}

func TestGCPartialManifestEnumeration(t *testing.T) {
	for _, removeUntagged := range []bool{false, true} {
		t.Run(fmt.Sprint(removeUntagged), func(t *testing.T) {
			for _, missing := range []bool{false, true} {
				t.Run(fmt.Sprint(missing), func(t *testing.T) {
					ctx := dcontext.Background()
					d := &gcTestDriver{StorageDriver: inmemory.New()}
					registry, indexDigest := gcTaggedIndex(t, d)
					before := allBlobs(t, registry)
					var err error
					d.partialRevision, err = manifestRevisionLinkPath("gc/references", indexDigest)
					if err != nil {
						t.Fatal(err)
					}
					d.enumerationError = errors.New("enumeration interrupted")
					if missing {
						d.enumerationError = driver.PathNotFoundError{Path: "missing revision"}
					}
					d.deletes.Store(0)
					err = MarkAndSweep(ctx, d, registry, GCOpts{RemoveUntagged: removeUntagged, Quiet: true})
					if missing {
						if err != nil {
							t.Fatal(err)
						}
					} else {
						if err == nil || !strings.Contains(err.Error(), "enumeration interrupted") {
							t.Fatalf("expected enumeration error, got %v", err)
						}
						if d.deletes.Load() != 0 {
							t.Fatal("sweep started after enumeration failure")
						}
					}
					if after := allBlobs(t, registry); !reflect.DeepEqual(before, after) {
						t.Fatalf("referenced blobs changed: before %v, after %v", before, after)
					}
				})
			}
		})
	}
}

func TestGCManifestAliasReferences(t *testing.T) {
	ctx := dcontext.Background()
	d := inmemory.New()
	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "gc/alias")
	child := uploadRandomOCIImage(t, repo)
	_, payload, err := child.manifest.Payload()
	if err != nil {
		t.Fatal(err)
	}
	alias := digest.SHA512.FromBytes(payload)
	aliasPath, err := manifestRevisionLinkPath(repo.Named().Name(), alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.PutContent(ctx, aliasPath, []byte(child.manifestDigest)); err != nil {
		t.Fatal(err)
	}
	index, err := ocischema.FromDescriptors([]v1.Descriptor{{Digest: alias}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ms := makeManifestService(t, repo)
	root, err := ms.Put(ctx, index)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Tags(ctx).Tag(ctx, "latest", v1.Descriptor{Digest: root}); err != nil {
		t.Fatal(err)
	}
	// Leave an alias-only revision, so enumeration cannot visit the child
	// independently and mask a missing recursive alias traversal.
	canonicalPath, err := manifestRevisionLinkPath(repo.Named().Name(), child.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Delete(ctx, canonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := MarkAndSweep(ctx, d, registry, GCOpts{RemoveUntagged: true, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	for _, ref := range child.manifest.References() {
		if _, err := repo.Blobs(ctx).Stat(ctx, ref.Digest); err != nil {
			t.Errorf("aliased child's referenced blob %s was lost: %v", ref.Digest, err)
		}
	}
}

type gcManifestReadErrorDriver struct{ *gcTestDriver }

func (d *gcManifestReadErrorDriver) Reader(ctx context.Context, p string, offset int64) (io.ReadCloser, error) {
	if p == d.lookupPath {
		return nil, d.lookupError
	}
	return d.StorageDriver.Reader(ctx, p, offset)
}

func TestGCStopsAfterManifestError(t *testing.T) {
	ctx := dcontext.Background()
	d := &gcManifestReadErrorDriver{&gcTestDriver{StorageDriver: inmemory.New()}}
	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "gc/mark-error")
	uploadRandomOCIImage(t, repo)
	uploadRandomOCIImage(t, repo)
	var revisions []digest.Digest
	ms := makeManifestService(t, repo)
	if err := ms.(distribution.ManifestEnumerator).Enumerate(ctx, func(dgst digest.Digest) error {
		revisions = append(revisions, dgst)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 {
		t.Fatalf("expected two manifest revisions, got %v", revisions)
	}
	var err error
	d.lookupPath, err = pathFor(blobDataPathSpec{digest: revisions[0]})
	if err != nil {
		t.Fatal(err)
	}
	d.lookupError = errors.New("manifest content unavailable")
	d.deletes.Store(0)
	err = MarkAndSweep(ctx, d, registry, GCOpts{Quiet: true})
	if err == nil || !strings.Contains(err.Error(), d.lookupError.Error()) {
		t.Errorf("expected first manifest's retrieval error, got %v", err)
	}
	if d.deletes.Load() != 0 {
		t.Error("sweep started after manifest retrieval failure")
	}
}

func TestGCInvalidReferences(t *testing.T) {
	ctx := dcontext.Background()
	d := inmemory.New()
	registry := createRegistry(t, d)
	repo := makeRepository(t, registry, "gc/invalid-references")
	layer, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageLayer, []byte("retained layer"))
	if err != nil {
		t.Fatal(err)
	}
	ms, err := repo.Manifests(ctx, SkipLayerVerification())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ocischema.FromStruct(ocischema.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: v1.MediaTypeImageManifest,
		Config:    v1.Descriptor{},
		Layers:    []v1.Descriptor{{Digest: "not-a-digest"}, layer},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err := ms.Put(ctx, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Tags(ctx).Tag(ctx, "latest", v1.Descriptor{Digest: root}); err != nil {
		t.Fatal(err)
	}
	if err := MarkAndSweep(ctx, d, registry, GCOpts{Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Blobs(ctx).Get(ctx, layer.Digest); err != nil {
		t.Fatalf("valid reference after invalid references was lost: %v", err)
	}
}

// gcSkippedRevisionDriver makes a revision unavailable during enumeration only.
type gcSkippedRevisionDriver struct {
	driver.StorageDriver
	mode, link, blob, current string
	reads                     int
	failed                    bool
}

func (d *gcSkippedRevisionDriver) Stat(ctx context.Context, p string) (driver.FileInfo, error) {
	if !d.failed && ((d.mode == "blob-stat" && d.current == d.link && p == d.blob) || (d.mode == "walk-stat" && p == d.link)) {
		d.failed = true
		return nil, driver.PathNotFoundError{Path: p}
	}
	return d.StorageDriver.Stat(ctx, p)
}

func (d *gcSkippedRevisionDriver) GetContent(ctx context.Context, p string) ([]byte, error) {
	if d.mode == "link-read" && d.current == d.link && p == d.link {
		d.reads++
		// The first read supplies the digest; the second is its link Stat.
		if d.reads == 2 {
			d.failed = true
			return nil, driver.PathNotFoundError{Path: p}
		}
	}
	return d.StorageDriver.GetContent(ctx, p)
}

func (d *gcSkippedRevisionDriver) Walk(ctx context.Context, p string, fn driver.WalkFn, options ...func(*driver.WalkOptions)) error {
	if !strings.HasSuffix(p, "/_manifests/revisions") {
		return d.StorageDriver.Walk(ctx, p, fn, options...)
	}
	visit := func(info driver.FileInfo) error {
		d.current = info.Path()
		defer func() { d.current = "" }()
		return fn(info)
	}
	if d.mode == "walk-stat" {
		return driver.WalkFallback(ctx, d, p, visit, options...)
	}
	return d.StorageDriver.Walk(ctx, p, visit, options...)
}

func TestGCSkippedManifestRevision(t *testing.T) {
	for _, removeUntagged := range []bool{false, true} {
		for _, mode := range []string{"blob-stat", "link-read", "walk-stat", "missing-blob"} {
			t.Run(fmt.Sprintf("%s/remove=%t", mode, removeUntagged), func(t *testing.T) {
				ctx := dcontext.Background()
				d := &gcSkippedRevisionDriver{StorageDriver: inmemory.New()}
				registry := createRegistry(t, d)
				repo := makeRepository(t, registry, "gc/skipped")
				ms := makeManifestService(t, repo)
				layer, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageLayer, []byte("retained layer"))
				if err != nil {
					t.Fatal(err)
				}
				child, err := testutil.MakeOCIManifest(repo, []digest.Digest{layer.Digest})
				if err != nil {
					t.Fatal(err)
				}
				childDigest, err := ms.Put(ctx, child)
				if err != nil {
					t.Fatal(err)
				}
				index, err := ocischema.FromDescriptors([]v1.Descriptor{{Digest: childDigest}}, nil)
				if err != nil {
					t.Fatal(err)
				}
				root, err := ms.Put(ctx, index)
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.Tags(ctx).Tag(ctx, "latest", v1.Descriptor{Digest: root}); err != nil {
					t.Fatal(err)
				}
				d.blob, err = pathFor(blobDataPathSpec{digest: childDigest})
				if err != nil {
					t.Fatal(err)
				}
				d.link, err = manifestRevisionLinkPath(repo.Named().Name(), childDigest)
				if err != nil {
					t.Fatal(err)
				}
				want := allBlobs(t, registry)
				if mode == "missing-blob" {
					if err := d.Delete(ctx, d.blob); err != nil {
						t.Fatal(err)
					}
					// A permanently absent child remains a dangling reference.
					want = map[digest.Digest]struct{}{root: {}}
				}
				d.mode = mode
				if err := MarkAndSweep(ctx, d, registry, GCOpts{RemoveUntagged: removeUntagged, Quiet: true}); err != nil {
					t.Fatal(err)
				}
				if mode != "missing-blob" && !d.failed {
					t.Fatal("enumeration did not encounter the missing revision")
				}
				if got := allBlobs(t, registry); !reflect.DeepEqual(want, got) {
					t.Errorf("blobs after skipped revision: got %v, want %v", got, want)
				}
			})
		}
	}
}

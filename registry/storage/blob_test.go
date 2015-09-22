package storage

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"path"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/testutil"
)

// TestSimpleBlobUpload covers the blob upload process, exercising common
// error paths that might be seen during an upload.
func TestSimpleBlobUpload(t *testing.T) {
	randomDataReader, dgst, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random reader: %v", err)
	}

	ctx := context.Background()
	imageName, _ := reference.ParseNamed("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repository.Blobs(ctx)

	h := sha256.New()
	rd := io.TeeReader(randomDataReader, h)

	blobUpload, err := bs.Create(ctx)

	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}

	// Cancel the upload then restart it
	if err := blobUpload.Cancel(ctx); err != nil {
		t.Fatalf("unexpected error during upload cancellation: %v", err)
	}

	// Do a resume, get unknown upload
	blobUpload, err = bs.Resume(ctx, blobUpload.ID())
	if err != distribution.ErrBlobUploadUnknown {
		t.Fatalf("unexpected error resuming upload, should be unknown: %v", err)
	}

	// Restart!
	blobUpload, err = bs.Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}

	// Get the size of our random tarfile
	randomDataSize, err := seekerSize(randomDataReader)
	if err != nil {
		t.Fatalf("error getting seeker size of random data: %v", err)
	}

	nn, err := io.Copy(blobUpload, rd)
	if err != nil {
		t.Fatalf("unexpected error uploading layer data: %v", err)
	}

	if nn != randomDataSize {
		t.Fatalf("layer data write incomplete")
	}

	offset, err := blobUpload.Seek(0, os.SEEK_CUR)
	if err != nil {
		t.Fatalf("unexpected error seeking layer upload: %v", err)
	}

	if offset != nn {
		t.Fatalf("blobUpload not updated with correct offset: %v != %v", offset, nn)
	}
	blobUpload.Close()

	// Do a resume, for good fun
	blobUpload, err = bs.Resume(ctx, blobUpload.ID())
	if err != nil {
		t.Fatalf("unexpected error resuming upload: %v", err)
	}

	sha256Digest := digest.NewDigest("sha256", h)
	desc, err := blobUpload.Commit(ctx, distribution.Descriptor{Digest: dgst})
	if err != nil {
		t.Fatalf("unexpected error finishing layer upload: %v", err)
	}

	// After finishing an upload, it should no longer exist.
	if _, err := bs.Resume(ctx, blobUpload.ID()); err != distribution.ErrBlobUploadUnknown {
		t.Fatalf("expected layer upload to be unknown, got %v", err)
	}

	// Test for existence.
	statDesc, err := bs.Stat(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v, %#v", err, bs)
	}

	if statDesc != desc {
		t.Fatalf("descriptors not equal: %v != %v", statDesc, desc)
	}

	rc, err := bs.Open(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error opening blob for read: %v", err)
	}
	defer rc.Close()

	h.Reset()
	nn, err = io.Copy(h, rc)
	if err != nil {
		t.Fatalf("error reading layer: %v", err)
	}

	if nn != randomDataSize {
		t.Fatalf("incorrect read length")
	}

	if digest.NewDigest("sha256", h) != sha256Digest {
		t.Fatalf("unexpected digest from uploaded layer: %q != %q", digest.NewDigest("sha256", h), sha256Digest)
	}

	checkBlobParentPath(t, ctx, driver, nil, desc.Digest, true)
	checkBlobParentPath(t, ctx, driver, imageName, desc.Digest, true)

	// Delete a blob
	err = bs.Delete(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("Unexpected error deleting blob: %v", err)
	}

	d, err := bs.Stat(ctx, desc.Digest)
	if err == nil {
		t.Fatalf("unexpected non-error stating deleted blob: %v", d)
	}

	checkBlobParentPath(t, ctx, driver, nil, desc.Digest, true)
	checkBlobParentPath(t, ctx, driver, imageName, desc.Digest, true)

	switch err {
	case distribution.ErrBlobUnknown:
		break
	default:
		t.Errorf("Unexpected error type stat-ing deleted manifest: %#v", err)
	}

	_, err = bs.Open(ctx, desc.Digest)
	if err == nil {
		t.Fatalf("unexpected success opening deleted blob for read")
	}

	switch err {
	case distribution.ErrBlobUnknown:
		break
	default:
		t.Errorf("Unexpected error type getting deleted manifest: %#v", err)
	}

	// Re-upload the blob
	randomBlob, err := ioutil.ReadAll(randomDataReader)
	if err != nil {
		t.Fatalf("Error reading all of blob %s", err.Error())
	}
	expectedDigest := digest.FromBytes(randomBlob)
	simpleUpload(t, bs, randomBlob, expectedDigest)

	d, err = bs.Stat(ctx, expectedDigest)
	if err != nil {
		t.Errorf("unexpected error stat-ing blob")
	}
	if d.Digest != expectedDigest {
		t.Errorf("Mismatching digest with restored blob")
	}

	_, err = bs.Open(ctx, expectedDigest)
	if err != nil {
		t.Errorf("Unexpected error opening blob")
	}

	// Reuse state to test delete with a delete-disabled registry
	registry, err = NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err = registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs = repository.Blobs(ctx)
	err = bs.Delete(ctx, desc.Digest)
	if err == nil {
		t.Errorf("Unexpected success deleting while disabled")
	}
}

// TestSimpleBlobRead just creates a simple blob file and ensures that basic
// open, read, seek, read works. More specific edge cases should be covered in
// other tests.
func TestSimpleBlobRead(t *testing.T) {
	ctx := context.Background()
	imageName, _ := reference.ParseNamed("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repository.Blobs(ctx)

	randomLayerReader, dgst, err := testutil.CreateRandomTarFile() // TODO(stevvooe): Consider using just a random string.
	if err != nil {
		t.Fatalf("error creating random data: %v", err)
	}

	// Test for existence.
	desc, err := bs.Stat(ctx, dgst)
	if err != distribution.ErrBlobUnknown {
		t.Fatalf("expected not found error when testing for existence: %v", err)
	}

	rc, err := bs.Open(ctx, dgst)
	if err != distribution.ErrBlobUnknown {
		t.Fatalf("expected not found error when opening non-existent blob: %v", err)
	}

	randomLayerSize, err := seekerSize(randomLayerReader)
	if err != nil {
		t.Fatalf("error getting seeker size for random layer: %v", err)
	}

	descBefore := distribution.Descriptor{Digest: dgst, MediaType: "application/octet-stream", Size: randomLayerSize}
	t.Logf("desc: %v", descBefore)

	desc, err = addBlob(ctx, bs, descBefore, randomLayerReader)
	if err != nil {
		t.Fatalf("error adding blob to blobservice: %v", err)
	}

	if desc.Size != randomLayerSize {
		t.Fatalf("committed blob has incorrect length: %v != %v", desc.Size, randomLayerSize)
	}

	rc, err = bs.Open(ctx, desc.Digest) // note that we are opening with original digest.
	if err != nil {
		t.Fatalf("error opening blob with %v: %v", dgst, err)
	}
	defer rc.Close()

	// Now check the sha digest and ensure its the same
	h := sha256.New()
	nn, err := io.Copy(h, rc)
	if err != nil {
		t.Fatalf("unexpected error copying to hash: %v", err)
	}

	if nn != randomLayerSize {
		t.Fatalf("stored incorrect number of bytes in blob: %d != %d", nn, randomLayerSize)
	}

	sha256Digest := digest.NewDigest("sha256", h)
	if sha256Digest != desc.Digest {
		t.Fatalf("fetched digest does not match: %q != %q", sha256Digest, desc.Digest)
	}

	// Now seek back the blob, read the whole thing and check against randomLayerData
	offset, err := rc.Seek(0, os.SEEK_SET)
	if err != nil {
		t.Fatalf("error seeking blob: %v", err)
	}

	if offset != 0 {
		t.Fatalf("seek failed: expected 0 offset, got %d", offset)
	}

	p, err := ioutil.ReadAll(rc)
	if err != nil {
		t.Fatalf("error reading all of blob: %v", err)
	}

	if len(p) != int(randomLayerSize) {
		t.Fatalf("blob data read has different length: %v != %v", len(p), randomLayerSize)
	}

	// Reset the randomLayerReader and read back the buffer
	_, err = randomLayerReader.Seek(0, os.SEEK_SET)
	if err != nil {
		t.Fatalf("error resetting layer reader: %v", err)
	}

	randomLayerData, err := ioutil.ReadAll(randomLayerReader)
	if err != nil {
		t.Fatalf("random layer read failed: %v", err)
	}

	if !bytes.Equal(p, randomLayerData) {
		t.Fatalf("layer data not equal")
	}
}

// TestBlobMount covers the blob mount process, exercising common
// error paths that might be seen during a mount.
func TestBlobMount(t *testing.T) {
	randomDataReader, dgst, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random reader: %v", err)
	}

	ctx := context.Background()
	imageName, _ := reference.ParseNamed("foo/bar")
	sourceImageName, _ := reference.ParseNamed("foo/source")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	sourceRepository, err := registry.Repository(ctx, sourceImageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	sbs := sourceRepository.Blobs(ctx)

	blobUpload, err := sbs.Create(ctx)

	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}

	// Get the size of our random tarfile
	randomDataSize, err := seekerSize(randomDataReader)
	if err != nil {
		t.Fatalf("error getting seeker size of random data: %v", err)
	}

	nn, err := io.Copy(blobUpload, randomDataReader)
	if err != nil {
		t.Fatalf("unexpected error uploading layer data: %v", err)
	}

	desc, err := blobUpload.Commit(ctx, distribution.Descriptor{Digest: dgst})
	if err != nil {
		t.Fatalf("unexpected error finishing layer upload: %v", err)
	}

	// Test for existence.
	statDesc, err := sbs.Stat(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v, %#v", err, sbs)
	}

	if statDesc != desc {
		t.Fatalf("descriptors not equal: %v != %v", statDesc, desc)
	}

	bs := repository.Blobs(ctx)
	// Test destination for existence.
	statDesc, err = bs.Stat(ctx, desc.Digest)
	if err == nil {
		t.Fatalf("unexpected non-error stating unmounted blob: %v", desc)
	}

	canonicalRef, err := reference.WithDigest(sourceRepository.Name(), desc.Digest)
	if err != nil {
		t.Fatal(err)
	}

	bw, err := bs.Create(ctx, WithMountFrom(canonicalRef))
	if bw != nil {
		t.Fatal("unexpected blobwriter returned from Create call, should mount instead")
	}

	ebm, ok := err.(distribution.ErrBlobMounted)
	if !ok {
		t.Fatalf("unexpected error mounting layer: %v", err)
	}

	if ebm.Descriptor != desc {
		t.Fatalf("descriptors not equal: %v != %v", ebm.Descriptor, desc)
	}

	// Test for existence.
	statDesc, err = bs.Stat(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v, %#v", err, bs)
	}

	if statDesc != desc {
		t.Fatalf("descriptors not equal: %v != %v", statDesc, desc)
	}

	rc, err := bs.Open(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error opening blob for read: %v", err)
	}
	defer rc.Close()

	h := sha256.New()
	nn, err = io.Copy(h, rc)
	if err != nil {
		t.Fatalf("error reading layer: %v", err)
	}

	if nn != randomDataSize {
		t.Fatalf("incorrect read length")
	}

	if digest.NewDigest("sha256", h) != dgst {
		t.Fatalf("unexpected digest from uploaded layer: %q != %q", digest.NewDigest("sha256", h), dgst)
	}

	// Delete the blob from the source repo
	err = sbs.Delete(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("Unexpected error deleting blob")
	}

	d, err := bs.Stat(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error stating blob deleted from source repository: %v", err)
	}

	d, err = sbs.Stat(ctx, desc.Digest)
	if err == nil {
		t.Fatalf("unexpected non-error stating deleted blob: %v", d)
	}

	switch err {
	case distribution.ErrBlobUnknown:
		break
	default:
		t.Errorf("Unexpected error type stat-ing deleted manifest: %#v", err)
	}

	// Delete the blob from the dest repo
	err = bs.Delete(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("Unexpected error deleting blob")
	}

	d, err = bs.Stat(ctx, desc.Digest)
	if err == nil {
		t.Fatalf("unexpected non-error stating deleted blob: %v", d)
	}

	switch err {
	case distribution.ErrBlobUnknown:
		break
	default:
		t.Errorf("Unexpected error type stat-ing deleted manifest: %#v", err)
	}
}

// TestLayerUploadZeroLength uploads zero-length
func TestLayerUploadZeroLength(t *testing.T) {
	ctx := context.Background()
	imageName, _ := reference.ParseNamed("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repository.Blobs(ctx)

	simpleUpload(t, bs, []byte{}, digest.DigestSha256EmptyTar)
}

// TestRemoveParentsOnDelete verifies that blob store deletes a directory
// together with blob's data or link when RemoveParentsOnDelete option is
// applied.
func TestRemoveBlobParentsOnDelete(t *testing.T) {
	ctx := context.Background()
	imageName, _ := reference.ParseNamed("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect, RemoveParentsOnDelete)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repository.Blobs(ctx)

	checkBlobParentPath(t, ctx, driver, nil, digest.DigestSha256EmptyTar, false)
	checkBlobParentPath(t, ctx, driver, imageName, digest.DigestSha256EmptyTar, false)

	simpleUpload(t, bs, []byte{}, digest.DigestSha256EmptyTar)

	checkBlobParentPath(t, ctx, driver, nil, digest.DigestSha256EmptyTar, true)
	checkBlobParentPath(t, ctx, driver, imageName, digest.DigestSha256EmptyTar, true)

	// Delete a layer link
	err = bs.Delete(ctx, digest.DigestSha256EmptyTar)
	if err != nil {
		t.Fatalf("Unexpected error deleting blob: %v", err)
	}

	checkBlobParentPath(t, ctx, driver, nil, digest.DigestSha256EmptyTar, true)
	checkBlobParentPath(t, ctx, driver, imageName, digest.DigestSha256EmptyTar, false)

	bd, err := RegistryBlobDeleter(registry)
	if err != nil {
		t.Fatalf("failed to obtain blob deleter: %v", err)
	}
	bd.Delete(ctx, digest.DigestSha256EmptyTar)

	checkBlobParentPath(t, ctx, driver, nil, digest.DigestSha256EmptyTar, false)
	checkBlobParentPath(t, ctx, driver, imageName, digest.DigestSha256EmptyTar, false)
}

// TestBlobEnumeration checks whether enumeration of repository and registry's
// blobs returns proper results.
func TestBlobEnumeration(t *testing.T) {
	ctx := context.Background()
	imageNames := [2]reference.Named{}
	for i, name := range []string{"foo/bar", "baz/gas"} {
		imageNames[i], _ = reference.ParseNamed(name)
	}
	driver := inmemory.New()
	reg, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	// holds a repository objects corresponding to imageNames
	repositories := make([]distribution.Repository, len(imageNames))
	// holds blob store of each repository
	blobStores := make([]distribution.BlobStore, len(imageNames))
	for i, name := range imageNames {
		repositories[i], err = reg.Repository(ctx, name)
		if err != nil {
			t.Fatalf("unexpected error getting repo: %v", err)
		}
		blobStores[i] = repositories[i].Blobs(ctx)
	}
	be, err := RegistryBlobEnumerator(reg)
	if err != nil {
		t.Fatalf("unexpected error getting blob enumerator: %v", err)
	}

	// doEnumeration calls Enumerate method on all repositories and registry's blob store.
	// Additinal arguments represent expected digests for each repository defined.
	doEnumeration := func(expectRegistryBlobs []digest.Digest, expectDigests ...[]digest.Digest) {
		expBlobSets := make([]map[digest.Digest]struct{}, len(imageNames))
		totalBlobSet := make(map[digest.Digest]struct{})
		for i, dgsts := range expectDigests {
			expBlobSets[i] = make(map[digest.Digest]struct{})
			for _, dgst := range dgsts {
				expBlobSets[i][dgst] = struct{}{}
			}
		}
		for _, d := range expectRegistryBlobs {
			totalBlobSet[d] = struct{}{}
		}

		dgsts := make([]digest.Digest, 0, len(totalBlobSet)+1)

		for i, bs := range blobStores {
			err := bs.Enumerate(ctx, func(dgst digest.Digest) error {
				dgsts = append(dgsts, dgst)
				return nil
			})
			if err != io.EOF {
				t.Fatalf("expected io.EOF when enumerating blobs of repository %s, not: %v", imageNames[i], err)
			}
			if len(dgsts) != len(expBlobSets[i]) {
				t.Errorf("got unexpected number of blobs in repository %s (%d != %d)", imageNames[i], len(dgsts), len(expBlobSets[i]))
			}
			for _, d := range dgsts {
				if _, exists := expBlobSets[i][d]; !exists {
					t.Errorf("received unexpected blob digest %s in repository %s", d, imageNames[i])
				}
				delete(expBlobSets[i], d)
			}
			for d := range expBlobSets[i] {
				t.Errorf("expected digest %s not received for repository %s", d, imageNames[i])
			}
			dgsts = dgsts[0:0]
		}

		err := be.Enumerate(ctx, func(dgst digest.Digest) error {
			dgsts = append(dgsts, dgst)
			return nil
		})
		if err != io.EOF {
			t.Fatalf("expected io.EOF when enumerating registry blobs, not: %v", err)
		}
		if len(dgsts) != len(totalBlobSet) {
			t.Errorf("got unexpected number of blobs in registry (%d != %d)", len(dgsts), len(totalBlobSet))
		}
		for _, d := range dgsts {
			if _, exists := totalBlobSet[d]; !exists {
				t.Errorf("received unexpected blob digest %s", d)
			}
			delete(totalBlobSet, d)
		}
		for d := range totalBlobSet {
			t.Errorf("expected digest %s not received", d)
		}
	}

	doEnumeration(
		[]digest.Digest{},
		[]digest.Digest{},
		[]digest.Digest{},
	)

	t.Logf("uploading an empty tarball to repository %s", imageNames[0])
	simpleUpload(t, blobStores[0], []byte{}, digest.DigestSha256EmptyTar)

	doEnumeration(
		[]digest.Digest{digest.DigestSha256EmptyTar},
		[]digest.Digest{digest.DigestSha256EmptyTar},
		[]digest.Digest{},
	)

	t.Logf("uploading a random tarball to repository %s", imageNames[1])
	tarballDgst := uploadRandomTarball(t, ctx, blobStores[1])

	doEnumeration(
		[]digest.Digest{digest.DigestSha256EmptyTar, tarballDgst},
		[]digest.Digest{digest.DigestSha256EmptyTar},
		[]digest.Digest{tarballDgst},
	)

	t.Logf("uploading a random layer to %s repository", imageNames[0])
	layerDgst := uploadRandomLayer(t, ctx, blobStores[0])

	doEnumeration(
		[]digest.Digest{digest.DigestSha256EmptyTar, layerDgst, tarballDgst},
		[]digest.Digest{digest.DigestSha256EmptyTar, layerDgst},
		[]digest.Digest{tarballDgst},
	)

	// delete is performed without parent directory being deleted
	t.Logf("deleting empty layer data from registry")
	bd, err := RegistryBlobDeleter(reg)
	if err != nil {
		t.Fatalf("failed to obtain blob deleter: %v", err)
	}
	err = bd.Delete(ctx, digest.DigestSha256EmptyTar)
	if err != nil {
		t.Fatalf("unexpected error while deleting registry blob: %v", err)
	}
	checkBlobParentPath(t, ctx, driver, nil, digest.DigestSha256EmptyTar, true)
	checkBlobParentPath(t, ctx, driver, imageNames[0], digest.DigestSha256EmptyTar, true)
	checkBlobParentPath(t, ctx, driver, imageNames[1], digest.DigestSha256EmptyTar, false)

	// check that deletion had no effect on digests enumerated
	doEnumeration(
		[]digest.Digest{digest.DigestSha256EmptyTar, layerDgst, tarballDgst},
		[]digest.Digest{digest.DigestSha256EmptyTar, layerDgst},
		[]digest.Digest{tarballDgst},
	)

	// set RemoveParentsOnDelete and delete the layer again
	if r, ok := reg.(*registry); ok {
		RemoveParentsOnDelete(r)
	} else {
		t.Fatalf("failed to cast registry")
	}

	repo, err := reg.Repository(ctx, imageNames[0])
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repo.Blobs(ctx)
	bd, err = RegistryBlobDeleter(reg)
	if err != nil {
		t.Fatalf("failed to obtain blob deleter: %v", err)
	}

	t.Logf("deleting empty layer link directory from %s repository", imageNames[0])
	err = bs.Delete(ctx, digest.DigestSha256EmptyTar)
	if err != nil {
		t.Fatalf("unexpected error while deleting empty layer link: %v", err)
	}
	checkBlobParentPath(t, ctx, driver, nil, digest.DigestSha256EmptyTar, true)
	checkBlobParentPath(t, ctx, driver, imageNames[0], digest.DigestSha256EmptyTar, false)
	checkBlobParentPath(t, ctx, driver, imageNames[1], digest.DigestSha256EmptyTar, false)

	// verify that blob data is still in registry's store
	doEnumeration(
		[]digest.Digest{digest.DigestSha256EmptyTar, layerDgst, tarballDgst},
		[]digest.Digest{layerDgst},
		[]digest.Digest{tarballDgst},
	)

	t.Logf("deleting empty layer directory from registry")
	err = bd.Delete(ctx, digest.DigestSha256EmptyTar)
	if err != nil {
		t.Fatalf("unexpected error while deleting registry blob: %v", err)
	}

	doEnumeration(
		[]digest.Digest{layerDgst, tarballDgst},
		[]digest.Digest{layerDgst},
		[]digest.Digest{tarballDgst},
	)

	checkBlobParentPath(t, ctx, driver, nil, digest.DigestSha256EmptyTar, false)
}

// TestBlobEnumeration checks whether enumeration of repository and registry's
// blobs returns proper results when callback indicates *stop processing*.
func TestBlobStopEnumeration(t *testing.T) {
	const numDigests = 10
	ctx := context.Background()
	imageName, _ := reference.ParseNamed("foo/bar")
	driver := inmemory.New()
	reg, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}

	repo, err := reg.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repo.Blobs(ctx)
	if err != nil {
		t.Fatalf("unexpected error getting blob enumerator: %v", err)
	}
	be, err := RegistryBlobEnumerator(reg)
	if err != nil {
		t.Fatal(err)
	}

	digests := make([]digest.Digest, numDigests)
	for i := 0; i < numDigests; i++ {
		var dgst digest.Digest
		if i%2 == 0 {
			dgst = uploadRandomLayer(t, ctx, bs)
			t.Logf("uploaded a new layer with digest %s", dgst.String())
		} else {
			dgst = uploadRandomTarball(t, ctx, bs)
			t.Logf("uploaded a new tarsum layer with digest %s", dgst.String())
		}
		digests[i] = dgst
	}

	testEnum := func(be distribution.BlobEnumerator, stopAfter int, withEOF bool, allDigests []digest.Digest) {
		ret := "error"
		if withEOF {
			ret = "EOF"
		}
		testCtx := fmt.Sprintf("%T: stopAfter=%d with %s", be, stopAfter, ret)

		dgsts := []digest.Digest{}
		err := be.Enumerate(ctx, func(dgst digest.Digest) error {
			if err := dgst.Validate(); err != nil {
				t.Errorf("%s: ingest callback called with invalid digest %q: %v", testCtx, dgst.String(), err)
			}
			if len(dgsts) >= stopAfter {
				t.Errorf("%s: ingest callback called again after returning a request for stop (n=%d)", testCtx, len(dgsts))
			}
			dgsts = append(dgsts, dgst)
			if len(dgsts) >= stopAfter {
				if withEOF {
					return io.EOF
				}
				return fmt.Errorf("don't call us again")
			}
			return nil
		})

		if err == io.EOF && len(dgsts) < len(allDigests) {
			t.Errorf("%s: got unexpected io.EOF", testCtx)
		} else if len(dgsts) > len(allDigests) && err != io.EOF {
			t.Errorf("%s: expected io.EOF, got: %v", testCtx, err)
		}

		if len(dgsts) != stopAfter {
			t.Errorf("%s: ingest function called %d times instead of %d", testCtx, len(dgsts), stopAfter)
		}

		dgstSet := map[digest.Digest]struct{}{}
		for _, d := range allDigests {
			if _, exists := dgstSet[d]; exists {
				t.Errorf("%s: received duplicate digest %q", testCtx, d.String())
			}
			dgstSet[d] = struct{}{}
		}

		for _, dgst := range allDigests {
			delete(dgstSet, dgst)
		}

		for dgst := range dgstSet {
			t.Errorf("%s: got unexpected digest %q", testCtx, dgst.String())
		}
	}

	// enumerate linked blob store
	testEnum(bs, 10, true, digests)
	testEnum(bs, 4, true, digests)
	testEnum(bs, 1, true, digests)

	testEnum(bs, 10, false, digests)
	testEnum(bs, 4, false, digests)
	testEnum(bs, 1, false, digests)

	testEnum(be, 10, true, digests)
	testEnum(be, 4, true, digests)
	testEnum(be, 1, true, digests)

	testEnum(be, 10, false, digests)
	testEnum(be, 4, false, digests)
	testEnum(be, 1, false, digests)
}

func simpleUpload(t *testing.T, bs distribution.BlobIngester, blob []byte, expectedDigest digest.Digest) {
	ctx := context.Background()
	wr, err := bs.Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting upload: %v", err)
	}

	nn, err := io.Copy(wr, bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("error copying into blob writer: %v", err)
	}

	if nn != 0 {
		t.Fatalf("unexpected number of bytes copied: %v > 0", nn)
	}

	dgst, err := digest.FromReader(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("error getting digest: %v", err)
	}

	if dgst != expectedDigest {
		// sanity check on zero digest
		t.Fatalf("digest not as expected: %v != %v", dgst, expectedDigest)
	}

	desc, err := wr.Commit(ctx, distribution.Descriptor{Digest: dgst})
	if err != nil {
		t.Fatalf("unexpected error committing write: %v", err)
	}

	if desc.Digest != dgst {
		t.Fatalf("unexpected digest: %v != %v", desc.Digest, dgst)
	}
}

// seekerSize seeks to the end of seeker, checks the size and returns it to
// the original state, returning the size. The state of the seeker should be
// treated as unknown if an error is returned.
func seekerSize(seeker io.ReadSeeker) (int64, error) {
	current, err := seeker.Seek(0, os.SEEK_CUR)
	if err != nil {
		return 0, err
	}

	end, err := seeker.Seek(0, os.SEEK_END)
	if err != nil {
		return 0, err
	}

	resumed, err := seeker.Seek(current, os.SEEK_SET)
	if err != nil {
		return 0, err
	}

	if resumed != current {
		return 0, fmt.Errorf("error returning seeker to original state, could not seek back to original location")
	}

	return end, nil
}

// addBlob simply consumes the reader and inserts into the blob service,
// returning a descriptor on success.
func addBlob(ctx context.Context, bs distribution.BlobIngester, desc distribution.Descriptor, rd io.Reader) (distribution.Descriptor, error) {
	wr, err := bs.Create(ctx)
	if err != nil {
		return distribution.Descriptor{}, err
	}
	defer wr.Cancel(ctx)

	if nn, err := io.Copy(wr, rd); err != nil {
		return distribution.Descriptor{}, err
	} else if nn != desc.Size {
		return distribution.Descriptor{}, fmt.Errorf("incorrect number of bytes copied: %v != %v", nn, desc.Size)
	}

	return wr.Commit(ctx, desc)
}

func createRandomData() (io.ReadSeeker, int64, error) {
	fileSize := mrand.Int63n(1<<20) + 1<<20

	randomData := make([]byte, fileSize)
	// Fill up the buffer with some random data.
	n, err := rand.Read(randomData)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fill buffer with random data: %v", err)
	}
	if n != len(randomData) {
		return nil, 0, fmt.Errorf("short read creating random reader: %v bytes != %v bytes", n, len(randomData))
	}

	return bytes.NewReader(randomData), fileSize, nil
}

func uploadRandomLayer(t *testing.T, ctx context.Context, bi distribution.BlobIngester) digest.Digest {
	dr, size, err := createRandomData()
	if err != nil {
		t.Fatalf("failed to create random file: %v", err)
	}

	h := sha256.New()
	rd := io.TeeReader(dr, h)
	blobUpload, err := bi.Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}
	nn, err := io.Copy(blobUpload, rd)
	if err != nil {
		t.Fatalf("unexpected error uploading layer data: %v", err)
	}
	if nn != size {
		t.Fatalf("layer data write incomplete")
	}
	dgst := digest.NewDigest("sha256", h)
	_, err = blobUpload.Commit(ctx, distribution.Descriptor{Digest: dgst})
	if err != nil {
		t.Fatalf("unexpected error finishing layer upload: %v", err)
	}
	return dgst
}

func uploadRandomTarball(t *testing.T, ctx context.Context, bi distribution.BlobIngester) digest.Digest {
	randomDataReader, tarSumStr, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random reader: %v", err)
	}
	dgst := digest.Digest(tarSumStr)
	if err != nil {
		t.Fatalf("error allocating upload store: %v", err)
	}

	randomLayerSize, err := seekerSize(randomDataReader)
	if err != nil {
		t.Fatalf("error getting seeker size for random layer: %v", err)
	}

	_, err = addBlob(ctx, bi, distribution.Descriptor{
		Digest:    dgst,
		MediaType: "application/octet-stream",
		Size:      randomLayerSize,
	}, randomDataReader)
	if err != nil {
		t.Fatalf("failed to add blob: %v", err)
	}
	return dgst
}

// checkBlobParentPath asserts that a directory containing blob's link or data
// does (not) exist. If repoName is given, link path in _layers directory of
// that repository will be checked. Registry's blob store will be checked
// otherwise.
func checkBlobParentPath(t *testing.T, ctx context.Context, driver *inmemory.Driver, repo reference.Named, dgst digest.Digest, expectExistent bool) {
	var (
		blobPath string
		err      error
	)

	if repo != nil {
		blobPath, err = pathFor(layerLinkPathSpec{name: repo.Name(), digest: dgst})
		if err != nil {
			t.Fatalf("failed to get layer link path for repo=%s, digest=%s: %v", repo.Name(), dgst.String(), err)
		}
		blobPath = path.Dir(blobPath)
	} else {
		blobPath, err = pathFor(blobPathSpec{digest: dgst})
		if err != nil {
			t.Fatalf("failed to get blob path for digest %s: %v", dgst.String(), err)
		}
	}

	parentExists, err := exists(ctx, driver, blobPath)
	if err != nil {
		t.Fatalf("failed to check whether path %s exists: %v", blobPath, err)
	}
	if expectExistent && !parentExists {
		t.Errorf("expected blob path %s to exist", blobPath)
	} else if !expectExistent && parentExists {
		t.Errorf("expected blob path %s not to exist", blobPath)
	}
}

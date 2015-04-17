package storage

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage/cache"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/testutil"
	"golang.org/x/net/context"
)

// TestSimpleLayerUpload covers the layer upload process, exercising common
// error paths that might be seen during an upload.
func TestSimpleLayerUpload(t *testing.T) {
	randomDataReader, tarSumStr, err := testutil.CreateRandomTarFile()

	if err != nil {
		t.Fatalf("error creating random reader: %v", err)
	}

	dgst := digest.Digest(tarSumStr)

	if err != nil {
		t.Fatalf("error allocating upload store: %v", err)
	}

	ctx := context.Background()
	imageName := "foo/bar"
	driver := inmemory.New()
	registry := NewRegistryWithDriver(driver, cache.NewInMemoryLayerInfoCache())
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	ls := repository.Layers()

	h := sha256.New()
	rd := io.TeeReader(randomDataReader, h)

	layerUpload, err := ls.Upload()

	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}

	// Cancel the upload then restart it
	if err := layerUpload.Cancel(); err != nil {
		t.Fatalf("unexpected error during upload cancellation: %v", err)
	}

	// Do a resume, get unknown upload
	layerUpload, err = ls.Resume(layerUpload.UUID())
	if err != distribution.ErrLayerUploadUnknown {
		t.Fatalf("unexpected error resuming upload, should be unkown: %v", err)
	}

	// Restart!
	layerUpload, err = ls.Upload()
	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}

	// Get the size of our random tarfile
	randomDataSize, err := seekerSize(randomDataReader)
	if err != nil {
		t.Fatalf("error getting seeker size of random data: %v", err)
	}

	nn, err := io.Copy(layerUpload, rd)
	if err != nil {
		t.Fatalf("unexpected error uploading layer data: %v", err)
	}

	if nn != randomDataSize {
		t.Fatalf("layer data write incomplete")
	}

	offset, err := layerUpload.Seek(0, os.SEEK_CUR)
	if err != nil {
		t.Fatalf("unexpected error seeking layer upload: %v", err)
	}

	if offset != nn {
		t.Fatalf("layerUpload not updated with correct offset: %v != %v", offset, nn)
	}
	layerUpload.Close()

	// Do a resume, for good fun
	layerUpload, err = ls.Resume(layerUpload.UUID())
	if err != nil {
		t.Fatalf("unexpected error resuming upload: %v", err)
	}

	sha256Digest := digest.NewDigest("sha256", h)
	layer, err := layerUpload.Finish(dgst)

	if err != nil {
		t.Fatalf("unexpected error finishing layer upload: %v", err)
	}

	// After finishing an upload, it should no longer exist.
	if _, err := ls.Resume(layerUpload.UUID()); err != distribution.ErrLayerUploadUnknown {
		t.Fatalf("expected layer upload to be unknown, got %v", err)
	}

	// Test for existence.
	exists, err := ls.Exists(layer.Digest())
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v", err)
	}

	if !exists {
		t.Fatalf("layer should now exist")
	}

	h.Reset()
	nn, err = io.Copy(h, layer)
	if err != nil {
		t.Fatalf("error reading layer: %v", err)
	}

	if nn != randomDataSize {
		t.Fatalf("incorrect read length")
	}

	if digest.NewDigest("sha256", h) != sha256Digest {
		t.Fatalf("unexpected digest from uploaded layer: %q != %q", digest.NewDigest("sha256", h), sha256Digest)
	}
}

// TestSimpleLayerRead just creates a simple layer file and ensures that basic
// open, read, seek, read works. More specific edge cases should be covered in
// other tests.
func TestSimpleLayerRead(t *testing.T) {
	ctx := context.Background()
	imageName := "foo/bar"
	driver := inmemory.New()
	registry := NewRegistryWithDriver(driver, cache.NewInMemoryLayerInfoCache())
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	ls := repository.Layers()

	randomLayerReader, tarSumStr, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random data: %v", err)
	}

	dgst := digest.Digest(tarSumStr)

	// Test for existence.
	exists, err := ls.Exists(dgst)
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v", err)
	}

	if exists {
		t.Fatalf("layer should not exist")
	}

	// Try to get the layer and make sure we get a not found error
	layer, err := ls.Fetch(dgst)
	if err == nil {
		t.Fatalf("error expected fetching unknown layer")
	}

	switch err.(type) {
	case distribution.ErrUnknownLayer:
		err = nil
	default:
		t.Fatalf("unexpected error fetching non-existent layer: %v", err)
	}

	randomLayerDigest, err := writeTestLayer(driver, defaultPathMapper, imageName, dgst, randomLayerReader)
	if err != nil {
		t.Fatalf("unexpected error writing test layer: %v", err)
	}

	randomLayerSize, err := seekerSize(randomLayerReader)
	if err != nil {
		t.Fatalf("error getting seeker size for random layer: %v", err)
	}

	layer, err = ls.Fetch(dgst)
	if err != nil {
		t.Fatal(err)
	}
	defer layer.Close()

	// Now check the sha digest and ensure its the same
	h := sha256.New()
	nn, err := io.Copy(h, layer)
	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error copying to hash: %v", err)
	}

	if nn != randomLayerSize {
		t.Fatalf("stored incorrect number of bytes in layer: %d != %d", nn, randomLayerSize)
	}

	sha256Digest := digest.NewDigest("sha256", h)
	if sha256Digest != randomLayerDigest {
		t.Fatalf("fetched digest does not match: %q != %q", sha256Digest, randomLayerDigest)
	}

	// Now seek back the layer, read the whole thing and check against randomLayerData
	offset, err := layer.Seek(0, os.SEEK_SET)
	if err != nil {
		t.Fatalf("error seeking layer: %v", err)
	}

	if offset != 0 {
		t.Fatalf("seek failed: expected 0 offset, got %d", offset)
	}

	p, err := ioutil.ReadAll(layer)
	if err != nil {
		t.Fatalf("error reading all of layer: %v", err)
	}

	if len(p) != int(randomLayerSize) {
		t.Fatalf("layer data read has different length: %v != %v", len(p), randomLayerSize)
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

// TestLayerUploadZeroLength uploads zero-length
func TestLayerUploadZeroLength(t *testing.T) {
	ctx := context.Background()
	imageName := "foo/bar"
	driver := inmemory.New()
	registry := NewRegistryWithDriver(driver, cache.NewInMemoryLayerInfoCache())
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	ls := repository.Layers()

	upload, err := ls.Upload()
	if err != nil {
		t.Fatalf("unexpected error starting upload: %v", err)
	}

	io.Copy(upload, bytes.NewReader([]byte{}))

	dgst, err := digest.FromReader(bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("error getting zero digest: %v", err)
	}

	if dgst != digest.DigestSha256EmptyTar {
		// sanity check on zero digest
		t.Fatalf("digest not as expected: %v != %v", dgst, digest.DigestTarSumV1EmptyTar)
	}

	layer, err := upload.Finish(dgst)
	if err != nil {
		t.Fatalf("unexpected error finishing upload: %v", err)
	}

	if layer.Digest() != dgst {
		t.Fatalf("unexpected digest: %v != %v", layer.Digest(), dgst)
	}
}

// writeRandomLayer creates a random layer under name and tarSum using driver
// and pathMapper. An io.ReadSeeker with the data is returned, along with the
// sha256 hex digest.
func writeRandomLayer(driver storagedriver.StorageDriver, pathMapper *pathMapper, name string) (rs io.ReadSeeker, tarSum digest.Digest, sha256digest digest.Digest, err error) {
	reader, tarSumStr, err := testutil.CreateRandomTarFile()
	if err != nil {
		return nil, "", "", err
	}

	tarSum = digest.Digest(tarSumStr)

	// Now, actually create the layer.
	randomLayerDigest, err := writeTestLayer(driver, pathMapper, name, tarSum, ioutil.NopCloser(reader))

	if _, err := reader.Seek(0, os.SEEK_SET); err != nil {
		return nil, "", "", err
	}

	return reader, tarSum, randomLayerDigest, err
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

// createTestLayer creates a simple test layer in the provided driver under
// tarsum dgst, returning the sha256 digest location. This is implemented
// piecemeal and should probably be replaced by the uploader when it's ready.
func writeTestLayer(driver storagedriver.StorageDriver, pathMapper *pathMapper, name string, dgst digest.Digest, content io.Reader) (digest.Digest, error) {
	h := sha256.New()
	rd := io.TeeReader(content, h)

	p, err := ioutil.ReadAll(rd)

	if err != nil {
		return "", nil
	}

	blobDigestSHA := digest.NewDigest("sha256", h)

	blobPath, err := pathMapper.path(blobDataPathSpec{
		digest: dgst,
	})

	if err := driver.PutContent(blobPath, p); err != nil {
		return "", err
	}

	if err != nil {
		return "", err
	}

	layerLinkPath, err := pathMapper.path(layerLinkPathSpec{
		name:   name,
		digest: dgst,
	})

	if err != nil {
		return "", err
	}

	if err := driver.PutContent(layerLinkPath, []byte(dgst)); err != nil {
		return "", nil
	}

	return blobDigestSHA, err
}

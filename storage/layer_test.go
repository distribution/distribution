package storage

import (
	"archive/tar"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	mrand "math/rand"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/pkg/tarsum"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/inmemory"
)

// TestSimpleLayerUpload covers the layer upload process, exercising common
// error paths that might be seen during an upload.
func TestSimpleLayerUpload(t *testing.T) {
	randomDataReader, tarSum, err := createRandomReader()

	if err != nil {
		t.Fatalf("error creating random reader: %v", err)
	}

	uploadStore, err := newTemporaryLocalFSLayerUploadStore()
	if err != nil {
		t.Fatalf("error allocating upload store: %v", err)
	}

	imageName := "foo/bar"
	driver := inmemory.New()

	ls := &layerStore{
		driver: driver,
		pathMapper: &pathMapper{
			root:    "/storage/testing",
			version: storagePathVersion,
		},
		uploadStore: uploadStore,
	}

	h := sha256.New()
	rd := io.TeeReader(randomDataReader, h)

	layerUpload, err := ls.Upload(imageName, tarSum)

	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}

	// Cancel the upload then restart it
	if err := layerUpload.Cancel(); err != nil {
		t.Fatalf("unexpected error during upload cancellation: %v", err)
	}

	// Do a resume, get unknown upload
	layerUpload, err = ls.Resume(imageName, tarSum, layerUpload.UUID())
	if err != ErrLayerUploadUnknown {
		t.Fatalf("unexpected error resuming upload, should be unkown: %v", err)
	}

	// Restart!
	layerUpload, err = ls.Upload(imageName, tarSum)
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

	if layerUpload.Offset() != nn {
		t.Fatalf("layerUpload not updated with correct offset: %v != %v", layerUpload.Offset(), nn)
	}
	layerUpload.Close()

	// Do a resume, for good fun
	layerUpload, err = ls.Resume(imageName, tarSum, layerUpload.UUID())
	if err != nil {
		t.Fatalf("unexpected error resuming upload: %v", err)
	}

	digest := NewDigest("sha256", h)
	layer, err := layerUpload.Finish(randomDataSize, string(digest))

	if err != nil {
		t.Fatalf("unexpected error finishing layer upload: %v", err)
	}

	// After finishing an upload, it should no longer exist.
	if _, err := ls.Resume(imageName, tarSum, layerUpload.UUID()); err != ErrLayerUploadUnknown {
		t.Fatalf("expected layer upload to be unknown, got %v", err)
	}

	// Test for existence.
	exists, err := ls.Exists(layer.TarSum())
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

	if NewDigest("sha256", h) != digest {
		t.Fatalf("unexpected digest from uploaded layer: %q != %q", NewDigest("sha256", h), digest)
	}
}

// TestSimpleLayerRead just creates a simple layer file and ensures that basic
// open, read, seek, read works. More specific edge cases should be covered in
// other tests.
func TestSimpleLayerRead(t *testing.T) {
	imageName := "foo/bar"
	driver := inmemory.New()
	ls := &layerStore{
		driver: driver,
		pathMapper: &pathMapper{
			root:    "/storage/testing",
			version: storagePathVersion,
		},
	}

	randomLayerReader, tarSum, err := createRandomReader()
	if err != nil {
		t.Fatalf("error creating random data: %v", err)
	}

	// Test for existence.
	exists, err := ls.Exists(tarSum)
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v", err)
	}

	if exists {
		t.Fatalf("layer should not exist")
	}

	// Try to get the layer and make sure we get a not found error
	layer, err := ls.Fetch(tarSum)
	if err == nil {
		t.Fatalf("error expected fetching unknown layer")
	}

	if err != ErrLayerUnknown {
		t.Fatalf("unexpected error fetching non-existent layer: %v", err)
	} else {
		err = nil
	}

	randomLayerDigest, err := writeTestLayer(driver, ls.pathMapper, imageName, tarSum, randomLayerReader)
	if err != nil {
		t.Fatalf("unexpected error writing test layer: %v", err)
	}

	randomLayerSize, err := seekerSize(randomLayerReader)
	if err != nil {
		t.Fatalf("error getting seeker size for random layer: %v", err)
	}

	layer, err = ls.Fetch(tarSum)
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

	digest := NewDigest("sha256", h)
	if digest != randomLayerDigest {
		t.Fatalf("fetched digest does not match: %q != %q", digest, randomLayerDigest)
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

func TestLayerReaderSeek(t *testing.T) {
	// TODO(stevvooe): Ensure that all relative seeks work as advertised.
	// Readers must close and re-open on command. This is important to support
	// resumable and concurrent downloads via HTTP range requests.
}

// TestLayerReadErrors covers the various error return type for different
// conditions that can arise when reading a layer.
func TestLayerReadErrors(t *testing.T) {
	// TODO(stevvooe): We need to cover error return types, driven by the
	// errors returned via the HTTP API. For now, here is a incomplete list:
	//
	// 	1. Layer Not Found: returned when layer is not found or access is
	//        denied.
	//	2. Layer Unavailable: returned when link references are unresolved,
	//     but layer is known to the registry.
	//  3. Layer Invalid: This may more split into more errors, but should be
	//     returned when name or tarsum does not reference a valid error. We
	//     may also need something to communication layer verification errors
	//     for the inline tarsum check.
	//	4. Timeout: timeouts to backend. Need to better understand these
	//     failure cases and how the storage driver propagates these errors
	//     up the stack.
}

// writeRandomLayer creates a random layer under name and tarSum using driver
// and pathMapper. An io.ReadSeeker with the data is returned, along with the
// sha256 hex digest.
func writeRandomLayer(driver storagedriver.StorageDriver, pathMapper *pathMapper, name string) (rs io.ReadSeeker, tarSum string, digest Digest, err error) {
	reader, tarSum, err := createRandomReader()
	if err != nil {
		return nil, "", "", err
	}

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

// createRandomReader returns a random read seeker and its tarsum. The
// returned content will be a valid tar file with a random number of files and
// content.
func createRandomReader() (rs io.ReadSeeker, tarSum string, err error) {
	nFiles := mrand.Intn(10) + 10
	target := &bytes.Buffer{}
	wr := tar.NewWriter(target)

	// Perturb this on each iteration of the loop below.
	header := &tar.Header{
		Mode:       0644,
		ModTime:    time.Now(),
		Typeflag:   tar.TypeReg,
		Uname:      "randocalrissian",
		Gname:      "cloudcity",
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
	}

	for fileNumber := 0; fileNumber < nFiles; fileNumber++ {
		fileSize := mrand.Int63n(1<<20) + 1<<20

		header.Name = fmt.Sprint(fileNumber)
		header.Size = fileSize

		if err := wr.WriteHeader(header); err != nil {
			return nil, "", err
		}

		randomData := make([]byte, fileSize)

		// Fill up the buffer with some random data.
		n, err := rand.Read(randomData)

		if n != len(randomData) {
			return nil, "", fmt.Errorf("short read creating random reader: %v bytes != %v bytes", n, len(randomData))
		}

		if err != nil {
			return nil, "", err
		}

		nn, err := io.Copy(wr, bytes.NewReader(randomData))
		if nn != fileSize {
			return nil, "", fmt.Errorf("short copy writing random file to tar")
		}

		if err != nil {
			return nil, "", err
		}

		if err := wr.Flush(); err != nil {
			return nil, "", err
		}
	}

	if err := wr.Close(); err != nil {
		return nil, "", err
	}

	reader := bytes.NewReader(target.Bytes())

	// A tar builder that supports tarsum inline calculation would be awesome
	// here.
	ts, err := tarsum.NewTarSum(reader, true, tarsum.Version1)
	if err != nil {
		return nil, "", err
	}

	nn, err := io.Copy(ioutil.Discard, ts)
	if nn != int64(len(target.Bytes())) {
		return nil, "", fmt.Errorf("short copy when getting tarsum of random layer: %v != %v", nn, len(target.Bytes()))
	}

	if err != nil {
		return nil, "", err
	}

	return bytes.NewReader(target.Bytes()), ts.Sum(nil), nil
}

// createTestLayer creates a simple test layer in the provided driver under
// tarsum, returning the string digest. This is implemented peicemeal and
// should probably be replaced by the uploader when it's ready.
func writeTestLayer(driver storagedriver.StorageDriver, pathMapper *pathMapper, name, tarSum string, content io.Reader) (Digest, error) {
	h := sha256.New()
	rd := io.TeeReader(content, h)

	p, err := ioutil.ReadAll(rd)

	if err != nil {
		return "", nil
	}

	digest := NewDigest("sha256", h)

	blobPath, err := pathMapper.path(blobPathSpec{
		alg:    digest.Algorithm(),
		digest: digest.Hex(),
	})

	if err := driver.PutContent(blobPath, p); err != nil {
		return "", err
	}

	layerIndexLinkPath, err := pathMapper.path(layerIndexLinkPathSpec{
		tarSum: tarSum,
	})

	if err != nil {
		return "", err
	}

	layerLinkPath, err := pathMapper.path(layerLinkPathSpec{
		name:   name,
		tarSum: tarSum,
	})

	if err != nil {
		return "", err
	}

	if err != nil {
		return "", err
	}

	if err := driver.PutContent(layerLinkPath, []byte(string(NewDigest("sha256", h)))); err != nil {
		return "", nil
	}

	if err = driver.PutContent(layerIndexLinkPath, []byte(name)); err != nil {
		return "", nil
	}

	return NewDigest("sha256", h), err
}

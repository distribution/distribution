package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// TestWriteSeek tests that the current file size can be
// obtained using Seek
func TestWriteSeek(t *testing.T) {
	ctx := context.Background()
	imageName, _ := reference.WithName("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repository.Blobs(ctx)

	blobUpload, err := bs.Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}
	contents := []byte{1, 2, 3}
	if _, err := blobUpload.Write(contents); err != nil {
		t.Fatalf("unexpected error writing contents: %v", err)
	}
	blobUpload.Close()
	offset := blobUpload.Size()
	if offset != int64(len(contents)) {
		t.Fatalf("unexpected value for blobUpload offset:  %v != %v", offset, len(contents))
	}
}

// TestBlobUploadCloseAfterCancel checks that closing a cancelled upload, as
// the HTTP handler does with a deferred Close, does not write the hash state
// back into the upload directory that Cancel removed.
func TestBlobUploadCloseAfterCancel(t *testing.T) {
	ctx := context.Background()
	imageName, _ := reference.WithName("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repository.Blobs(ctx)

	blobUpload, err := bs.Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting layer upload: %s", err)
	}
	if _, err := blobUpload.Write([]byte{1, 2, 3}); err != nil {
		t.Fatalf("unexpected error writing contents: %v", err)
	}
	uploadPath := path.Dir(blobUpload.(*blobWriter).path)

	if err := blobUpload.Cancel(ctx); err != nil {
		t.Fatalf("unexpected error during upload cancellation: %v", err)
	}
	if err := blobUpload.Close(); err != nil {
		t.Fatalf("unexpected error closing cancelled upload: %v", err)
	}

	if paths, err := driver.List(ctx, uploadPath); err == nil {
		t.Fatalf("upload path recreated after cancel and close: %v", paths)
	}
}

// TestBlobWriterCancel checks cancellation before and after the driver writer
// has been closed or committed, including deferred and repeated cancellation.
func TestBlobWriterCancel(t *testing.T) {
	for _, backend := range []string{"inmemory", "filesystem"} {
		for _, resumable := range []bool{false, true} {
			for _, size := range []int{0, 1, 11} {
				for _, state := range []string{"open", "closed", "wrong digest", "empty digest", "committed"} {
					t.Run(fmt.Sprintf("%s/resumable=%t/size=%d/%s", backend, resumable, size, state), func(t *testing.T) {
						ctx := context.Background()
						var driver storagedriver.StorageDriver = inmemory.New()
						if backend == "filesystem" {
							driver = filesystem.New(filesystem.DriverParameters{RootDirectory: t.TempDir(), MaxThreads: 25})
						}
						var options []RegistryOption
						if !resumable {
							options = append(options, DisableDigestResumption)
						}
						cleanupDriver := &uploadCleanupErrorDriver{StorageDriver: driver}
						reg, err := NewRegistry(ctx, cleanupDriver, options...)
						if err != nil {
							t.Fatal(err)
						}
						name, _ := reference.WithName("foo/bar")
						repo, err := reg.Repository(ctx, name)
						if err != nil {
							t.Fatal(err)
						}
						bs := repo.Blobs(ctx)
						upload, err := bs.Create(ctx)
						if err != nil {
							t.Fatal(err)
						}
						defer upload.Close()
						bw := upload.(*blobWriter)
						data := bytes.Repeat([]byte("x"), size)
						if _, err := upload.Write(data); err != nil {
							t.Fatal(err)
						}
						switch state {
						case "closed":
							if err := upload.Close(); err != nil {
								t.Fatalf("Close: %v", err)
							}
						case "wrong digest", "empty digest", "committed":
							dgst := digest.FromBytes(data)
							switch state {
							case "wrong digest":
								dgst = digest.FromString("different payload")
							case "empty digest":
								dgst = ""
							}
							_, err := upload.Commit(ctx, v1.Descriptor{Digest: dgst})
							if (err == nil) != (state == "committed") {
								t.Fatalf("Commit in state %s: %v", state, err)
							}
						}
						if state == "committed" {
							cleanupDriver.err = errors.New("backend unavailable after Commit")
						}
						for range 2 {
							if err := upload.Cancel(ctx); err != nil {
								t.Errorf("Cancel returned %v", err)
							}
							if _, err := driver.Stat(ctx, path.Dir(bw.path)); err == nil {
								t.Error("upload directory remains after Cancel")
							} else if _, ok := err.(storagedriver.PathNotFoundError); !ok {
								t.Fatalf("Stat upload directory: %v", err)
							}
							resumed, err := bs.Resume(ctx, upload.ID())
							if resumed != nil {
								_ = resumed.Close()
							}
							if err != distribution.ErrBlobUploadUnknown {
								t.Errorf("Resume after Cancel returned %v, want ErrBlobUploadUnknown", err)
							}
							if state == "committed" {
								got, err := bs.Get(ctx, digest.FromBytes(data))
								if err != nil || !bytes.Equal(got, data) {
									t.Errorf("committed blob after Cancel = %q, %v; want %q", got, err, data)
								}
							}
						}
					})
				}
			}
		}
	}
}

// stagedUploadDriver models data, such as S3 multipart parts, that Delete
// cannot remove. Only cancelling or committing the driver writer releases it.
type stagedUploadDriver struct {
	storagedriver.StorageDriver
	staged       map[string]int64
	writers      []*stagedUploadWriter
	cancelClosed bool
	reopenErr    error
	cancelErr    error
}

func (d *stagedUploadDriver) Writer(ctx context.Context, path string, appendMode bool) (storagedriver.FileWriter, error) {
	if appendMode && d.reopenErr != nil {
		return nil, d.reopenErr
	}
	w, err := d.StorageDriver.Writer(ctx, path, appendMode)
	if err != nil {
		return nil, err
	}
	writer := &stagedUploadWriter{FileWriter: w, driver: d, path: path, reopened: appendMode}
	d.writers = append(d.writers, writer)
	return writer, nil
}

type stagedUploadWriter struct {
	storagedriver.FileWriter
	driver   *stagedUploadDriver
	path     string
	reopened bool
	closes   int
}

func (w *stagedUploadWriter) Write(p []byte) (int, error) {
	n, err := w.FileWriter.Write(p)
	w.driver.staged[w.path] += int64(n)
	return n, err
}

func (w *stagedUploadWriter) Close() error {
	w.closes++
	return w.FileWriter.Close()
}

func (w *stagedUploadWriter) Cancel(ctx context.Context) error {
	if w.reopened && w.driver.cancelErr != nil {
		return w.driver.cancelErr
	}
	var err error
	if w.driver.cancelClosed {
		// Like GCS, allow cancellation even after the writer has closed.
		err = w.driver.StorageDriver.Delete(ctx, w.path)
	} else {
		err = w.FileWriter.Cancel(ctx)
	}
	if err != nil {
		return err
	}
	delete(w.driver.staged, w.path)
	return nil
}

func (w *stagedUploadWriter) Commit(ctx context.Context) error {
	if err := w.FileWriter.Commit(ctx); err != nil {
		return err
	}
	delete(w.driver.staged, w.path)
	return nil
}

func TestBlobWriterCancelStagedData(t *testing.T) {
	errUnavailable := errors.New("storage unavailable")
	for _, resumable := range []bool{false, true} {
		for _, tc := range []struct {
			name         string
			cancelClosed bool
			reopenErr    error
			cancelErr    error
			absentData   bool
		}{
			{name: "closed"},
			{name: "absent data", absentData: true},
			{name: "reopen error", reopenErr: errUnavailable},
			{name: "cancel error", cancelErr: errUnavailable},
			{name: "cancel closed", cancelClosed: true, reopenErr: errUnavailable},
		} {
			t.Run(fmt.Sprintf("resumable=%t/%s", resumable, tc.name), func(t *testing.T) {
				ctx := context.Background()
				driver := &stagedUploadDriver{StorageDriver: inmemory.New(), staged: make(map[string]int64), cancelClosed: tc.cancelClosed}
				var options []RegistryOption
				if !resumable {
					options = append(options, DisableDigestResumption)
				}
				reg, err := NewRegistry(ctx, driver, options...)
				if err != nil {
					t.Fatal(err)
				}
				name, _ := reference.WithName("foo/bar")
				repo, err := reg.Repository(ctx, name)
				if err != nil {
					t.Fatal(err)
				}
				bs := repo.Blobs(ctx)
				upload, err := bs.Create(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer upload.Close()
				data := []byte("staged bytes")
				if _, err := upload.Write(data); err != nil {
					t.Fatal(err)
				}
				if err := upload.Close(); err != nil {
					t.Fatal(err)
				}
				driver.reopenErr, driver.cancelErr = tc.reopenErr, tc.cancelErr
				wantErr := errUnavailable
				if tc.cancelClosed || (tc.reopenErr == nil && tc.cancelErr == nil) {
					wantErr = nil
				}
				if tc.absentData {
					dataPath := upload.(*blobWriter).path
					if err := driver.StorageDriver.Delete(ctx, dataPath); err != nil {
						t.Fatal(err)
					}
					delete(driver.staged, dataPath)
					// Model drivers that cannot reopen data which is already gone.
					driver.reopenErr = storagedriver.PathNotFoundError{Path: dataPath}
					if _, err := driver.Stat(ctx, path.Dir(dataPath)); err != nil {
						t.Fatalf("staging metadata before Cancel: %v", err)
					}
				}
				writers := len(driver.writers)
				err = upload.Cancel(ctx)
				if err != wantErr {
					if tc.cancelClosed {
						t.Errorf("Cancel on a closed-capable writer returned %v", err)
					} else {
						t.Errorf("Cancel error = %v, want %v", err, wantErr)
					}
				}
				for _, writer := range driver.writers[writers:] {
					if writer.closes != 1 {
						t.Errorf("reopened writer closed %d times, want 1", writer.closes)
					}
				}
				if tc.absentData {
					if _, err := driver.Stat(ctx, path.Dir(upload.(*blobWriter).path)); err == nil {
						t.Error("upload directory remains after Cancel")
					} else if _, ok := err.(storagedriver.PathNotFoundError); !ok {
						t.Errorf("upload directory after Cancel: %v", err)
					}
					resumed, err := bs.Resume(ctx, upload.ID())
					if resumed != nil {
						defer resumed.Close()
					}
					if err != distribution.ErrBlobUploadUnknown {
						t.Errorf("Resume after Cancel returned %v, want ErrBlobUploadUnknown", err)
					}
					return
				}
				if err != nil {
					if staged := driver.staged[upload.(*blobWriter).path]; staged != int64(len(data)) {
						t.Errorf("staged bytes after failed Cancel = %d, want %d", staged, len(data))
					}
					driver.reopenErr, driver.cancelErr = nil, nil
					resumed, resumeErr := bs.Resume(ctx, upload.ID())
					if resumeErr != nil {
						t.Fatalf("Resume after failed Cancel: %v (Cancel: %v)", resumeErr, err)
					}
					defer resumed.Close()
					if resumed.Size() != int64(len(data)) {
						t.Errorf("resumed size = %d, want %d", resumed.Size(), len(data))
					}
				} else if staged := driver.staged[upload.(*blobWriter).path]; staged != 0 {
					t.Errorf("staged bytes after successful Cancel = %d, want 0", staged)
				}
			})
		}
	}
}

type cancelErrorWriter struct {
	storagedriver.FileWriter
	err error
}

func (w *cancelErrorWriter) Cancel(ctx context.Context) error {
	if w.err != nil {
		return w.err
	}
	return w.FileWriter.Cancel(ctx)
}

type uploadCleanupErrorDriver struct {
	storagedriver.StorageDriver
	err error
}

func (d *uploadCleanupErrorDriver) Delete(ctx context.Context, path string) error {
	if d.err != nil {
		return d.err
	}
	return d.StorageDriver.Delete(ctx, path)
}

func TestBlobWriterCancelError(t *testing.T) {
	for _, stage := range []string{"driver cancellation", "resource cleanup"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			driver := &uploadCleanupErrorDriver{StorageDriver: inmemory.New()}
			reg, err := NewRegistry(ctx, driver)
			if err != nil {
				t.Fatal(err)
			}
			name, _ := reference.WithName("foo/bar")
			repo, err := reg.Repository(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			bs := repo.Blobs(ctx)
			upload, err := bs.Create(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer upload.Close()
			cancelErr := errors.New(stage + " failed")
			bw := upload.(*blobWriter)
			fw := &cancelErrorWriter{FileWriter: bw.fileWriter}
			bw.fileWriter = fw
			if stage == "driver cancellation" {
				fw.err = cancelErr
			} else {
				driver.err = cancelErr
			}
			if err := upload.Cancel(ctx); err != cancelErr {
				t.Errorf("Cancel error = %v, want %v", err, cancelErr)
			}
			if stage == "driver cancellation" {
				// A driver that rejected cancellation must remain usable.
				if _, err := upload.Write([]byte("x")); err != nil {
					t.Fatalf("Write after failed Cancel: %v", err)
				}
			}
			fw.err, driver.err = nil, nil
			if err := upload.Cancel(ctx); err != nil {
				t.Errorf("Cancel retry: %v", err)
			}
			if _, err := bs.Resume(ctx, upload.ID()); err != distribution.ErrBlobUploadUnknown {
				t.Errorf("Resume after Cancel retry returned %v", err)
			}
			driver.err = errors.New("backend unavailable after cleanup")
			if err := upload.Cancel(ctx); err != nil {
				t.Errorf("Cancel after completed cleanup returned %v", err)
			}
		})
	}
}

// TestSimpleBlobUpload covers the blob upload process, exercising common
// error paths that might be seen during an upload.
func TestSimpleBlobUpload(t *testing.T) {
	randomDataReader, dgst, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random reader: %v", err)
	}

	ctx := context.Background()
	imageName, _ := reference.WithName("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete, EnableRedirect)
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

	// get the enclosing directory
	uploadPath := path.Dir(blobUpload.(*blobWriter).path)

	// ensure state was cleaned up
	_, err = driver.List(ctx, uploadPath)
	if err == nil {
		t.Fatal("files in upload path after cleanup")
	}

	// Do a resume, get unknown upload
	_, err = bs.Resume(ctx, blobUpload.ID())
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
		t.Fatal("layer data write incomplete")
	}

	blobUpload.Close()

	offset := blobUpload.Size()
	if offset != nn {
		t.Fatalf("blobUpload not updated with correct offset: %v != %v", offset, nn)
	}

	// Do a resume, for good fun
	blobUpload, err = bs.Resume(ctx, blobUpload.ID())
	if err != nil {
		t.Fatalf("unexpected error resuming upload: %v", err)
	}

	sha256Digest := digest.NewDigest("sha256", h)
	desc, err := blobUpload.Commit(ctx, v1.Descriptor{Digest: dgst})
	if err != nil {
		t.Fatalf("unexpected error finishing layer upload: %v", err)
	}

	// ensure state was cleaned up
	uploadPath = path.Dir(blobUpload.(*blobWriter).path)
	_, err = driver.List(ctx, uploadPath)
	if err == nil {
		t.Fatal("files in upload path after commit")
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

	if !reflect.DeepEqual(statDesc, desc) {
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
		t.Fatal("incorrect read length")
	}

	if digest.NewDigest("sha256", h) != sha256Digest {
		t.Fatalf("unexpected digest from uploaded layer: %q != %q", digest.NewDigest("sha256", h), sha256Digest)
	}

	// Delete a blob
	err = bs.Delete(ctx, desc.Digest)
	if err != nil {
		t.Fatal("Unexpected error deleting blob")
	}

	d, err := bs.Stat(ctx, desc.Digest)
	if err == nil {
		t.Fatalf("unexpected non-error stating deleted blob: %v", d)
	}

	switch err {
	case distribution.ErrBlobUnknown:
		break
	default:
		t.Errorf("Unexpected error type stat-ing deleted manifest: %#v", err)
	}

	_, err = bs.Open(ctx, desc.Digest)
	if err == nil {
		t.Fatal("unexpected success opening deleted blob for read")
	}

	switch err {
	case distribution.ErrBlobUnknown:
		break
	default:
		t.Errorf("Unexpected error type getting deleted manifest: %#v", err)
	}

	// Re-upload the blob
	randomBlob, err := io.ReadAll(randomDataReader)
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
	registry, err = NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableRedirect)
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
	imageName, _ := reference.WithName("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete, EnableRedirect)
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

	_, err = bs.Open(ctx, dgst)
	if err != distribution.ErrBlobUnknown {
		t.Fatalf("expected not found error when opening non-existent blob: %v", err)
	}

	randomLayerSize, err := seekerSize(randomLayerReader)
	if err != nil {
		t.Fatalf("error getting seeker size for random layer: %v", err)
	}

	descBefore := v1.Descriptor{Digest: dgst, MediaType: "application/octet-stream", Size: randomLayerSize}
	t.Logf("desc: %v", descBefore)

	desc, err = addBlob(ctx, bs, descBefore, randomLayerReader)
	if err != nil {
		t.Fatalf("error adding blob to blobservice: %v", err)
	}

	if desc.Size != randomLayerSize {
		t.Fatalf("committed blob has incorrect length: %v != %v", desc.Size, randomLayerSize)
	}

	rc, err := bs.Open(ctx, desc.Digest) // note that we are opening with original digest.
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
	offset, err := rc.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("error seeking blob: %v", err)
	}

	if offset != 0 {
		t.Fatalf("seek failed: expected 0 offset, got %d", offset)
	}

	p, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("error reading all of blob: %v", err)
	}

	if len(p) != int(randomLayerSize) {
		t.Fatalf("blob data read has different length: %v != %v", len(p), randomLayerSize)
	}

	// Reset the randomLayerReader and read back the buffer
	_, err = randomLayerReader.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatalf("error resetting layer reader: %v", err)
	}

	randomLayerData, err := io.ReadAll(randomLayerReader)
	if err != nil {
		t.Fatalf("random layer read failed: %v", err)
	}

	if !bytes.Equal(p, randomLayerData) {
		t.Fatal("layer data not equal")
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
	imageName, _ := reference.WithName("foo/bar")
	sourceImageName, _ := reference.WithName("foo/source")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete, EnableRedirect)
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

	_, err = io.Copy(blobUpload, randomDataReader)
	if err != nil {
		t.Fatalf("unexpected error uploading layer data: %v", err)
	}

	desc, err := blobUpload.Commit(ctx, v1.Descriptor{Digest: dgst})
	if err != nil {
		t.Fatalf("unexpected error finishing layer upload: %v", err)
	}

	// Test for existence.
	statDesc, err := sbs.Stat(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v, %#v", err, sbs)
	}

	if !reflect.DeepEqual(statDesc, desc) {
		t.Fatalf("descriptors not equal: %v != %v", statDesc, desc)
	}

	bs := repository.Blobs(ctx)
	// Test destination for existence.
	_, err = bs.Stat(ctx, desc.Digest)
	if err == nil {
		t.Fatalf("unexpected non-error stating unmounted blob: %v", desc)
	}

	canonicalRef, err := reference.WithDigest(sourceRepository.Named(), desc.Digest)
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

	if !reflect.DeepEqual(ebm.Descriptor, desc) {
		t.Fatalf("descriptors not equal: %v != %v", ebm.Descriptor, desc)
	}

	// Test for existence.
	statDesc, err = bs.Stat(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error checking for existence: %v, %#v", err, bs)
	}

	if !reflect.DeepEqual(statDesc, desc) {
		t.Fatalf("descriptors not equal: %v != %v", statDesc, desc)
	}

	rc, err := bs.Open(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error opening blob for read: %v", err)
	}
	defer rc.Close()

	h := sha256.New()
	nn, err := io.Copy(h, rc)
	if err != nil {
		t.Fatalf("error reading layer: %v", err)
	}

	if nn != randomDataSize {
		t.Fatal("incorrect read length")
	}

	if digest.NewDigest("sha256", h) != dgst {
		t.Fatalf("unexpected digest from uploaded layer: %q != %q", digest.NewDigest("sha256", h), dgst)
	}

	// Delete the blob from the source repo
	err = sbs.Delete(ctx, desc.Digest)
	if err != nil {
		t.Fatal("Unexpected error deleting blob")
	}

	_, err = bs.Stat(ctx, desc.Digest)
	if err != nil {
		t.Fatalf("unexpected error stating blob deleted from source repository: %v", err)
	}

	d, err := sbs.Stat(ctx, desc.Digest)
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
		t.Fatal("Unexpected error deleting blob")
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
	imageName, _ := reference.WithName("foo/bar")
	driver := inmemory.New()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete, EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	bs := repository.Blobs(ctx)

	simpleUpload(t, bs, []byte{}, digestSha256Empty)
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

	desc, err := wr.Commit(ctx, v1.Descriptor{Digest: dgst})
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
	current, err := seeker.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	end, err := seeker.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}

	resumed, err := seeker.Seek(current, io.SeekStart)
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
func addBlob(ctx context.Context, bs distribution.BlobIngester, desc v1.Descriptor, rd io.Reader) (v1.Descriptor, error) {
	wr, err := bs.Create(ctx)
	if err != nil {
		return v1.Descriptor{}, err
	}
	// nolint:errcheck
	defer wr.Cancel(ctx)

	if nn, err := io.Copy(wr, rd); err != nil {
		return v1.Descriptor{}, err
	} else if nn != desc.Size {
		return v1.Descriptor{}, fmt.Errorf("incorrect number of bytes copied: %v != %v", nn, desc.Size)
	}

	return wr.Commit(ctx, desc)
}

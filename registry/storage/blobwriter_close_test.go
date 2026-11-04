package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
)

var errHashStatePut = errors.New("hash state put failed")

// hashStateFailingDriver fails hash state writes and records whether the
// upload file writers it handed out were closed.
type hashStateFailingDriver struct {
	storagedriver.StorageDriver
	closed int
}

func (d *hashStateFailingDriver) PutContent(ctx context.Context, path string, content []byte) error {
	if strings.Contains(path, "/hashstates/") {
		return errHashStatePut
	}
	return d.StorageDriver.PutContent(ctx, path, content)
}

func (d *hashStateFailingDriver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	fw, err := d.StorageDriver.Writer(ctx, path, append)
	if err != nil {
		return nil, err
	}
	return &closeRecordingFileWriter{FileWriter: fw, driver: d}, nil
}

type closeRecordingFileWriter struct {
	storagedriver.FileWriter
	driver *hashStateFailingDriver
}

func (w *closeRecordingFileWriter) Close() error {
	w.driver.closed++
	return w.FileWriter.Close()
}

func TestBlobWriterCloseClosesFileWriterWhenHashStateFails(t *testing.T) {
	ctx := context.Background()
	driver := &hashStateFailingDriver{StorageDriver: inmemory.New()}
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)))
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	imageName, _ := reference.WithName("foo/bar")
	repository, err := registry.Repository(ctx, imageName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	upload, err := repository.Blobs(ctx).Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting upload: %v", err)
	}
	if _, err := upload.Write([]byte{1, 2, 3}); err != nil {
		t.Fatalf("unexpected error writing upload: %v", err)
	}

	if err := upload.Close(); !errors.Is(err, errHashStatePut) {
		t.Errorf("Close() error = %v, want %v", err, errHashStatePut)
	}
	if driver.closed != 1 {
		t.Errorf("file writer closed %d times, want 1", driver.closed)
	}
}

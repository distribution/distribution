package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type fixedBlobStatter struct {
	desc v1.Descriptor
}

func (s fixedBlobStatter) Stat(context.Context, digest.Digest) (v1.Descriptor, error) {
	return s.desc, nil
}

// On the filesystem driver, ServeBlob answers a GET with the blob file as it
// is on disk, also when its size differs from the size in the blob descriptor.
func TestBlobServerFilesystemFileSize(t *testing.T) {
	desc := v1.Descriptor{Digest: digest.FromString("0123456789"), Size: 10, MediaType: "application/octet-stream"}
	for _, tc := range []struct{ name, fileContent string }{
		{name: "file-smaller-than-descriptor", fileContent: "01234"},
		{name: "file-larger-than-descriptor", fileContent: "0123456789ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := filesystem.New(filesystem.DriverParameters{RootDirectory: t.TempDir(), MaxThreads: 25})
			if err := fs.PutContent(context.Background(), "/blob", []byte(tc.fileContent)); err != nil {
				t.Fatal(err)
			}
			bs := &blobServer{
				driver:  fs,
				statter: fixedBlobStatter{desc},
				pathFn:  func(digest.Digest) (string, error) { return "/blob", nil },
			}
			w := httptest.NewRecorder()
			if err := bs.ServeBlob(context.Background(), w, httptest.NewRequest(http.MethodGet, "/", nil), desc.Digest); err != nil {
				t.Fatal(err)
			}
			if got := w.Body.String(); got != tc.fileContent {
				t.Errorf("body = %q, want %q", got, tc.fileContent)
			}
			if got, want := w.Header().Get("Content-Length"), strconv.Itoa(len(tc.fileContent)); got != want {
				t.Errorf("Content-Length = %q, want %q", got, want)
			}
		})
	}
}

type readerCallCounter struct {
	driver.StorageDriver
	calls int
}

func (d *readerCallCounter) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	d.calls++
	return d.StorageDriver.Reader(ctx, path, offset)
}

// For a driver other than the filesystem driver, ServeBlob does not call
// Reader itself: the only call per GET is fileReader's.
func TestBlobServerOtherDriverReaderCalls(t *testing.T) {
	const content = "0123456789"
	counter := &readerCallCounter{StorageDriver: inmemory.New()}
	if err := counter.PutContent(context.Background(), "/blob", []byte(content)); err != nil {
		t.Fatal(err)
	}
	desc := v1.Descriptor{Digest: digest.FromString(content), Size: int64(len(content)), MediaType: "application/octet-stream"}
	bs := &blobServer{
		driver:  counter,
		statter: fixedBlobStatter{desc},
		pathFn:  func(digest.Digest) (string, error) { return "/blob", nil },
	}
	w := httptest.NewRecorder()
	if err := bs.ServeBlob(context.Background(), w, httptest.NewRequest(http.MethodGet, "/", nil), desc.Digest); err != nil {
		t.Fatal(err)
	}
	if got := w.Body.String(); got != content {
		t.Errorf("body = %q, want %q", got, content)
	}
	if counter.calls != 1 {
		t.Errorf("Reader calls = %d, want 1", counter.calls)
	}
}

package storage

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// fileObservingListener wraps every accepted connection in a
// fileObservingConn.
type fileObservingListener struct {
	net.Listener
	sawFile *atomic.Bool
}

func (l fileObservingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return fileObservingConn{c.(*net.TCPConn), l.sawFile}, nil
}

// fileObservingConn records whether net/http passes an *os.File to
// net.TCPConn.ReadFrom, where Go calls sendfile(2).
type fileObservingConn struct {
	*net.TCPConn
	sawFile *atomic.Bool
}

func (c fileObservingConn) ReadFrom(r io.Reader) (int64, error) {
	if limited, ok := r.(*io.LimitedReader); ok {
		if _, ok := limited.R.(*os.File); ok {
			c.sawFile.Store(true)
		}
	}
	return c.TCPConn.ReadFrom(r)
}

// With the registry's response writer in the handler chain, the blob file
// reaches net.TCPConn.ReadFrom. The blob is larger than the 512 bytes that
// net/http copies through Write before it calls ReadFrom.
func TestBlobServerFilesystemFileReachesTCPConnReadFrom(t *testing.T) {
	content := strings.Repeat("0123456789", 1000)
	fs := filesystem.New(filesystem.DriverParameters{RootDirectory: t.TempDir(), MaxThreads: 25})
	if err := fs.PutContent(context.Background(), "/blob", []byte(content)); err != nil {
		t.Fatal(err)
	}
	desc := v1.Descriptor{Digest: digest.FromString(content), Size: int64(len(content)), MediaType: "application/octet-stream"}
	bs := &blobServer{
		driver:  fs,
		statter: fixedBlobStatter{desc},
		pathFn:  func(digest.Digest) (string, error) { return "/blob", nil },
	}
	var sawFile atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, w := dcontext.WithResponseWriter(r.Context(), w)
		if err := bs.ServeBlob(ctx, w, r, desc.Digest); err != nil {
			t.Error(err)
		}
	}))
	server.Listener = fileObservingListener{server.Listener, &sawFile}
	server.Start()
	defer server.Close()

	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != content {
		t.Fatalf("status = %d, body length = %d, error = %v", response.StatusCode, len(body), err)
	}
	if !sawFile.Load() {
		t.Error("blob file did not reach net.TCPConn.ReadFrom")
	}
}

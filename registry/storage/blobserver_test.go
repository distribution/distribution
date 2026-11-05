package storage

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type fixedBlobStatter struct {
	desc v1.Descriptor
}

func (s fixedBlobStatter) Stat(context.Context, digest.Digest) (v1.Descriptor, error) {
	return s.desc, nil
}

type blobReaderSpy struct {
	driver.StorageDriver
	reads int
	files []*os.File
	wrap  func(io.ReadCloser) io.ReadCloser
}

func (d *blobReaderSpy) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	d.reads++
	rc, err := d.StorageDriver.Reader(ctx, path, offset)
	if err != nil {
		return rc, err
	}
	if f, ok := rc.(*os.File); ok {
		d.files = append(d.files, f)
	}
	if d.wrap != nil {
		rc = d.wrap(rc)
	}
	return rc, nil
}

// Check sendfile eligibility without depending on the operating system or socket type.
type fileBodyProbe struct {
	*httptest.ResponseRecorder
	sawFile bool
}

func (w *fileBodyProbe) ReadFrom(r io.Reader) (int64, error) {
	if limited, ok := r.(*io.LimitedReader); ok {
		_, w.sawFile = limited.R.(*os.File)
	}
	return io.Copy(w.ResponseRecorder, r)
}

func newFilesystemBlobServer(t *testing.T, body string) (*blobServer, *blobReaderSpy, digest.Digest) {
	t.Helper()
	const path = "/blob"
	content := []byte(body)
	dgst := digest.FromBytes(content)
	fs := filesystem.New(filesystem.DriverParameters{RootDirectory: t.TempDir(), MaxThreads: 25})
	if err := fs.PutContent(context.Background(), path, content); err != nil {
		t.Fatal(err)
	}
	spy := &blobReaderSpy{StorageDriver: fs}
	return &blobServer{
		driver:  spy,
		statter: fixedBlobStatter{desc: v1.Descriptor{Digest: dgst, Size: int64(len(content)), MediaType: "application/octet-stream"}},
		pathFn:  func(digest.Digest) (string, error) { return path, nil },
	}, spy, dgst
}

func assertBlobReadersClosed(t *testing.T, spy *blobReaderSpy) {
	t.Helper()
	for _, f := range spy.files {
		if _, err := f.Stat(); !errors.Is(err, os.ErrClosed) {
			t.Errorf("blob file remains open: Stat error = %v", err)
		}
	}
}

// Cover direct-file selection and lazy opening; ServeContent owns the detailed
// conditional and range-header behavior.
func TestBlobServerFilesystemContent(t *testing.T) {
	for _, tc := range []struct {
		name, method, rangeHeader, wantBody string
		precondition, preconditionValue     string
		wantStatus, wantReads               int
		wantFile                            bool
	}{
		{name: "full", method: http.MethodGet, wantBody: "0123456789", wantStatus: http.StatusOK, wantReads: 1, wantFile: true},
		{name: "range", method: http.MethodGet, rangeHeader: "bytes=2-5", wantBody: "2345", wantStatus: http.StatusPartialContent, wantReads: 1, wantFile: true},
		{name: "head", method: http.MethodHead, wantStatus: http.StatusOK},
		{name: "not-modified", method: http.MethodGet, precondition: "If-None-Match", preconditionValue: "match", wantStatus: http.StatusNotModified},
		{name: "failed-precondition", method: http.MethodGet, precondition: "If-Match", preconditionValue: `"other"`, wantStatus: http.StatusPreconditionFailed},
		{name: "if-match-pass", method: http.MethodGet, precondition: "If-Match", preconditionValue: "match", wantBody: "0123456789", wantStatus: http.StatusOK, wantReads: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bs, spy, dgst := newFilesystemBlobServer(t, "0123456789")
			req := httptest.NewRequest(tc.method, "/blob", nil)
			req.Header.Set("Range", tc.rangeHeader)
			if tc.precondition != "" {
				value := tc.preconditionValue
				if value == "match" {
					value = `"` + dgst.String() + `"`
				}
				req.Header.Set(tc.precondition, value)
			}
			probe := &fileBodyProbe{ResponseRecorder: httptest.NewRecorder()}
			ctx, w := dcontext.WithResponseWriter(context.Background(), probe)
			if err := bs.ServeBlob(ctx, w, req, dgst); err != nil {
				t.Fatal(err)
			}
			assertBlobReadersClosed(t, spy)
			if ctx.Value("http.response.written") != int64(len(tc.wantBody)) || ctx.Value("http.response.status") != tc.wantStatus {
				t.Errorf("recorded bytes = %v, status = %v", ctx.Value("http.response.written"), ctx.Value("http.response.status"))
			}
			if probe.Code != tc.wantStatus || probe.Body.String() != tc.wantBody {
				t.Fatalf("response: status %d, body %q", probe.Code, probe.Body.String())
			}
			if spy.reads != tc.wantReads || probe.sawFile != tc.wantFile {
				t.Errorf("Reader calls = %d, file source = %t", spy.reads, probe.sawFile)
			}
			if tc.wantStatus == http.StatusPartialContent && (probe.Header().Get("Content-Range") != "bytes 2-5/10" || probe.Header().Get("Content-Length") != "4") {
				t.Errorf("range headers: %v", probe.Header())
			}
			if tc.method == http.MethodHead && probe.Header().Get("Content-Length") != "10" {
				t.Errorf("HEAD headers: %v", probe.Header())
			}
		})
	}
}

// A blob file missing after Stat must fail before HTTP 200 is committed;
// the lazy fileReader would serve an empty body with the descriptor length.
func TestBlobServerFilesystemMissingFile(t *testing.T) {
	bs, spy, dgst := newFilesystemBlobServer(t, "0123456789")
	if err := spy.Delete(context.Background(), "/blob"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	err := bs.ServeBlob(context.Background(), w, httptest.NewRequest(http.MethodGet, "/blob", nil), dgst)
	if !errors.As(err, new(driver.PathNotFoundError)) {
		t.Fatalf("missing file error = %v", err)
	}
	if len(w.Header()) != 0 || w.Body.Len() != 0 {
		t.Fatal("response written for a missing blob file")
	}
}

func TestBlobServerFilesystemWrappedReader(t *testing.T) {
	bs, spy, dgst := newFilesystemBlobServer(t, "0123456789")
	spy.wrap = func(rc io.ReadCloser) io.ReadCloser { return struct{ io.ReadCloser }{rc} }
	probe := &fileBodyProbe{ResponseRecorder: httptest.NewRecorder()}
	ctx, w := dcontext.WithResponseWriter(context.Background(), probe)
	if err := bs.ServeBlob(ctx, w, httptest.NewRequest(http.MethodGet, "/blob", nil), dgst); err != nil {
		t.Fatal(err)
	}
	assertBlobReadersClosed(t, spy)
	if spy.reads != 2 || probe.sawFile || probe.Code != http.StatusOK || probe.Body.String() != "0123456789" || ctx.Value("http.response.written") != int64(10) {
		t.Fatalf("Reader calls = %d, file source = %t, response = %d %q", spy.reads, probe.sawFile, probe.Code, probe.Body.String())
	}
}

type nonFilesystemDriver struct{ *blobReaderSpy }

func (nonFilesystemDriver) Name() string { return "non-filesystem" }

func TestBlobServerNonFilesystemReader(t *testing.T) {
	bs, spy, dgst := newFilesystemBlobServer(t, "0123456789")
	spy.wrap = func(rc io.ReadCloser) io.ReadCloser { return struct{ io.ReadCloser }{rc} }
	bs.driver = nonFilesystemDriver{spy}
	probe := &fileBodyProbe{ResponseRecorder: httptest.NewRecorder()}
	if err := bs.ServeBlob(context.Background(), probe, httptest.NewRequest(http.MethodGet, "/blob", nil), dgst); err != nil {
		t.Fatal(err)
	}
	assertBlobReadersClosed(t, spy)
	// Non-filesystem drivers must avoid a speculative Reader call before the lazy read.
	if spy.reads != 1 || probe.sawFile || probe.Code != http.StatusOK || probe.Body.String() != "0123456789" {
		t.Fatalf("Reader calls = %d, file source = %t, response = %d %q", spy.reads, probe.sawFile, probe.Code, probe.Body.String())
	}
}

// fileHandoffListener records whether net/http passes the blob file to
// TCPConn.ReadFrom, where Go selects sendfile.
type fileHandoffListener struct {
	net.Listener
	sawFile *atomic.Bool
}

func (l fileHandoffListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if tcp, ok := c.(*net.TCPConn); ok {
		return fileHandoffConn{tcp, l.sawFile}, nil
	}
	return c, err
}

type fileHandoffConn struct {
	*net.TCPConn
	sawFile *atomic.Bool
}

func (c fileHandoffConn) ReadFrom(r io.Reader) (int64, error) {
	src := r
	if limited, ok := r.(*io.LimitedReader); ok {
		src = limited.R
	}
	if _, ok := src.(*os.File); ok {
		c.sawFile.Store(true)
	}
	// Delegate the original reader so the real copy path still runs.
	return c.TCPConn.ReadFrom(r)
}

// Copy a range through a real socket and check that the blob file reaches
// TCPConn.ReadFrom. The range starts mid-file and exceeds net/http's initial
// buffered copy.
func TestBlobServerFilesystemHTTP(t *testing.T) {
	payload := make([]byte, 64*1024)
	// Use varied data so incorrect range offsets change the response body.
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range payload {
		payload[i] = byte(rng.Uint32())
	}
	body := string(payload)
	bs, spy, dgst := newFilesystemBlobServer(t, body)
	written := make(chan any, 1)
	var sawFile atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, w := dcontext.WithResponseWriter(r.Context(), w)
		if err := bs.ServeBlob(ctx, w, r, dgst); err != nil {
			t.Error(err)
		}
		written <- ctx.Value("http.response.written")
	}))
	server.Listener = fileHandoffListener{server.Listener, &sawFile}
	server.Start()
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=1025-32768")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	gotBody, err := io.ReadAll(response.Body)
	response.Body.Close()
	wantBody := body[1025:32769]
	if err != nil || response.StatusCode != http.StatusPartialContent || string(gotBody) != wantBody {
		t.Fatalf("status = %d, body length = %d, error = %v", response.StatusCode, len(gotBody), err)
	}
	if got := <-written; got != int64(len(wantBody)) {
		t.Errorf("recorded bytes = %v, want %d", got, len(wantBody))
	}
	if !sawFile.Load() {
		t.Error("blob file did not reach TCPConn.ReadFrom")
	}
	assertBlobReadersClosed(t, spy)
}

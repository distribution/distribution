package dcontext

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func TestWithRequest(t *testing.T) {
	var req http.Request

	start := time.Now()
	req.Method = http.MethodGet
	req.Host = "example.com"
	req.RequestURI = "/test-test"
	req.Header = make(http.Header)
	req.Header.Set("Referer", "foo.com/referer")
	req.Header.Set("User-Agent", "test/0.1")

	ctx := WithRequest(Background(), &req)
	for _, tc := range []struct {
		key      string
		expected any
	}{
		{
			key:      "http.request",
			expected: &req,
		},
		{
			key: "http.request.id",
		},
		{
			key:      "http.request.method",
			expected: req.Method,
		},
		{
			key:      "http.request.host",
			expected: req.Host,
		},
		{
			key:      "http.request.uri",
			expected: req.RequestURI,
		},
		{
			key:      "http.request.referer",
			expected: req.Referer(),
		},
		{
			key:      "http.request.useragent",
			expected: req.UserAgent(),
		},
		{
			key:      "http.request.remoteaddr",
			expected: req.RemoteAddr,
		},
		{
			key: "http.request.startedat",
		},
	} {
		v := ctx.Value(tc.key)

		if v == nil {
			t.Fatalf("value not found for %q", tc.key)
		}

		if tc.expected != nil && v != tc.expected {
			t.Fatalf("%s: %v != %v", tc.key, v, tc.expected)
		}

		// Key specific checks!
		switch tc.key {
		case "http.request.id":
			if _, ok := v.(string); !ok {
				t.Fatalf("request id not a string: %v", v)
			}
		case "http.request.startedat":
			vt, ok := v.(time.Time)
			if !ok {
				t.Fatalf("value not a time: %v", v)
			}

			now := time.Now()
			if vt.After(now) {
				t.Fatalf("time generated too late: %v > %v", vt, now)
			}

			if vt.Before(start) {
				t.Fatalf("time generated too early: %v < %v", vt, start)
			}
		}
	}
}

type testResponseWriter struct {
	flushed bool
	status  int
	written int64
	header  http.Header
}

func (trw *testResponseWriter) Header() http.Header {
	if trw.header == nil {
		trw.header = make(http.Header)
	}

	return trw.header
}

func (trw *testResponseWriter) Write(p []byte) (n int, err error) {
	if trw.status == 0 {
		trw.status = http.StatusOK
	}

	n = len(p)
	trw.written += int64(n)
	return
}

func (trw *testResponseWriter) WriteHeader(status int) {
	trw.status = status
}

func (trw *testResponseWriter) Flush() {
	trw.flushed = true
}

type testReaderFromResponseWriter struct {
	testResponseWriter
	source io.Reader
}

func (trw *testReaderFromResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	trw.source = r
	return io.Copy(&trw.testResponseWriter, r)
}

func TestWithResponseWriter(t *testing.T) {
	trw := testResponseWriter{}
	ctx, rw := WithResponseWriter(Background(), &trw)

	if ctx.Value("http.response") != rw {
		t.Fatalf("response not available in context: %v != %v", ctx.Value("http.response"), rw)
	}

	grw, err := GetResponseWriter(ctx)
	if err != nil {
		t.Fatalf("error getting response writer: %v", err)
	}

	if grw != rw {
		t.Fatalf("unexpected response writer returned: %#v != %#v", grw, rw)
	}

	if ctx.Value("http.response.status") != 0 {
		t.Fatalf("response status should always be a number and should be zero here: %v != 0", ctx.Value("http.response.status"))
	}

	if n, err := rw.Write(make([]byte, 1024)); err != nil {
		t.Fatalf("unexpected error writing: %v", err)
	} else if n != 1024 {
		t.Fatalf("unexpected number of bytes written: %v != %v", n, 1024)
	}

	if ctx.Value("http.response.status") != http.StatusOK {
		t.Fatalf("unexpected response status in context: %v != %v", ctx.Value("http.response.status"), http.StatusOK)
	}

	if ctx.Value("http.response.written") != int64(1024) {
		t.Fatalf("unexpected number reported bytes written: %v != %v", ctx.Value("http.response.written"), 1024)
	}

	// Make sure flush propagates
	rw.(http.Flusher).Flush()

	if !trw.flushed {
		t.Fatal("response writer not flushed")
	}

	// Write another status and make sure context is correct. This normally
	// wouldn't work except for in this contrived testcase.
	rw.WriteHeader(http.StatusBadRequest)

	if ctx.Value("http.response.status") != http.StatusBadRequest {
		t.Fatalf("unexpected response status in context: %v != %v", ctx.Value("http.response.status"), http.StatusBadRequest)
	}
}

func TestWithResponseWriterReadFrom(t *testing.T) {
	copyErr := errors.New("copy failed")
	for _, tc := range []struct {
		name       string
		delegate   bool
		body       string
		status     int
		readErr    error
		wantStatus int
	}{
		// ServeContent commits 206 before copying a range.
		{name: "delegate-explicit-status", delegate: true, body: "content", status: http.StatusPartialContent, wantStatus: http.StatusPartialContent},
		{name: "delegate-partial-error", delegate: true, body: "part", readErr: copyErr, wantStatus: http.StatusOK},
		{name: "delegate-error-without-bytes", delegate: true, readErr: copyErr},
		{name: "fallback", body: "content", wantStatus: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trw := &testResponseWriter{}
			rf := &testReaderFromResponseWriter{}
			var destination http.ResponseWriter = trw
			if tc.delegate {
				destination = rf
				trw = &rf.testResponseWriter
			}
			ctx, rw := WithResponseWriter(Background(), destination)
			if tc.status != 0 {
				rw.WriteHeader(tc.status)
			}
			var source io.Reader = strings.NewReader(tc.body)
			copySize := int64(len(tc.body))
			if tc.readErr != nil {
				source = io.MultiReader(source, iotest.ErrReader(tc.readErr))
				copySize++
			}
			// Use io.CopyN to exercise ReadFrom even when the source implements WriterTo.
			n, err := io.CopyN(rw, source, copySize)
			if n != int64(len(tc.body)) || !errors.Is(err, tc.readErr) {
				t.Fatalf("CopyN = (%d, %v), want (%d, %v)", n, err, len(tc.body), tc.readErr)
			}
			if tc.delegate {
				limited, ok := rf.source.(*io.LimitedReader)
				if !ok || limited.R != source {
					t.Fatalf("ReadFrom received %T; want the original limited source", rf.source)
				}
			}
			if got := ctx.Value("http.response.status"); got != tc.wantStatus || trw.status != tc.wantStatus {
				t.Errorf("recorded status = %v, writer status = %d, want %d", got, trw.status, tc.wantStatus)
			}
			wantWritten := int64(len(tc.body))
			if got := ctx.Value("http.response.written"); got != wantWritten || trw.written != wantWritten {
				t.Errorf("recorded bytes = %v, writer bytes = %d, want %d", got, trw.written, wantWritten)
			}
		})
	}
}

func TestWithVars(t *testing.T) {
	var req http.Request
	vars := map[string]string{
		"foo": "asdf",
		"bar": "qwer",
	}

	getVarsFromRequest = func(r *http.Request) map[string]string {
		if r != &req {
			t.Fatalf("unexpected request: %v != %v", r, req)
		}

		return vars
	}

	ctx := WithVars(Background(), &req)
	for _, tc := range []struct {
		key      string
		expected any
	}{
		{
			key:      "vars",
			expected: vars,
		},
		{
			key:      "vars.foo",
			expected: "asdf",
		},
		{
			key:      "vars.bar",
			expected: "qwer",
		},
	} {
		v := ctx.Value(tc.key)

		if !reflect.DeepEqual(v, tc.expected) {
			t.Fatalf("%q: %v != %v", tc.key, v, tc.expected)
		}
	}
}

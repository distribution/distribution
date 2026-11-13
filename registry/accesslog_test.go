package registry

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"

	gorhandlers "github.com/gorilla/handlers"
)

var logTimestamp = regexp.MustCompile(`\[[^]]*\]`)

// TestAccessLogMatchesGorilla compares accessLogHandler with
// CombinedLoggingHandler on the Write path, where Gorilla counts correctly.
func TestAccessLogMatchesGorilla(t *testing.T) {
	for _, tc := range []struct {
		name, target, referer, userAgent, remoteAddr string
		status                                       int
	}{
		{"plain", "/v2/", "", "docker/28.0", "192.0.2.1:1234", http.StatusOK},
		{"status", "/v2/x/blobs/sha256:00", "", "", "192.0.2.1:1234", http.StatusNotFound},
		{"userinfo", "http://alice@example.com/v2/", "", "", "192.0.2.1:1234", http.StatusOK},
		{"ipv6", "/v2/", "", "", "[2001:db8::1]:443", http.StatusOK},
		{"no port", "/v2/", "", "", "192.0.2.1", http.StatusOK},
		{"escaping", "/v2/%22", "a\"b\\c\td\x7fe\x01\xff", "ü   \U0001F600 \x7f\\x7f", "192.0.2.1:1234", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("hello"))
			})
			logLine := func(wrap func(io.Writer, http.Handler) http.Handler) string {
				req := httptest.NewRequest(http.MethodGet, tc.target, nil)
				req.RemoteAddr = tc.remoteAddr
				req.Header.Set("Referer", tc.referer)
				req.Header.Set("User-Agent", tc.userAgent)
				var out bytes.Buffer
				wrap(&out, h).ServeHTTP(httptest.NewRecorder(), req)
				// The handlers take their own timestamps.
				return logTimestamp.ReplaceAllString(out.String(), "[ts]")
			}
			got, want := logLine(accessLogHandler), logLine(gorhandlers.CombinedLoggingHandler)
			if want == "" || got != want {
				t.Errorf("log line mismatch\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

// readerFromRecorder records whether ReadFrom reached the underlying writer.
type readerFromRecorder struct {
	*httptest.ResponseRecorder
	readFromCalls int
}

func (r *readerFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	r.readFromCalls++
	return io.Copy(r.ResponseRecorder, src)
}

// TestAccessLogCountsReadFrom checks that bytes copied through io.ReaderFrom
// are logged and that ReadFrom is still delegated to the wrapped writer.
func TestAccessLogCountsReadFrom(t *testing.T) {
	const body = "0123456789"
	for _, tc := range []struct {
		name, prefix, body          string
		readErr                     error
		afterCopyStatus, wantStatus int
	}{
		{name: "read-from", body: body, wantStatus: http.StatusOK},
		{name: "mixed", prefix: "pre-", body: body, wantStatus: http.StatusOK},
		{name: "partial-error", body: body, readErr: io.ErrUnexpectedEOF, afterCopyStatus: http.StatusInternalServerError, wantStatus: http.StatusOK},
		{name: "error-before-bytes", readErr: io.ErrUnexpectedEOF, afterCopyStatus: http.StatusInternalServerError, wantStatus: http.StatusInternalServerError},
		{name: "empty", afterCopyStatus: http.StatusNoContent, wantStatus: http.StatusNoContent},
		{name: "empty-after-write", prefix: "pre-", afterCopyStatus: http.StatusInternalServerError, wantStatus: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.prefix != "" {
					_, _ = w.Write([]byte(tc.prefix))
				}
				var src io.Reader = strings.NewReader(tc.body)
				if tc.readErr != nil {
					src = io.MultiReader(src, iotest.ErrReader(tc.readErr))
				}
				// Hide WriteTo so io.Copy uses the writer's ReadFrom, as for *os.File.
				n, err := io.Copy(w, struct{ io.Reader }{src})
				if n != int64(len(tc.body)) || !errors.Is(err, tc.readErr) {
					t.Errorf("copy = %d, %v; want %d, %v", n, err, len(tc.body), tc.readErr)
				}
				if tc.afterCopyStatus != 0 {
					w.WriteHeader(tc.afterCopyStatus)
				}
			})
			var out bytes.Buffer
			rec := &readerFromRecorder{ResponseRecorder: httptest.NewRecorder()}
			accessLogHandler(&out, h).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v2/", nil))

			if rec.readFromCalls != 1 {
				t.Errorf("ReadFrom calls = %d, want 1", rec.readFromCalls)
			}
			wantBody := tc.prefix + tc.body
			if rec.Code != tc.wantStatus || rec.Body.String() != wantBody {
				t.Errorf("response = %d %q, want %d %q", rec.Code, rec.Body.String(), tc.wantStatus, wantBody)
			}
			want := fmt.Sprintf(`" %d %d "`, tc.wantStatus, len(wantBody))
			if !strings.Contains(out.String(), want) {
				t.Errorf("log line does not contain %q: %q", want, out.String())
			}
		})
	}
}

// TestAccessLogStatusMatchesClient checks, against a net/http server, that the
// access log shows the status that the client received. The handler sets each
// status from 100 to 599. The test has no expected status per request: it
// compares the log with what the client received.
func TestAccessLogStatusMatchesClient(t *testing.T) {
	// The pipe buffers each log line, so the handler's write does not wait for
	// the test, which reads the line after the response.
	logReader, logWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer logReader.Close()
	defer logWriter.Close()
	srv := httptest.NewServer(accessLogHandler(logWriter, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, _ := strconv.Atoi(r.URL.Path[1:])
		w.WriteHeader(status)
	})))
	defer srv.Close()

	logs := bufio.NewReader(logReader)
	for status := 100; status < 600; status++ {
		path := fmt.Sprintf("/%d", status)
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		line, err := logs.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		var logged int
		_, afterRequest, _ := strings.Cut(line, `HTTP/1.1" `)
		_, _ = fmt.Sscan(afterRequest, &logged)
		if logged != resp.StatusCode {
			t.Errorf("GET %s: access log shows status %d, client received %d", path, logged, resp.StatusCode)
		}
	}
}

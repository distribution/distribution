package client

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

func TestServeBlobRange(t *testing.T) {
	data := []byte("0123456789abcdef")
	dgst := digest.FromBytes(data)
	for _, tc := range []struct {
		name, value, ifRange string
		status               int
	}{
		{"bounded", "bytes=2-5", "", 206},
		{"open", "bytes=12-", "", 206},
		{"suffix", "bytes=-4", "", 206},
		{"multipart", "bytes=0-1,8-9", "", 206},
		{"if-range fallback", "bytes=2-5", `"old"`, 200},
		{"ignored", "bytes=2-5", "", 200},
		{"unsatisfiable", "bytes=99-", "", 416},
		{"missing", "bytes=2-5", "", 404},
		{"failure", "bytes=2-5", "", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			responses := make(chan *httptest.ResponseRecorder, 1)
			// Use a distinct hostname on the redirect destination, as object stores do.
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Range") != tc.value || r.Header.Get("If-Range") != tc.ifRange || r.Header.Get("Accept-Encoding") != "identity" {
					t.Errorf("origin request: %s %v", r.Method, r.Header)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("forwarded downstream credentials")
				}
				expected := httptest.NewRecorder()
				expected.Header().Set("Etag", `"current"`)
				if tc.status >= 400 && tc.status != 416 {
					expected.WriteHeader(tc.status)
				} else {
					if tc.name == "ignored" {
						r.Header.Del("Range")
					}
					http.ServeContent(expected, r, "", time.Time{}, bytes.NewReader(data))
				}
				responses <- expected
				for k, values := range expected.Header() {
					w.Header()[k] = values
				}
				w.Header().Set("Connection", "X-Origin-Only, Etag")
				w.Header().Set("X-Origin-Only", "private")
				w.WriteHeader(expected.Code)
				// Flush exercises the relay with a streamed response as well.
				w.(http.Flusher).Flush()
				_, _ = w.Write(expected.Body.Bytes())
			}))
			defer target.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Docker-Content-Digest", dgst.String())
				http.Redirect(w, r, strings.Replace(target.URL, "127.0.0.1", "localhost", 1), http.StatusTemporaryRedirect)
			}))
			defer origin.Close()
			name, _ := reference.WithName("test/blob")
			repo, err := NewRepository(name, origin.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Range", tc.value)
			req.Header.Set("If-Range", tc.ifRange)
			req.Header.Set("Authorization", "Bearer downstream-only")
			req.Header.Set("Cookie", "session=downstream-only")
			w := httptest.NewRecorder()
			err = repo.Blobs(context.Background()).ServeBlob(req.Context(), w, req, dgst)
			if tc.status == 404 {
				if err != distribution.ErrBlobUnknown {
					t.Fatalf("missing blob: %v", err)
				}
				return
			}
			if tc.status == 500 {
				if _, ok := err.(*UnexpectedHTTPStatusError); !ok {
					t.Fatalf("origin failure: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			expected := <-responses
			if w.Code != tc.status || !bytes.Equal(w.Body.Bytes(), expected.Body.Bytes()) {
				t.Fatalf("relay: status=%d body=%q, want %d %q", w.Code, w.Body.Bytes(), tc.status, expected.Body.Bytes())
			}
			if tc.status == http.StatusPartialContent || tc.name == "if-range fallback" {
				resp := w.Result()
				defer resp.Body.Close()
				if got := resp.Header.Get("Docker-Content-Digest"); got != dgst.String() {
					t.Errorf("Docker-Content-Digest: got %q want %q", got, dgst.String())
				}
			}
			for _, key := range []string{"Content-Range", "Content-Length", "Content-Type"} {
				if w.Header().Get(key) != expected.Header().Get(key) {
					t.Errorf("%s: got %q want %q", key, w.Header().Get(key), expected.Header().Get(key))
				}
			}
			for _, key := range []string{"Connection", "X-Origin-Only", "Etag"} {
				if w.Header().Get(key) != "" {
					t.Errorf("forwarded hop-by-hop header %s", key)
				}
			}
		})
	}
}

func TestServeBlobRangeCanceled(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer origin.Close()
	name, _ := reference.WithName("test/blob")
	repo, err := NewRepository(name, origin.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	req.Header.Set("Range", "bytes=0-1")
	done := make(chan error, 1)
	go func() { done <- repo.Blobs(ctx).ServeBlob(ctx, httptest.NewRecorder(), req, digest.FromString("blob")) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("origin request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled request: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client did not stop")
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("origin was not canceled")
	}
}

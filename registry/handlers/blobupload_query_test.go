package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/auth"
)

// from and mount are query parameters. FormValue parses a multipart body to
// read them, and ServeHTTP copies the request first, so the temp files never
// land on the request net/http cleans up.
func TestBlobUploadQueryDoesNotParseMultipartBody(t *testing.T) {
	config := &configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": nil,
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
	}
	config.HTTP.Secret = "test-secret"

	t.Run("auth", func(t *testing.T) {
		app := NewApp(context.Background(), config)
		t.Cleanup(app.cancel)
		rec := &recordAccess{}
		app.accessController = rec

		body := &countReader{r: strings.NewReader("not a multipart body")}
		req := httptest.NewRequest(http.MethodPost, "http://registry/v2/probe/blobs/uploads/?from=other/repo", body)
		req.Header.Set("Content-Type", `multipart/form-data; boundary=abc`)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if body.n != 0 {
			t.Fatalf("authorization read %d bytes of the multipart body", body.n)
		}
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d, want 401", rr.Code)
		}
		if !sawAccess(rec.accesses, "other/repo", "pull") {
			t.Fatalf("from query was not checked, accesses: %+v", rec.accesses)
		}
	})

	t.Run("no auth", func(t *testing.T) {
		app := NewApp(context.Background(), config)
		t.Cleanup(app.cancel)

		body := &countReader{r: strings.NewReader("not a multipart body")}
		req := httptest.NewRequest(http.MethodPost, "http://registry/v2/probe/blobs/uploads/?from=other/repo&mount=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", body)
		req.Header.Set("Content-Type", `multipart/form-data; boundary=abc`)
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		if body.n != 0 {
			t.Fatalf("upload start read %d bytes of the multipart body", body.n)
		}
		if rr.Code != http.StatusAccepted {
			t.Fatalf("status %d, want 202: %s", rr.Code, rr.Body.String())
		}
	})
}

type countReader struct {
	r io.Reader
	n int
}

func (c *countReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

type recordAccess struct {
	accesses []auth.Access
}

func (r *recordAccess) Authorized(_ *http.Request, access ...auth.Access) (*auth.Grant, error) {
	r.accesses = append(r.accesses, access...)
	return nil, denyChallenge{}
}

type denyChallenge struct{}

func (denyChallenge) Error() string { return "denied" }

func (denyChallenge) SetHeaders(*http.Request, http.ResponseWriter) {}

func sawAccess(accesses []auth.Access, repo, action string) bool {
	for _, a := range accesses {
		if a.Name == repo && a.Action == action {
			return true
		}
	}
	return false
}

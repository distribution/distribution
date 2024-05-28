package client

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func testServer(rrm testutil.RequestResponseMap) (string, func()) {
	h := testutil.NewHandler(rrm)
	s := httptest.NewServer(h)
	return s.URL, s.Close
}

func newRandomBlob(size int) (digest.Digest, []byte) {
	b := make([]byte, size)
	if n, err := rand.Read(b); err != nil {
		panic(err)
	} else if n != size {
		panic("unable to read enough bytes")
	}

	return digest.FromBytes(b), b
}

func addTestFetch(repo string, dgst digest.Digest, content []byte, m *testutil.RequestResponseMap) {
	*m = append(*m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodGet,
			Route:  "/v2/" + repo + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Body:       content,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(content))},
				"Content-Type":   {"application/octet-stream"},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	*m = append(*m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(content))},
				"Content-Type":   {"application/octet-stream"},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})
}

func addTestCatalog(route string, content []byte, link string, m *testutil.RequestResponseMap) {
	headers := map[string][]string{
		"Content-Length": {strconv.Itoa(len(content))},
		"Content-Type":   {"application/json"},
	}
	if link != "" {
		headers["Link"] = append(headers["Link"], link)
	}

	*m = append(*m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodGet,
			Route:  route,
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Body:       content,
			Headers:    http.Header(headers),
		},
	})
}

func TestBlobServeBlob(t *testing.T) {
	dgst, blob := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	addTestFetch("test.example.com/repo1", dgst, blob, &m)

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	repo, _ := reference.WithName("test.example.com/repo1")
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	err = l.ServeBlob(ctx, resp, req, dgst)
	if err != nil {
		t.Errorf("Error serving blob: %s", err.Error())
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading response body: %s", err.Error())
	}
	if string(body) != string(blob) {
		t.Errorf("Unexpected response body. Got %q, expected %q", string(body), string(blob))
	}

	expectedHeaders := []struct {
		Name  string
		Value string
	}{
		{Name: "Content-Length", Value: "1024"},
		{Name: "Content-Type", Value: "application/octet-stream"},
		{Name: "Docker-Content-Digest", Value: dgst.String()},
		{Name: "Etag", Value: dgst.String()},
	}

	for _, h := range expectedHeaders {
		if resp.Header().Get(h.Name) != h.Value {
			t.Errorf("Unexpected %s. Got %s, expected %s", h.Name, resp.Header().Get(h.Name), h.Value)
		}
	}
}

func TestBlobServeBlobHEAD(t *testing.T) {
	dgst, blob := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	addTestFetch("test.example.com/repo1", dgst, blob, &m)

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	repo, _ := reference.WithName("test.example.com/repo1")
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/", nil)

	err = l.ServeBlob(ctx, resp, req, dgst)
	if err != nil {
		t.Errorf("Error serving blob: %s", err.Error())
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("Error reading response body: %s", err.Error())
	}
	if string(body) != "" {
		t.Errorf("Unexpected response body. Got %q, expected %q", string(body), "")
	}

	expectedHeaders := []struct {
		Name  string
		Value string
	}{
		{Name: "Content-Length", Value: "1024"},
		{Name: "Content-Type", Value: "application/octet-stream"},
		{Name: "Docker-Content-Digest", Value: dgst.String()},
		{Name: "Etag", Value: dgst.String()},
	}

	for _, h := range expectedHeaders {
		if resp.Header().Get(h.Name) != h.Value {
			t.Errorf("Unexpected %s. Got %s, expected %s", h.Name, resp.Header().Get(h.Name), h.Value)
		}
	}
}

func TestBlobResume(t *testing.T) {
	dgst, b1 := newRandomBlob(1024)
	id := uuid.NewString()
	var m testutil.RequestResponseMap
	repo, _ := reference.WithName("test.example.com/repo1")
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPatch,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + id,
			Body:   b1,
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Docker-Content-Digest": {dgst.String()},
				"Range":                 {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPut,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + id,
			QueryParams: map[string][]string{
				"digest": {dgst.String()},
			},
		},
		Response: testutil.Response{
			StatusCode: http.StatusCreated,
			Headers: http.Header(map[string][]string{
				"Docker-Content-Digest": {dgst.String()},
				"Content-Range":         {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(b1))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	l := r.Blobs(ctx)
	upload, err := l.Resume(ctx, id)
	if err != nil {
		t.Errorf("Error resuming blob: %s", err.Error())
	}

	if upload.ID() != id {
		t.Errorf("Unexpected UUID %s; expected %s", upload.ID(), id)
	}

	n, err := upload.ReadFrom(bytes.NewReader(b1))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(b1)) {
		t.Fatalf("Unexpected ReadFrom length: %d; expected: %d", n, len(b1))
	}

	blob, err := upload.Commit(ctx, distribution.Descriptor{
		Digest: dgst,
		Size:   int64(len(b1)),
	})
	if err != nil {
		t.Fatal(err)
	}

	if blob.Size != int64(len(b1)) {
		t.Fatalf("Unexpected blob size: %d; expected: %d", blob.Size, len(b1))
	}
}

func TestBlobDelete(t *testing.T) {
	dgst, _ := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	repo, _ := reference.WithName("test.example.com/repo1")
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodDelete,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length": {"0"},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)
	err = l.Delete(ctx, dgst)
	if err != nil {
		t.Errorf("Error deleting blob: %s", err.Error())
	}
}

func TestBlobFetch(t *testing.T) {
	d1, b1 := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	addTestFetch("test.example.com/repo1", d1, b1, &m)

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	repo, _ := reference.WithName("test.example.com/repo1")
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	b, err := l.Get(ctx, d1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, b1) {
		t.Fatalf("Wrong bytes values fetched: [%d]byte != [%d]byte", len(b), len(b1))
	}

	// TODO(dmcgowan): Test for unknown blob case
}

func TestBlobExistsNoContentLength(t *testing.T) {
	var m testutil.RequestResponseMap

	repo, _ := reference.WithName("biff")
	dgst, content := newRandomBlob(1024)
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodGet,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Body:       content,
			Headers: http.Header(map[string][]string{
				//			"Content-Length": {fmt.Sprint(len(content))},
				"Last-Modified": {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				//			"Content-Length": {fmt.Sprint(len(content))},
				"Last-Modified": {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})
	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	_, err = l.Stat(ctx, dgst)
	if err == nil {
		t.Fatal(err)
	}
	if !strings.Contains(err.Error(), "missing content-length heade") {
		t.Fatalf("Expected missing content-length error message")
	}
}

func TestBlobExists(t *testing.T) {
	d1, b1 := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	addTestFetch("test.example.com/repo1", d1, b1, &m)

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	repo, _ := reference.WithName("test.example.com/repo1")
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	stat, err := l.Stat(ctx, d1)
	if err != nil {
		t.Fatal(err)
	}

	if stat.Digest != d1 {
		t.Fatalf("Unexpected digest: %s, expected %s", stat.Digest, d1)
	}

	if stat.Size != int64(len(b1)) {
		t.Fatalf("Unexpected length: %d, expected %d", stat.Size, len(b1))
	}

	// TODO(dmcgowan): Test error cases and ErrBlobUnknown case
}

func TestBlobUploadChunked(t *testing.T) {
	dgst, b1 := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	chunks := [][]byte{
		b1[0:256],
		b1[256:512],
		b1[512:513],
		b1[513:1024],
	}
	repo, _ := reference.WithName("test.example.com/uploadrepo")
	uuids := []string{uuid.NewString()}
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPost,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/",
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length":     {"0"},
				"Location":           {"/v2/" + repo.Name() + "/blobs/uploads/" + uuids[0]},
				"Docker-Upload-UUID": {uuids[0]},
				"Range":              {"0-0"},
			}),
		},
	})
	offset := 0
	for i, chunk := range chunks {
		uuids = append(uuids, uuid.NewString())
		newOffset := offset + len(chunk)
		m = append(m, testutil.RequestResponseMapping{
			Request: testutil.Request{
				Method: http.MethodPatch,
				Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + uuids[i],
				Body:   chunk,
			},
			Response: testutil.Response{
				StatusCode: http.StatusAccepted,
				Headers: http.Header(map[string][]string{
					"Content-Length":     {"0"},
					"Location":           {"/v2/" + repo.Name() + "/blobs/uploads/" + uuids[i+1]},
					"Docker-Upload-UUID": {uuids[i+1]},
					"Range":              {fmt.Sprintf("%d-%d", offset, newOffset-1)},
				}),
			},
		})
		offset = newOffset
	}
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPut,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + uuids[len(uuids)-1],
			QueryParams: map[string][]string{
				"digest": {dgst.String()},
			},
		},
		Response: testutil.Response{
			StatusCode: http.StatusCreated,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
				"Content-Range":         {fmt.Sprintf("0-%d", offset-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(offset)},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	upload, err := l.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if upload.ID() != uuids[0] {
		log.Fatalf("Unexpected UUID %s; expected %s", upload.ID(), uuids[0])
	}

	for _, chunk := range chunks {
		n, err := upload.Write(chunk)
		if err != nil {
			t.Fatal(err)
		}
		if n != len(chunk) {
			t.Fatalf("Unexpected length returned from write: %d; expected: %d", n, len(chunk))
		}
	}

	blob, err := upload.Commit(ctx, distribution.Descriptor{
		Digest: dgst,
		Size:   int64(len(b1)),
	})
	if err != nil {
		t.Fatal(err)
	}

	if blob.Size != int64(len(b1)) {
		t.Fatalf("Unexpected blob size: %d; expected: %d", blob.Size, len(b1))
	}
}

func TestBlobUploadMonolithic(t *testing.T) {
	dgst, b1 := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	repo, _ := reference.WithName("test.example.com/uploadrepo")
	uploadID := uuid.NewString()
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPost,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/",
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length":     {"0"},
				"Location":           {"/v2/" + repo.Name() + "/blobs/uploads/" + uploadID},
				"Docker-Upload-UUID": {uploadID},
				"Range":              {"0-0"},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPatch,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + uploadID,
			Body:   b1,
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Location":              {"/v2/" + repo.Name() + "/blobs/uploads/" + uploadID},
				"Docker-Upload-UUID":    {uploadID},
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
				"Range":                 {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPut,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + uploadID,
			QueryParams: map[string][]string{
				"digest": {dgst.String()},
			},
		},
		Response: testutil.Response{
			StatusCode: http.StatusCreated,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
				"Content-Range":         {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(b1))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	upload, err := l.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if upload.ID() != uploadID {
		log.Fatalf("Unexpected UUID %s; expected %s", upload.ID(), uploadID)
	}

	n, err := upload.ReadFrom(bytes.NewReader(b1))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(b1)) {
		t.Fatalf("Unexpected ReadFrom length: %d; expected: %d", n, len(b1))
	}

	blob, err := upload.Commit(ctx, distribution.Descriptor{
		Digest: dgst,
		Size:   int64(len(b1)),
	})
	if err != nil {
		t.Fatal(err)
	}

	if blob.Size != int64(len(b1)) {
		t.Fatalf("Unexpected blob size: %d; expected: %d", blob.Size, len(b1))
	}
}

func TestBlobUploadMonolithicDockerUploadUUIDFromURL(t *testing.T) {
	dgst, b1 := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	repo, _ := reference.WithName("test.example.com/uploadrepo")
	uploadID := uuid.NewString()
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPost,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/",
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length": {"0"},
				"Location":       {"/v2/" + repo.Name() + "/blobs/uploads/" + uploadID},
				"Range":          {"0-0"},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPatch,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + uploadID,
			Body:   b1,
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Location":              {"/v2/" + repo.Name() + "/blobs/uploads/" + uploadID},
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
				"Range":                 {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPut,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/" + uploadID,
			QueryParams: map[string][]string{
				"digest": {dgst.String()},
			},
		},
		Response: testutil.Response{
			StatusCode: http.StatusCreated,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
				"Content-Range":         {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(b1))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	upload, err := l.Create(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if upload.ID() != uploadID {
		log.Fatalf("Unexpected UUID %s; expected %s", upload.ID(), uploadID)
	}

	n, err := upload.ReadFrom(bytes.NewReader(b1))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(b1)) {
		t.Fatalf("Unexpected ReadFrom length: %d; expected: %d", n, len(b1))
	}

	blob, err := upload.Commit(ctx, distribution.Descriptor{
		Digest: dgst,
		Size:   int64(len(b1)),
	})
	if err != nil {
		t.Fatal(err)
	}

	if blob.Size != int64(len(b1)) {
		t.Fatalf("Unexpected blob size: %d; expected: %d", blob.Size, len(b1))
	}
}

func TestBlobUploadMonolithicNoDockerUploadUUID(t *testing.T) {
	dgst, b1 := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	repo, _ := reference.WithName("test.example.com/uploadrepo")
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPost,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/",
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length": {"0"},
				"Location":       {"/v2/" + repo.Name() + "/blobs/uploads/"},
				"Range":          {"0-0"},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPatch,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/",
			Body:   b1,
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Location":              {"/v2/" + repo.Name() + "/blobs/uploads/"},
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
				"Range":                 {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPut,
			Route:  "/v2/" + repo.Name() + "/blobs/uploads/",
			QueryParams: map[string][]string{
				"digest": {dgst.String()},
			},
		},
		Response: testutil.Response{
			StatusCode: http.StatusCreated,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
				"Content-Range":         {fmt.Sprintf("0-%d", len(b1)-1)},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(b1))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := r.Blobs(ctx)

	upload, err := l.Create(ctx)

	if err.Error() != "cannot retrieve docker upload UUID" {
		log.Fatalf("expected rejection to retrieve docker upload UUID error. Got %q", err)
	}

	if upload != nil {
		log.Fatal("Expected upload to be nil")
	}
}

func TestBlobMount(t *testing.T) {
	dgst, content := newRandomBlob(1024)
	var m testutil.RequestResponseMap
	repo, _ := reference.WithName("test.example.com/uploadrepo")

	sourceRepo, _ := reference.WithName("test.example.com/sourcerepo")
	canonicalRef, _ := reference.WithDigest(sourceRepo, dgst)

	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method:      http.MethodPost,
			Route:       "/v2/" + repo.Name() + "/blobs/uploads/",
			QueryParams: map[string][]string{"from": {sourceRepo.Name()}, "mount": {dgst.String()}},
		},
		Response: testutil.Response{
			StatusCode: http.StatusCreated,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {"0"},
				"Location":              {"/v2/" + repo.Name() + "/blobs/" + dgst.String()},
				"Docker-Content-Digest": {dgst.String()},
			}),
		},
	})
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/blobs/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(content))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	l := r.Blobs(ctx)

	bw, err := l.Create(ctx, WithMountFrom(canonicalRef))
	if bw != nil {
		t.Fatalf("Expected blob writer to be nil, was %v", bw)
	}

	if ebm, ok := err.(distribution.ErrBlobMounted); ok {
		if ebm.From.Digest() != dgst {
			t.Fatalf("Unexpected digest: %s, expected %s", ebm.From.Digest(), dgst)
		}
		if ebm.From.Name() != sourceRepo.Name() {
			t.Fatalf("Unexpected from: %s, expected %s", ebm.From.Name(), sourceRepo)
		}
	} else {
		t.Fatalf("Unexpected error: %v, expected an ErrBlobMounted", err)
	}
}

func newRandomOCIManifest(t *testing.T, blobCount int) (*ocischema.Manifest, digest.Digest, []byte) {
	layers := make([]distribution.Descriptor, blobCount)
	for i := 0; i < blobCount; i++ {
		dgst, blob := newRandomBlob((i % 5) * 16)
		layers[i] = distribution.Descriptor{
			MediaType: v1.MediaTypeImageLayer,
			Digest:    dgst,
			Size:      int64(len(blob)),
		}
	}

	m := ocischema.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     v1.MediaTypeImageManifest,
		},
		Config: distribution.Descriptor{
			Digest:    "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:      123,
			MediaType: v1.MediaTypeImageConfig,
		},
		Layers: layers,
	}

	sm, err := ocischema.FromStruct(m)
	if err != nil {
		t.Fatal(err)
	}

	_, payload, _ := sm.Payload()

	return &m, digest.FromBytes(payload), payload
}

func addTestManifestWithEtag(repo reference.Named, reference string, content []byte, m *testutil.RequestResponseMap, dgst string) {
	actualDigest := digest.FromBytes(content)
	getReqWithEtag := testutil.Request{
		Method: http.MethodGet,
		Route:  "/v2/" + repo.Name() + "/manifests/" + reference,
		Headers: http.Header(map[string][]string{
			"If-None-Match": {fmt.Sprintf(`"%s"`, dgst)},
		}),
	}

	var getRespWithEtag testutil.Response
	if actualDigest.String() == dgst {
		getRespWithEtag = testutil.Response{
			StatusCode: http.StatusNotModified,
			Body:       []byte{},
			Headers: http.Header(map[string][]string{
				"Content-Length": {"0"},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
				"Content-Type":   {v1.MediaTypeImageManifest},
			}),
		}
	} else {
		getRespWithEtag = testutil.Response{
			StatusCode: http.StatusOK,
			Body:       content,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(content))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
				"Content-Type":   {v1.MediaTypeImageManifest},
			}),
		}
	}
	*m = append(*m, testutil.RequestResponseMapping{Request: getReqWithEtag, Response: getRespWithEtag})
}

func addTestManifest(repo reference.Named, reference string, mediatype string, content []byte, m *testutil.RequestResponseMap) {
	*m = append(*m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodGet,
			Route:  "/v2/" + repo.Name() + "/manifests/" + reference,
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Body:       content,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {fmt.Sprint(len(content))},
				"Last-Modified":         {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
				"Content-Type":          {mediatype},
				"Docker-Content-Digest": {digest.FromBytes(content).String()},
			}),
		},
	})
	*m = append(*m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/manifests/" + reference,
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {fmt.Sprint(len(content))},
				"Last-Modified":         {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
				"Content-Type":          {mediatype},
				"Docker-Content-Digest": {digest.Canonical.FromBytes(content).String()},
			}),
		},
	})
}

func addTestManifestWithoutDigestHeader(repo reference.Named, reference string, mediatype string, content []byte, m *testutil.RequestResponseMap) {
	*m = append(*m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodGet,
			Route:  "/v2/" + repo.Name() + "/manifests/" + reference,
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Body:       content,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(content))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
				"Content-Type":   {mediatype},
			}),
		},
	})
	*m = append(*m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodHead,
			Route:  "/v2/" + repo.Name() + "/manifests/" + reference,
		},
		Response: testutil.Response{
			StatusCode: http.StatusOK,
			Headers: http.Header(map[string][]string{
				"Content-Length": {fmt.Sprint(len(content))},
				"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
				"Content-Type":   {mediatype},
			}),
		},
	})
}

func checkEqualManifest(m1, m2 *ocischema.DeserializedManifest) error {
	if !reflect.DeepEqual(m1.Versioned, m2.Versioned) {
		return fmt.Errorf("versions do not match: %v != %v", m1.Versioned, m2.Versioned)
	}
	if !reflect.DeepEqual(m1.Config, m2.Config) {
		return fmt.Errorf("config do not match: %v != %v", m1.Config, m2.Config)
	}
	if !reflect.DeepEqual(m1.Layers, m2.Layers) {
		return fmt.Errorf("layers do not match: %v != %v", m1.Layers, m2.Layers)
	}

	return nil
}

func TestOCIManifestFetch(t *testing.T) {
	ctx := dcontext.Background()
	repo, _ := reference.WithName("test.example.com/repo")
	m1, dgst, pl := newRandomOCIManifest(t, 6)
	var m testutil.RequestResponseMap
	addTestManifest(repo, dgst.String(), v1.MediaTypeImageManifest, pl, &m)
	addTestManifest(repo, "latest", v1.MediaTypeImageManifest, pl, &m)
	addTestManifest(repo, "badcontenttype", "text/html", pl, &m)

	e, c := testServer(m)
	defer c()

	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := r.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := ms.Exists(ctx, dgst)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Manifest does not exist")
	}

	manifest, err := ms.Get(ctx, dgst)
	if err != nil {
		t.Fatal(err)
	}
	ociManifest, ok := manifest.(*ocischema.DeserializedManifest)
	if !ok {
		t.Fatalf("Unexpected manifest type from Get: %T", manifest)
	}

	dm1, err := ocischema.FromStruct(*m1)
	if err != nil {
		t.Fatal(err)
	}

	if err := checkEqualManifest(ociManifest, dm1); err != nil {
		t.Fatal(err)
	}

	var contentDigest digest.Digest
	manifest, err = ms.Get(ctx, dgst, distribution.WithTag("latest"), ReturnContentDigest(&contentDigest))
	if err != nil {
		t.Fatal(err)
	}
	ociManifest, ok = manifest.(*ocischema.DeserializedManifest)
	if !ok {
		t.Fatalf("Unexpected manifest type from Get: %T", manifest)
	}

	if err = checkEqualManifest(ociManifest, dm1); err != nil {
		t.Fatal(err)
	}

	if contentDigest != dgst {
		t.Fatalf("Unexpected returned content digest %v, expected %v", contentDigest, dgst)
	}

	// TODO(milosgajdos): once the schema1 manifest package is removed we need to
	// return some predefined error from distribution.UnmarshalManifest() for the cases
	// where empty mediaType/ctHeader is provided; currently this is handled by schema1 unmarshaler.
	// Ideally we'd like to returns something like UnsupportedManifest error and assert it in this test.
	_, err = ms.Get(ctx, dgst, distribution.WithTag("badcontenttype"))
	if err == nil {
		t.Fatal("expected to fail")
	}
}

func TestManifestFetchWithEtag(t *testing.T) {
	repo, _ := reference.WithName("test.example.com/repo/by/tag")
	_, d1, p1 := newRandomOCIManifest(t, 6)
	var m testutil.RequestResponseMap
	addTestManifestWithEtag(repo, "latest", p1, &m, d1.String())

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	ms, err := r.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	clientManifestService, ok := ms.(*manifests)
	if !ok {
		panic("wrong type for client manifest service")
	}
	_, err = clientManifestService.Get(ctx, d1, distribution.WithTag("latest"), AddEtagToTag("latest", d1.String()))
	if err != distribution.ErrManifestNotModified {
		t.Fatal(err)
	}
}

func TestManifestFetchWithAccept(t *testing.T) {
	ctx := dcontext.Background()
	repo, _ := reference.WithName("test.example.com/repo")
	_, dgst, _ := newRandomOCIManifest(t, 6)
	headers := make(chan []string, 1)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		headers <- req.Header["Accept"]
	}))
	defer close(headers)
	defer s.Close()

	r, err := NewRepository(repo, s.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := r.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		// the media types we send
		mediaTypes []string
		// the expected Accept headers the server should receive
		expect []string
		// whether to sort the request and response values for comparison
		sort bool
	}{
		{
			mediaTypes: []string{},
			expect:     distribution.ManifestMediaTypes(),
			sort:       true,
		},
		{
			mediaTypes: []string{"test1", "test2"},
			expect:     []string{"test1", "test2"},
		},
		{
			mediaTypes: []string{"test1"},
			expect:     []string{"test1"},
		},
		{
			mediaTypes: []string{""},
			expect:     []string{""},
		},
	}
	for _, testCase := range testCases {
		// NOTE(milosgajdos): we are not checking error values here because this test
		// is not storing any manifests, so this will inevitably error out.
		// This test is about checking if the Accept headers are returned as expected.
		// nolint:errcheck
		ms.Get(ctx, dgst, distribution.WithManifestMediaTypes(testCase.mediaTypes))
		actual := <-headers
		if testCase.sort {
			sort.Strings(actual)
			sort.Strings(testCase.expect)
		}
		if !reflect.DeepEqual(actual, testCase.expect) {
			t.Fatalf("unexpected Accept header values: %v", actual)
		}
	}
}

func TestManifestDelete(t *testing.T) {
	repo, _ := reference.WithName("test.example.com/repo/delete")
	_, dgst1, _ := newRandomOCIManifest(t, 6)
	_, dgst2, _ := newRandomOCIManifest(t, 6)
	var m testutil.RequestResponseMap
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodDelete,
			Route:  "/v2/" + repo.Name() + "/manifests/" + dgst1.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length": {"0"},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := dcontext.Background()
	ms, err := r.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := ms.Delete(ctx, dgst1); err != nil {
		t.Fatal(err)
	}
	if err := ms.Delete(ctx, dgst2); err == nil {
		t.Fatal("Expected error deleting unknown manifest")
	}
}

func TestManifestPut(t *testing.T) {
	repo, _ := reference.WithName("test.example.com/repo/delete")
	m1, dgst, payload := newRandomOCIManifest(t, 6)

	var m testutil.RequestResponseMap
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPut,
			Route:  "/v2/" + repo.Name() + "/manifests/sometag",
			Body:   payload,
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {dgst.String()},
			}),
		},
	})

	putDgst := digest.FromBytes(payload)
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodPut,
			Route:  "/v2/" + repo.Name() + "/manifests/" + putDgst.String(),
			Body:   payload,
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: http.Header(map[string][]string{
				"Content-Length":        {"0"},
				"Docker-Content-Digest": {putDgst.String()},
			}),
		},
	})

	e, c := testServer(m)
	defer c()

	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := dcontext.Background()
	ms, err := r.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	dm, err := ocischema.FromStruct(*m1)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ms.Put(ctx, dm, distribution.WithTag("sometag")); err != nil {
		t.Fatal(err)
	}

	if _, err := ms.Put(ctx, dm); err != nil {
		t.Fatal(err)
	}
}

func TestManifestTags(t *testing.T) {
	repo, _ := reference.WithName("test.example.com/repo/tags/list")
	tagsList := []byte(strings.TrimSpace(`
{
	"name": "test.example.com/repo/tags/list",
	"tags": [
		"tag1",
		"tag2",
		"funtag"
	]
}
	`))
	var m testutil.RequestResponseMap
	for i := 0; i < 3; i++ {
		m = append(m, testutil.RequestResponseMapping{
			Request: testutil.Request{
				Method: http.MethodGet,
				Route:  "/v2/" + repo.Name() + "/tags/list",
			},
			Response: testutil.Response{
				StatusCode: http.StatusOK,
				Body:       tagsList,
				Headers: http.Header(map[string][]string{
					"Content-Length": {fmt.Sprint(len(tagsList))},
					"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
				}),
			},
		})
	}
	e, c := testServer(m)
	defer c()

	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := dcontext.Background()
	tagService := r.Tags(ctx)

	tags, err := tagService.All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("Wrong number of tags returned: %d, expected 3", len(tags))
	}

	expected := map[string]struct{}{
		"tag1":   {},
		"tag2":   {},
		"funtag": {},
	}
	for _, t := range tags {
		delete(expected, t)
	}
	if len(expected) != 0 {
		t.Fatalf("unexpected tags returned: %v", expected)
	}
	// TODO(dmcgowan): Check for error cases
}

func TestTagDelete(t *testing.T) {
	tag := "latest"
	repo, _ := reference.WithName("test.example.com/repo/delete")
	newRandomOCIManifest(t, 1)

	var m testutil.RequestResponseMap
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodDelete,
			Route:  "/v2/" + repo.Name() + "/manifests/" + tag,
		},
		Response: testutil.Response{
			StatusCode: http.StatusAccepted,
			Headers: map[string][]string{
				"Content-Length": {"0"},
			},
		},
	})

	e, c := testServer(m)
	defer c()

	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := dcontext.Background()
	ts := r.Tags(ctx)

	if err := ts.Untag(ctx, tag); err != nil {
		t.Fatal(err)
	}
	if err := ts.Untag(ctx, tag); err == nil {
		t.Fatal("expected error deleting unknown tag")
	}
}

func TestObtainsErrorForMissingTag(t *testing.T) {
	repo, _ := reference.WithName("test.example.com/repo")

	var m testutil.RequestResponseMap
	var errors errcode.Errors
	errors = append(errors, errcode.ErrorCodeManifestUnknown.WithDetail("unknown manifest"))
	errBytes, err := json.Marshal(errors)
	if err != nil {
		t.Fatal(err)
	}
	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodGet,
			Route:  "/v2/" + repo.Name() + "/manifests/1.0.0",
		},
		Response: testutil.Response{
			StatusCode: http.StatusNotFound,
			Body:       errBytes,
			Headers: http.Header(map[string][]string{
				"Content-Type": {"application/json"},
			}),
		},
	})
	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	tagService := r.Tags(ctx)

	_, err = tagService.Get(ctx, "1.0.0")
	if err == nil {
		t.Fatalf("Expected an error")
	}
	if !strings.Contains(err.Error(), "manifest unknown") {
		t.Fatalf("Expected unknown manifest error message")
	}
}

func TestObtainsManifestForTagWithoutHeaders(t *testing.T) {
	repo, _ := reference.WithName("test.example.com/repo")

	var m testutil.RequestResponseMap
	_, dgst, pl := newRandomOCIManifest(t, 6)
	addTestManifestWithoutDigestHeader(repo, "1.0.0", v1.MediaTypeImageManifest, pl, &m)

	e, c := testServer(m)
	defer c()

	ctx := dcontext.Background()
	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}

	tagService := r.Tags(ctx)

	desc, err := tagService.Get(ctx, "1.0.0")
	if err != nil {
		t.Fatalf("Expected no error")
	}
	if desc.Digest != dgst {
		t.Fatalf("Unexpected digest")
	}
}

func TestManifestTagsPaginated(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()

	repo, _ := reference.WithName("test.example.com/repo/tags/list")
	tagsList := []string{"tag1", "tag2", "funtag"}
	var m testutil.RequestResponseMap
	for i := 0; i < 3; i++ {
		body, err := json.Marshal(map[string]interface{}{
			"name": "test.example.com/repo/tags/list",
			"tags": []string{tagsList[i]},
		})
		if err != nil {
			t.Fatal(err)
		}
		queryParams := make(map[string][]string)
		if i > 0 {
			queryParams["n"] = []string{"1"}
			queryParams["last"] = []string{tagsList[i-1]}
		}

		// Test both relative and absolute links.
		relativeLink := "/v2/" + repo.Name() + "/tags/list?n=1&last=" + tagsList[i]
		var link string
		switch i {
		case 0:
			link = relativeLink
		case len(tagsList) - 1:
			link = ""
		default:
			link = s.URL + relativeLink
		}

		headers := http.Header(map[string][]string{
			"Content-Length": {fmt.Sprint(len(body))},
			"Last-Modified":  {time.Now().Add(-1 * time.Second).Format(time.ANSIC)},
		})
		if link != "" {
			headers.Set("Link", fmt.Sprintf(`<%s>; rel="next"`, link))
		}

		m = append(m, testutil.RequestResponseMapping{
			Request: testutil.Request{
				Method:      http.MethodGet,
				Route:       "/v2/" + repo.Name() + "/tags/list",
				QueryParams: queryParams,
			},
			Response: testutil.Response{
				StatusCode: http.StatusOK,
				Body:       body,
				Headers:    headers,
			},
		})
	}

	s.Config.Handler = testutil.NewHandler(m)

	r, err := NewRepository(repo, s.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := dcontext.Background()
	tagService := r.Tags(ctx)

	tags, err := tagService.All(ctx)
	if err != nil {
		t.Fatal(tags, err)
	}
	if len(tags) != 3 {
		t.Fatalf("Wrong number of tags returned: %d, expected 3", len(tags))
	}

	expected := map[string]struct{}{
		"tag1":   {},
		"tag2":   {},
		"funtag": {},
	}
	for _, t := range tags {
		delete(expected, t)
	}
	if len(expected) != 0 {
		t.Fatalf("unexpected tags returned: %v", expected)
	}
}

func TestManifestUnauthorized(t *testing.T) {
	repo, _ := reference.WithName("test.example.com/repo")
	_, dgst, _ := newRandomOCIManifest(t, 6)
	var m testutil.RequestResponseMap

	m = append(m, testutil.RequestResponseMapping{
		Request: testutil.Request{
			Method: http.MethodGet,
			Route:  "/v2/" + repo.Name() + "/manifests/" + dgst.String(),
		},
		Response: testutil.Response{
			StatusCode: http.StatusUnauthorized,
			Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
			Body:       []byte("<html>garbage</html>"),
		},
	})

	e, c := testServer(m)
	defer c()

	r, err := NewRepository(repo, e, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := dcontext.Background()
	ms, err := r.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, err = ms.Get(ctx, dgst)
	if err == nil {
		t.Fatal("Expected error fetching manifest")
	}
	v2Err, ok := err.(errcode.Error)
	if !ok {
		t.Fatalf("Unexpected error type: %#v", err)
	}
	if v2Err.Code != errcode.ErrorCodeUnauthorized {
		t.Fatalf("Unexpected error code: %s", v2Err.Code.String())
	}
	if expected := errcode.ErrorCodeUnauthorized.Message(); v2Err.Message != expected {
		t.Fatalf("Unexpected message value: %q, expected %q", v2Err.Message, expected)
	}
}

func TestCatalog(t *testing.T) {
	var m testutil.RequestResponseMap
	addTestCatalog(
		"/v2/_catalog?n=5",
		[]byte("{\"repositories\":[\"foo\", \"bar\", \"baz\"]}"), "", &m)

	e, c := testServer(m)
	defer c()

	entries := make([]string, 5)

	r, err := NewRegistry(e, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := dcontext.Background()
	numFilled, err := r.Repositories(ctx, entries, "")
	if err != io.EOF {
		t.Fatal(err)
	}

	if numFilled != 3 {
		t.Fatalf("Got wrong number of repos")
	}
}

func TestCatalogInParts(t *testing.T) {
	var m testutil.RequestResponseMap
	addTestCatalog(
		"/v2/_catalog?n=2",
		[]byte("{\"repositories\":[\"bar\", \"baz\"]}"),
		"</v2/_catalog?last=baz&n=2>", &m)
	addTestCatalog(
		"/v2/_catalog?last=baz&n=2",
		[]byte("{\"repositories\":[\"foo\"]}"),
		"", &m)

	e, c := testServer(m)
	defer c()

	entries := make([]string, 2)

	r, err := NewRegistry(e, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := dcontext.Background()
	numFilled, err := r.Repositories(ctx, entries, "")
	if err != nil {
		t.Fatal(err)
	}

	if numFilled != 2 {
		t.Fatalf("Got wrong number of repos")
	}

	numFilled, err = r.Repositories(ctx, entries, "baz")
	if err != io.EOF {
		t.Fatal(err)
	}

	if numFilled != 1 {
		t.Fatalf("Got wrong number of repos")
	}
}

func TestSanitizeLocation(t *testing.T) {
	for _, testcase := range []struct {
		description string
		location    string
		source      string
		expected    string
		err         error
	}{
		{
			description: "ensure relative location correctly resolved",
			location:    "/v2/foo/baasdf",
			source:      "http://blahalaja.com/v1",
			expected:    "http://blahalaja.com/v2/foo/baasdf",
		},
		{
			description: "ensure parameters are preserved",
			location:    "/v2/foo/baasdf?_state=asdfasfdasdfasdf&digest=foo",
			source:      "http://blahalaja.com/v1",
			expected:    "http://blahalaja.com/v2/foo/baasdf?_state=asdfasfdasdfasdf&digest=foo",
		},
		{
			description: "ensure new hostname overridden",
			location:    "https://mwhahaha.com/v2/foo/baasdf?_state=asdfasfdasdfasdf",
			source:      "http://blahalaja.com/v1",
			expected:    "https://mwhahaha.com/v2/foo/baasdf?_state=asdfasfdasdfasdf",
		},
	} {
		fatalf := func(format string, args ...interface{}) {
			t.Fatalf(testcase.description+": "+format, args...)
		}

		s, err := sanitizeLocation(testcase.location, testcase.source)
		if err != testcase.err {
			if testcase.err != nil {
				fatalf("expected error: %v != %v", err, testcase)
			} else {
				fatalf("unexpected error sanitizing: %v", err)
			}
		}

		if s != testcase.expected {
			fatalf("bad sanitize: %q != %q", s, testcase.expected)
		}
	}
}

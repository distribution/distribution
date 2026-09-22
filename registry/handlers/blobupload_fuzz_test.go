package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/docker/distribution/configuration"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
)

// FuzzBlobUploadSession drives the resumable blob upload API over real HTTP.
//
// Threat model coverage (registry-threat-model.md), harness 3: the blob upload
// path cannot be reached by structural fuzzing of a single document, because it
// is a session -- POST to open, PATCH to append, PUT to finalise -- whose state
// lives in the registry between requests. A fuzzed byte string is interpreted
// here as a program over that session, so the fuzzer explores orderings the
// protocol does not describe: appending after finalising, resuming a deleted
// session, contradictory Content-Range headers, finalising with a digest that
// describes different bytes.
//
// The invariants are the ones a content-addressed store cannot give up:
//
//   - No request may produce a 5xx. The registry is reached by every node in the
//     cluster; an unhandled internal error is a denial of service on the pull
//     path, and one that a client controls.
//   - A blob may only be created under a digest that describes its bytes. A PUT
//     whose digest disagrees with what was uploaded must fail, and a blob that
//     was created must read back as exactly those bytes.
//   - A session that was deleted must not accept further writes.
func FuzzBlobUploadSession(f *testing.F) {
	f.Add([]byte("payload"), []byte{opPost, opPatch, opPutCorrectDigest, opGetBlob})
	f.Add([]byte(""), []byte{opPost, opPutCorrectDigest, opGetBlob})
	f.Add([]byte("payload"), []byte{opPost, opPatch, opPutWrongDigest, opGetBlob})
	f.Add([]byte("payload"), []byte{opPost, opPatch, opPatch, opPutCorrectDigest})
	f.Add([]byte("payload"), []byte{opPost, opPatchBadRange, opPutCorrectDigest})
	f.Add([]byte("payload"), []byte{opPost, opPatch, opDelete, opPatch, opPutCorrectDigest})
	f.Add([]byte("payload"), []byte{opPost, opPatch, opPutCorrectDigest, opPatch})
	f.Add([]byte("payload"), []byte{opPost, opStatus, opPatch, opStatus, opPutCorrectDigest})
	f.Add([]byte("payload"), []byte{opPost, opPutNoDigest})
	f.Add([]byte("payload"), []byte{opMonolithic, opGetBlob})
	f.Add([]byte("payload"), []byte{opPatch, opPutCorrectDigest})
	f.Add([]byte("payload"), []byte{opPost, opPost, opPatch, opPutCorrectDigest})
	f.Add([]byte("\x00\x01\x02"), []byte{opPost, opPatch, opPutCorrectDigest, opGetBlob})
	f.Add(bytes.Repeat([]byte("x"), 4096), []byte{opPost, opPatch, opPatch, opPutCorrectDigest, opGetBlob})

	f.Fuzz(func(t *testing.T, content, program []byte) {
		// A longer program adds orderings, not states; keep an iteration bounded.
		if len(program) > 16 || len(content) > 1<<16 {
			return
		}

		env := blobFuzzEnv(t)
		session := &uploadSession{
			env:        env,
			t:          t,
			repository: fmt.Sprintf("fuzz/repo%d", atomic.AddUint64(&blobFuzzRepo, 1)),
			content:    content,
		}

		for _, op := range program {
			session.run(op)
		}

		session.verifyBlob()
	})
}

// Operations a fuzzed program byte can select.
const (
	opPost = iota
	opPatch
	opPatchBadRange
	opPutCorrectDigest
	opPutWrongDigest
	opPutNoDigest
	opStatus
	opDelete
	opGetBlob
	opMonolithic
	opCount
)

var (
	blobFuzzOnce   sync.Once
	blobFuzzServer *httptest.Server
	blobFuzzRepo   uint64
)

// blobFuzzEnv starts one registry per process: the sessions are isolated by
// repository name, so a shared server keeps an iteration cheap.
func blobFuzzEnv(t *testing.T) *httptest.Server {
	blobFuzzOnce.Do(func() {
		config := configuration.Configuration{
			Storage: configuration.Storage{
				"inmemory": configuration.Parameters{},
				"delete":   configuration.Parameters{"enabled": true},
				"maintenance": configuration.Parameters{"uploadpurging": map[interface{}]interface{}{
					"enabled": false,
				}},
			},
		}
		app := NewApp(context.Background(), &config)
		blobFuzzServer = httptest.NewServer(app)
	})

	if blobFuzzServer == nil {
		t.Fatal("the fuzz registry did not start")
	}
	return blobFuzzServer
}

// uploadSession tracks what the program has done, so that the oracle knows what
// the registry is allowed to hold.
type uploadSession struct {
	env        *httptest.Server
	t          *testing.T
	repository string
	content    []byte

	location string // current upload session, empty when there is none
	offset   int    // bytes the program believes it has appended
	deleted  bool   // the session was deleted

	// created records the digest of a blob the registry reported as created,
	// together with the bytes it was created from.
	created     digest.Digest
	createdFrom []byte
}

func (s *uploadSession) run(op byte) {
	switch int(op) % opCount {
	case opPost:
		s.post("")
	case opPatch:
		s.patch(false)
	case opPatchBadRange:
		s.patch(true)
	case opPutCorrectDigest:
		s.put(digest.FromBytes(s.content).String())
	case opPutWrongDigest:
		// A digest of different bytes: accepting this would let a client store
		// content under a name that does not describe it.
		s.put(digest.FromBytes(append([]byte("other"), s.content...)).String())
	case opPutNoDigest:
		s.put("")
	case opStatus:
		s.status()
	case opDelete:
		s.del()
	case opGetBlob:
		s.verifyBlob()
	case opMonolithic:
		s.post(digest.FromBytes(s.content).String())
	}
}

func (s *uploadSession) uploadsURL() string {
	return s.env.URL + "/v2/" + s.repository + "/blobs/uploads/"
}

// do performs the request and enforces the no-5xx invariant on every response.
//
// The body is always read to completion and replaced with an equivalent reader.
// Draining is what lets the transport reuse the connection; without it the
// fuzzer exhausts the ephemeral port range within seconds and reports a dial
// failure that says nothing about the registry.
func (s *uploadSession) do(request *http.Request, what string) *http.Response {
	s.t.Helper()

	response, err := s.env.Client().Do(request)
	if err != nil {
		s.t.Fatalf("%s failed at the transport level: %v", what, err)
	}

	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		s.t.Fatalf("%s: cannot read the response body: %v", what, readErr)
	}
	response.Body = io.NopCloser(bytes.NewReader(body))

	if response.StatusCode >= 500 {
		s.t.Fatalf("%s produced %s, which a client must not be able to cause: %s",
			what, response.Status, truncate(body))
	}

	return response
}

func truncate(body []byte) string {
	if len(body) > 4096 {
		return string(body[:4096]) + "..."
	}
	return string(body)
}

func (s *uploadSession) post(monolithicDigest string) {
	url := s.uploadsURL()
	var body io.Reader
	if monolithicDigest != "" {
		url += "?digest=" + monolithicDigest
		body = bytes.NewReader(s.content)
	}

	request, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		s.t.Fatalf("cannot build the POST request: %v", err)
	}
	if monolithicDigest != "" {
		request.Header.Set("Content-Type", "application/octet-stream")
	}

	response := s.do(request, "POST uploads")
	defer response.Body.Close()

	if monolithicDigest != "" && response.StatusCode == http.StatusCreated {
		// Not reached today: StartBlobUpload does not implement the
		// single-request monolithic upload and answers 202 with a fresh
		// session, discarding the body. Kept so that the invariant still holds
		// if that changes.
		s.recordCreated(monolithicDigest, s.content)
		return
	}

	if response.StatusCode == http.StatusAccepted {
		s.location = response.Header.Get("Location")
		s.offset = 0
		s.deleted = false
	}
}

func (s *uploadSession) patch(badRange bool) {
	if s.location == "" {
		return
	}

	remaining := s.content[min(s.offset, len(s.content)):]
	chunk := remaining
	if len(remaining) > 1 {
		chunk = remaining[:len(remaining)/2]
	}

	request, err := http.NewRequest(http.MethodPatch, s.location, bytes.NewReader(chunk))
	if err != nil {
		s.t.Fatalf("cannot build the PATCH request: %v", err)
	}
	request.Header.Set("Content-Type", "application/octet-stream")

	if badRange {
		// A range that contradicts both the offset and the body length.
		request.Header.Set("Content-Range", fmt.Sprintf("%d-%d", s.offset+7, s.offset+3))
	} else {
		request.Header.Set("Content-Range", fmt.Sprintf("%d-%d", s.offset, s.offset+len(chunk)))
	}

	response := s.do(request, "PATCH upload")
	defer response.Body.Close()

	if s.deleted && response.StatusCode/100 == 2 {
		s.t.Fatalf("PATCH on a deleted upload session returned %s", response.Status)
	}

	if response.StatusCode == http.StatusAccepted {
		s.offset += len(chunk)
		if location := response.Header.Get("Location"); location != "" {
			s.location = location
		}
	}
}

func (s *uploadSession) put(digestParam string) {
	if s.location == "" {
		return
	}

	url := s.location
	if digestParam != "" {
		separator := "?"
		if bytes.ContainsRune([]byte(url), '?') {
			separator = "&"
		}
		url += separator + "digest=" + digestParam
	}

	// Whatever the program has not appended yet goes in the final request, so a
	// correct program finalises exactly s.content.
	remaining := s.content[min(s.offset, len(s.content)):]

	request, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(remaining))
	if err != nil {
		s.t.Fatalf("cannot build the PUT request: %v", err)
	}
	request.Header.Set("Content-Type", "application/octet-stream")

	response := s.do(request, "PUT upload")
	defer response.Body.Close()

	if s.deleted && response.StatusCode/100 == 2 {
		s.t.Fatalf("PUT on a deleted upload session returned %s", response.Status)
	}

	if response.StatusCode != http.StatusCreated {
		return
	}

	// The registry says a blob now exists. It may only exist under a digest that
	// describes the bytes the session actually carried.
	uploaded := s.content[:min(s.offset, len(s.content))]
	uploaded = append(append([]byte{}, uploaded...), remaining...)

	if digestParam == "" {
		s.t.Fatalf("PUT without a digest parameter created a blob: the registry cannot "+
			"know what to address it by (repository %s)", s.repository)
	}
	if want := digest.FromBytes(uploaded).String(); digestParam != want {
		s.t.Fatalf("PUT created a blob under %s, but the session carried %d bytes whose digest is %s",
			digestParam, len(uploaded), want)
	}

	s.recordCreated(digestParam, uploaded)
	s.location = ""
	s.offset = 0
}

func (s *uploadSession) recordCreated(digestParam string, from []byte) {
	s.created = digest.Digest(digestParam)
	s.createdFrom = append([]byte{}, from...)
}

func (s *uploadSession) status() {
	if s.location == "" {
		return
	}

	request, err := http.NewRequest(http.MethodGet, s.location, nil)
	if err != nil {
		s.t.Fatalf("cannot build the status request: %v", err)
	}

	response := s.do(request, "GET upload status")
	response.Body.Close()
}

func (s *uploadSession) del() {
	if s.location == "" {
		return
	}

	request, err := http.NewRequest(http.MethodDelete, s.location, nil)
	if err != nil {
		s.t.Fatalf("cannot build the DELETE request: %v", err)
	}

	response := s.do(request, "DELETE upload")
	response.Body.Close()

	if response.StatusCode/100 == 2 {
		s.deleted = true
	}
}

// verifyBlob reads back whatever the registry reported as created and checks it
// is byte-for-byte what the session uploaded.
func (s *uploadSession) verifyBlob() {
	if s.created == "" {
		return
	}

	url := s.env.URL + "/v2/" + s.repository + "/blobs/" + s.created.String()
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		s.t.Fatalf("cannot build the blob request: %v", err)
	}

	response := s.do(request, "GET blob")
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		s.t.Fatalf("a blob the registry reported as created reads back as %s", response.Status)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		s.t.Fatalf("cannot read the blob back: %v", err)
	}

	if !bytes.Equal(body, s.createdFrom) {
		s.t.Fatalf("the blob addressed by %s reads back as %d bytes, %d were uploaded",
			s.created, len(body), len(s.createdFrom))
	}
	if got := digest.FromBytes(body); got != s.created {
		s.t.Fatalf("the blob stored under %s has digest %s", s.created, got)
	}
}

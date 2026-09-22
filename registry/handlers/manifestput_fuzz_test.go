package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/docker/distribution"
	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema2"
	_ "github.com/docker/distribution/registry/storage/driver/inmemory"
)

// FuzzManifestPut drives a manifest through the real PUT handler.
//
// Threat model coverage (registry-threat-model.md), harness 3: a manifest is the
// document that decides which bytes a node executes. Parsing it is only half of
// the check -- the parser in manifests.go will happily produce a manifest whose
// config or layer descriptor is a zero value, because rejecting that is the job
// of verifyManifest in registry/storage, which resolves every reference against
// the blob store. This harness drives that layer, so the assertion belongs here
// rather than in FuzzUnmarshalManifest.
//
// The invariant: if the registry accepts a manifest, every blob it names must
// already exist under exactly that digest. A manifest accepted with a dangling
// or empty reference is a tag that resolves to content the registry cannot
// produce, and on a node that is a pull that fails after the pod has been
// scheduled -- or, if the dangling digest is later filled by a different
// client, content substituted under a name that was already trusted.
func FuzzManifestPut(f *testing.F) {
	repo := manifestFuzzRepo(f)

	valid := func(configDigest string, configSize int64, layerDigest string, layerSize int64) []byte {
		return []byte(fmt.Sprintf(`{
  "schemaVersion": 2,
  "mediaType": %q,
  "config": {"mediaType": %q, "size": %d, "digest": %q},
  "layers": [{"mediaType": %q, "size": %d, "digest": %q}]
}`, schema2.MediaTypeManifest, schema2.MediaTypeImageConfig, configSize, configDigest,
			schema2.MediaTypeLayer, layerSize, layerDigest))
	}

	// A manifest that names exactly the blobs the shared repository holds.
	f.Add(valid(repo.configDigest.String(), repo.configSize,
		repo.layerDigest.String(), repo.layerSize), schema2.MediaTypeManifest)

	// The same, with a size that disagrees with the stored blob.
	f.Add(valid(repo.configDigest.String(), repo.configSize+1,
		repo.layerDigest.String(), repo.layerSize), schema2.MediaTypeManifest)

	// A digest of content that was never uploaded.
	f.Add(valid(digest.FromString("never uploaded").String(), 12,
		repo.layerDigest.String(), repo.layerSize), schema2.MediaTypeManifest)

	// No config at all: the parser fills m.Config with a zero descriptor and
	// References() reports an empty digest.
	f.Add([]byte(`{"schemaVersion": 2, "mediaType": "`+schema2.MediaTypeManifest+`", "layers": []}`),
		schema2.MediaTypeManifest)

	// The case-insensitive field match that reaches the parser with nothing set.
	f.Add([]byte(`{"mediATYpe": "`+schema2.MediaTypeManifest+`"}`), schema2.MediaTypeManifest)

	f.Add([]byte(`{"schemaVersion": 2, "config": {"digest": ""}, "layers": [{"digest": ""}]}`),
		schema2.MediaTypeManifest)
	f.Add([]byte(`{"schemaVersion": 1}`), schema2.MediaTypeManifest)
	f.Add([]byte(`{"schemaVersion": 2, "manifests": []}`), "application/vnd.docker.distribution.manifest.list.v2+json")
	f.Add([]byte(`{}`), "")
	f.Add([]byte(`{`), schema2.MediaTypeManifest)
	f.Add([]byte(``), schema2.MediaTypeManifest)
	f.Add(valid(repo.configDigest.String(), repo.configSize,
		repo.layerDigest.String(), repo.layerSize), "application/octet-stream")
	f.Add(valid(repo.configDigest.String(), repo.configSize,
		repo.layerDigest.String(), repo.layerSize), schema2.MediaTypeManifest+"; charset=utf-8")

	f.Fuzz(func(t *testing.T, body []byte, contentType string) {
		if len(body) > 1<<16 || len(contentType) > 256 {
			return
		}
		if !sendableHeaderValue(contentType) {
			// net/http refuses to transmit it, and the server side of net/http
			// would reject it before any registry code ran, so there is nothing
			// here for this harness to reach.
			return
		}

		tag := fmt.Sprintf("fuzz%d", atomic.AddUint64(&manifestFuzzTag, 1))
		url := repo.server.URL + "/v2/" + repo.name + "/manifests/" + tag

		request, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("cannot build the PUT request: %v", err)
		}
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}

		response, _, ok := manifestDo(t, repo.server, request, "PUT manifest")
		if !ok || response.StatusCode != http.StatusCreated {
			return
		}

		// The registry accepted it. Everything it names must resolve.
		manifest, _, err := distribution.UnmarshalManifest(contentType, body)
		if err != nil {
			t.Fatalf("the registry accepted a manifest that does not parse as %q: %v\nbody: %s",
				contentType, err, body)
		}

		for i, reference := range manifest.References() {
			if reference.Digest == "" {
				t.Fatalf("the registry accepted a manifest whose reference[%d] has an empty digest; "+
					"the tag %s now names content that cannot be produced\nbody: %s", i, tag, body)
			}

			blobURL := repo.server.URL + "/v2/" + repo.name + "/blobs/" + reference.Digest.String()
			blobRequest, err := http.NewRequest(http.MethodHead, blobURL, nil)
			if err != nil {
				t.Fatalf("cannot build the blob request: %v", err)
			}

			blobResponse, _, ok := manifestDo(t, repo.server, blobRequest, "HEAD blob")
			if !ok {
				return
			}
			if blobResponse.StatusCode != http.StatusOK {
				t.Fatalf("the registry accepted a manifest referencing %s, which reads back as %s\nbody: %s",
					reference.Digest, blobResponse.Status, body)
			}
		}

		// A stored manifest must read back byte for byte: the digest a client
		// pins is computed over these bytes.
		getRequest, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("cannot build the GET request: %v", err)
		}
		// Name every type the registry can store, so that the read-back checks
		// storage and not content negotiation. Negotiation has its own test:
		// TestManifestGetAcceptIsCaseInsensitive.
		getRequest.Header.Set("Accept", canonicalAccept)

		getResponse, stored, ok := manifestDo(t, repo.server, getRequest, "GET manifest")
		if !ok {
			return
		}
		if getResponse.StatusCode != http.StatusOK {
			t.Fatalf("a manifest the registry reported as created reads back as %s", getResponse.Status)
		}

		_, canonicalDescriptor, err := manifest.Payload()
		if err != nil {
			t.Fatalf("cannot take the payload of an accepted manifest: %v", err)
		}
		if !bytes.Equal(stored, canonicalDescriptor) {
			t.Fatalf("the manifest stored under %s reads back as %d bytes, %d were sent",
				tag, len(stored), len(canonicalDescriptor))
		}
		if got := digest.FromBytes(stored); got != digest.FromBytes(body) {
			t.Fatalf("the manifest stored under %s has digest %s, the bytes sent digest to %s",
				tag, got, digest.FromBytes(body))
		}
	})
}

// canonicalAccept names every manifest type this registry can store.
var canonicalAccept = strings.Join([]string{
	schema2.MediaTypeManifest,
	manifestlist.MediaTypeManifestList,
	v1.MediaTypeImageManifest,
	v1.MediaTypeImageIndex,
}, ", ")

// TestManifestGetAcceptIsCaseInsensitive states a finding the fuzzer reached
// from a Content-Type that differed from the real one only in case.
//
// A media type is case-insensitive: RFC 9110 section 8.3.1 for Content-Type and
// RFC 2045 section 5.1 for the grammar both say so, and the registry itself
// relies on it when it parses Content-Type through mime.ParseMediaType, which
// lowercases. The Accept header is parsed by hand in manifests.go:109 and
// compared with ==, so the two directions disagree.
//
// The consequence is not a rejected request. A client whose Accept header names
// schema2 in a different case is treated as a client that does not understand
// schema2 at all, and the registry silently rewrites the stored manifest into
// schema1 -- a different document, under a different digest. For a manifest
// built from an image config without history, which is every image built by a
// modern tool, that rewrite then fails, and a valid stored manifest reads back
// as 400.
func TestManifestGetAcceptIsCaseInsensitive(t *testing.T) {
	repo := manifestRepoFor(t)

	body := []byte(fmt.Sprintf(`{
  "schemaVersion": 2,
  "mediaType": %q,
  "config": {"mediaType": %q, "size": %d, "digest": %q},
  "layers": [{"mediaType": %q, "size": %d, "digest": %q}]
}`, schema2.MediaTypeManifest, schema2.MediaTypeImageConfig, repo.configSize, repo.configDigest,
		schema2.MediaTypeLayer, repo.layerSize, repo.layerDigest))

	tag := fmt.Sprintf("accept%d", atomic.AddUint64(&manifestFuzzTag, 1))
	url := repo.server.URL + "/v2/" + repo.name + "/manifests/" + tag

	put, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("cannot build the PUT request: %v", err)
	}
	put.Header.Set("Content-Type", schema2.MediaTypeManifest)

	response, responseBody, _ := manifestDo(t, repo.server, put, "PUT manifest")
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("cannot store the manifest: %s: %s", response.Status, responseBody)
	}

	get := func(accept string) (*http.Response, []byte) {
		request, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("cannot build the GET request: %v", err)
		}
		request.Header.Set("Accept", accept)

		response, stored, _ := manifestDo(t, repo.server, request, "GET manifest")
		return response, stored
	}

	exact, wantBytes := get(schema2.MediaTypeManifest)
	if exact.StatusCode != http.StatusOK {
		t.Fatalf("the exactly-cased Accept header reads the manifest back as %s", exact.Status)
	}

	// One letter, in the subtype only, so the value stays a well-formed media
	// type that names the same thing.
	varied := strings.Replace(schema2.MediaTypeManifest, "distribution", "distriBution", 1)

	cased, gotBytes := get(varied)
	if cased.StatusCode != exact.StatusCode || !bytes.Equal(gotBytes, wantBytes) {
		t.Errorf("Accept: %q reads the manifest back as %s (%d bytes), while Accept: %q reads it as %s "+
			"(%d bytes); a media type is case-insensitive, so both must serve the same document",
			varied, cased.Status, len(gotBytes), schema2.MediaTypeManifest, exact.Status, len(wantBytes))
	}
}

// manifestRepoFor is manifestFuzzRepo for a test rather than a fuzz target.
func manifestRepoFor(t *testing.T) *manifestRepo {
	t.Helper()

	startManifestRepo(func(format string, args ...interface{}) {
		t.Fatalf(format, args...)
	})

	if manifestFuzzRepoValue == nil {
		t.Fatal("the fuzz registry did not start")
	}
	return manifestFuzzRepoValue
}

// sendableHeaderValue reports whether net/http will put the value on the wire.
// It mirrors the rule in net/http: no control bytes other than horizontal tab.
func sendableHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		if c := value[i]; (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}

// manifestRepo is a repository pre-loaded with the blobs a valid manifest can
// name, shared across iterations so that an iteration is a single PUT.
type manifestRepo struct {
	server       *httptest.Server
	name         string
	configDigest digest.Digest
	configSize   int64
	layerDigest  digest.Digest
	layerSize    int64
}

var (
	manifestFuzzOnce      sync.Once
	manifestFuzzRepoValue *manifestRepo
	manifestFuzzTag       uint64
)

func manifestFuzzRepo(f *testing.F) *manifestRepo {
	f.Helper()

	startManifestRepo(func(format string, args ...interface{}) {
		f.Fatalf(format, args...)
	})

	if manifestFuzzRepoValue == nil {
		f.Fatal("the fuzz registry did not start")
	}
	return manifestFuzzRepoValue
}

// startManifestRepo brings up the shared registry once per process.
func startManifestRepo(fatalf func(string, ...interface{})) {
	manifestFuzzOnce.Do(func() {
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
		server := httptest.NewServer(app)

		repo := &manifestRepo{server: server, name: "fuzz/manifests"}
		configBlob := []byte(`{"architecture":"amd64","os":"linux","rootfs":{"type":"layers","diff_ids":[]}}`)
		layerBlob := bytes.Repeat([]byte("layer"), 64)

		repo.configDigest, repo.configSize = putBlob(fatalf, repo, configBlob)
		repo.layerDigest, repo.layerSize = putBlob(fatalf, repo, layerBlob)

		manifestFuzzRepoValue = repo
	})
}

// putBlob stores a blob.
//
// It takes two requests because StartBlobUpload in this registry does not
// implement the single-request monolithic upload: a POST carrying ?digest= and
// a body always answers 202 with a fresh session and does not consume the body.
func putBlob(fatalf func(string, ...interface{}), repo *manifestRepo, content []byte) (digest.Digest, int64) {
	blobDigest := digest.FromBytes(content)

	post, err := http.NewRequest(http.MethodPost,
		repo.server.URL+"/v2/"+repo.name+"/blobs/uploads/", nil)
	if err != nil {
		fatalf("cannot build the upload session request: %v", err)
	}

	response, body := setupDo(fatalf, repo.server, post)
	if response.StatusCode != http.StatusAccepted {
		fatalf("cannot open an upload session: %s: %s", response.Status, body)
	}

	location := response.Header.Get("Location")
	if location == "" {
		fatalf("the upload session carries no Location header")
	}

	separator := "?"
	if strings.ContainsRune(location, '?') {
		separator = "&"
	}

	put, err := http.NewRequest(http.MethodPut, location+separator+"digest="+blobDigest.String(),
		bytes.NewReader(content))
	if err != nil {
		fatalf("cannot build the blob request: %v", err)
	}
	put.Header.Set("Content-Type", "application/octet-stream")

	response, body = setupDo(fatalf, repo.server, put)
	if response.StatusCode != http.StatusCreated {
		fatalf("cannot store the blob: %s: %s", response.Status, body)
	}

	return blobDigest, int64(len(content))
}

// fuzzDo performs a setup request, draining the body so the connection is reused.
func setupDo(fatalf func(string, ...interface{}), server *httptest.Server, request *http.Request) (*http.Response, []byte) {
	response, err := server.Client().Do(request)
	if err != nil {
		fatalf("the setup request failed: %v", err)
		return nil, nil
	}

	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		fatalf("cannot read the setup response: %v", err)
		return nil, nil
	}

	return response, body
}

// manifestDo performs the request, drains the body so the connection can be
// reused, and enforces the no-5xx invariant.
func manifestDo(t *testing.T, server *httptest.Server, request *http.Request, what string) (*http.Response, []byte, bool) {
	t.Helper()

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("%s failed at the transport level: %v", what, err)
	}

	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatalf("%s: cannot read the response body: %v", what, readErr)
	}

	if response.StatusCode >= 500 {
		if allowKnown5xx {
			// Abandon the iteration: the state after an internal error is not
			// something the oracle can reason about.
			return response, body, false
		}
		t.Fatalf("%s produced %s, which a client must not be able to cause: %s",
			what, response.Status, truncate(body))
	}

	return response, body, true
}

// allowKnown5xx downgrades the no-5xx invariant to an early return.
//
// It exists because the invariant already holds a finding: a manifest whose
// schemaVersion is absent or not 2 reaches verifyManifest in registry/storage,
// whose schema-version branch returns a bare fmt.Errorf while every other
// branch accumulates distribution.ErrManifestVerification. Only the accumulated
// form is mapped to 400 MANIFEST_INVALID, so the bare error falls through to
// 500 UNKNOWN -- see schema2manifesthandler.go:75 and ocimanifesthandler.go:69.
// A default `go test` run reports it from seed#4.
//
// Set FUZZ_ALLOW_KNOWN_5XX=1 to look past it for further defects, which is what
// the long fuzzing runs do. Unset, the harness reports the finding.
var allowKnown5xx = os.Getenv("FUZZ_ALLOW_KNOWN_5XX") != ""

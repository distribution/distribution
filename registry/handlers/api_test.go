package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/manifest/ocischema"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	hookstest "github.com/sirupsen/logrus/hooks/test"
)

var (
	headerConfig = http.Header{
		"X-Content-Type-Options": []string{"nosniff"},
	}
	emptyJsonDescriptor = distribution.Descriptor{
		MediaType: v1.DescriptorEmptyJSON.MediaType,
		Size:      v1.DescriptorEmptyJSON.Size,
		Digest:    v1.DescriptorEmptyJSON.Digest,
	}
)

const (
	// digestSha256EmptyTar is the canonical sha256 digest of empty data
	digestSha256EmptyTar = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// TestCheckAPI hits the base endpoint (/v2/) ensures we return the specified
// 200 OK response.
func TestCheckAPI(t *testing.T) {
	env := newTestEnv(t, false)
	defer env.Shutdown()
	baseURL, err := env.builder.BuildBaseURL()
	if err != nil {
		t.Fatalf("unexpected error building base url: %v", err)
	}

	resp, err := http.Get(baseURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing api base check", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{"2"},
	})

	p, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unexpected error reading response body: %v", err)
	}

	if string(p) != "{}" {
		t.Fatalf("unexpected response body: %v", string(p))
	}
}

// TestCatalogAPI tests the /v2/_catalog endpoint
func TestCatalogAPI(t *testing.T) {
	env := newTestEnv(t, false)
	defer env.Shutdown()

	maxEntries := env.config.Catalog.MaxEntries
	allCatalog := []string{
		"foo/aaaa", "foo/bbbb", "foo/cccc", "foo/dddd", "foo/eeee", "foo/ffff",
	}

	chunkLen := maxEntries - 1

	catalogURL, err := env.builder.BuildCatalogURL()
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	// -----------------------------------
	// Case No. 1: Empty catalog
	resp, err := http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	var ctlg struct {
		Repositories []string `json:"repositories"`
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	// No images pushed = no image returned
	if len(ctlg.Repositories) != 0 {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", 0, len(ctlg.Repositories))
	}

	// No pagination should be returned
	if resp.Header.Get("Link") != "" {
		t.Fatal("repositories has more data when none expected")
	}

	for _, image := range allCatalog {
		createRepository(env, t, image, "sometag")
	}

	// -----------------------------------
	// Case No. 2: Catalog populated & n is not provided nil (n internally will be min(100, maxEntries))

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	// it must match max entries
	if len(ctlg.Repositories) != maxEntries {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", maxEntries, len(ctlg.Repositories))
	}

	// it must return the first maxEntries entries from the catalog
	for _, image := range allCatalog[:maxEntries] {
		if !contains(ctlg.Repositories, image) {
			t.Fatalf("didn't find our repository '%s' in the catalog", image)
		}
	}

	// fail if there's no pagination
	link := resp.Header.Get("Link")
	if link == "" {
		t.Fatal("repositories has less data than expected")
	}
	// -----------------------------------
	// Case No. 2.1: Second page (n internally will be min(100, maxEntries))

	// build pagination link
	values := checkLink(t, link, maxEntries, ctlg.Repositories[len(ctlg.Repositories)-1])

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	expectedRemainder := len(allCatalog) - maxEntries

	if len(ctlg.Repositories) != expectedRemainder {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", expectedRemainder, len(ctlg.Repositories))
	}

	// -----------------------------------
	// Case No. 3: request n = maxentries
	values = url.Values{
		"last": []string{""},
		"n":    []string{strconv.Itoa(maxEntries)},
	}

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	if len(ctlg.Repositories) != maxEntries {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", maxEntries, len(ctlg.Repositories))
	}

	// fail if there's no pagination
	link = resp.Header.Get("Link")
	if link == "" {
		t.Fatal("repositories has less data than expected")
	}

	// -----------------------------------
	// Case No. 3.1: Second (last) page

	// build pagination link
	values = checkLink(t, link, maxEntries, ctlg.Repositories[len(ctlg.Repositories)-1])

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	expectedRemainder = len(allCatalog) - maxEntries

	if len(ctlg.Repositories) != expectedRemainder {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", expectedRemainder, len(ctlg.Repositories))
	}

	// -----------------------------------
	// Case No. 4: request n < maxentries

	values = url.Values{
		"n": []string{strconv.Itoa(chunkLen)},
	}

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	// returns the requested amount
	if len(ctlg.Repositories) != chunkLen {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", expectedRemainder, len(ctlg.Repositories))
	}

	// fail if there's no pagination
	link = resp.Header.Get("Link")
	if link == "" {
		t.Fatal("repositories has less data than expected")
	}

	// -----------------------------------
	// Case No. 4.1: request n < maxentries (second page)

	// build pagination link
	values = checkLink(t, link, chunkLen, ctlg.Repositories[len(ctlg.Repositories)-1])

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	expectedRemainder = len(allCatalog) - chunkLen

	if len(ctlg.Repositories) != expectedRemainder {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", expectedRemainder, len(ctlg.Repositories))
	}

	// -----------------------------------
	// Case No. 5: request n > maxentries

	values = url.Values{
		"n": []string{strconv.Itoa(maxEntries + 10)},
	}

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusBadRequest)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, "invalid number of results requested", resp, errcode.ErrorCodePaginationNumberInvalid)

	// -----------------------------------
	// Case No. 6: request n > maxentries but <= total catalog

	values = url.Values{
		"n": []string{strconv.Itoa(len(allCatalog))},
	}

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusBadRequest)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, "invalid number of results requested", resp, errcode.ErrorCodePaginationNumberInvalid)

	// -----------------------------------
	// Case No. 7: n = 0
	values = url.Values{
		"n": []string{"0"},
	}

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	// it must match max entries
	if len(ctlg.Repositories) != 0 {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", 0, len(ctlg.Repositories))
	}

	// -----------------------------------
	// Case No. 8: n = -1
	values = url.Values{
		"n": []string{"-1"},
	}

	catalogURL, err = env.builder.BuildCatalogURL(values)
	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusBadRequest)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, "invalid number of results requested", resp, errcode.ErrorCodePaginationNumberInvalid)

	// -----------------------------------
	// Case No. 9: n = 5, max = 5, total catalog = 4
	values = url.Values{
		"n": []string{strconv.Itoa(maxEntries)},
	}

	envWithLessImages := newTestEnv(t, false)
	for _, image := range allCatalog[0:(maxEntries - 1)] {
		createRepository(envWithLessImages, t, image, "sometag")
	}

	catalogURL, err = envWithLessImages.builder.BuildCatalogURL(values)

	if err != nil {
		t.Fatalf("unexpected error building catalog url: %v", err)
	}

	resp, err = http.Get(catalogURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing catalog api check", resp, http.StatusOK)

	dec = json.NewDecoder(resp.Body)
	if err = dec.Decode(&ctlg); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	// it must match max entries
	if len(ctlg.Repositories) != maxEntries-1 {
		t.Fatalf("repositories returned unexpected entries (expected: %d, returned: %d)", maxEntries-1, len(ctlg.Repositories))
	}
}

// TestTagsAPI tests the /v2/<name>/tags/list endpoint
func TestTagsAPI(t *testing.T) {
	env := newTestEnv(t, false)
	defer env.Shutdown()

	imageName, err := reference.WithName("test")
	if err != nil {
		t.Fatalf("unable to parse reference: %v", err)
	}

	tags := []string{
		"2j2ar",
		"asj9e",
		"jyi7b",
		"kb0j5",
		"sb71y",
	}

	for _, tag := range tags {
		createRepository(env, t, imageName.Name(), tag)
	}

	tt := []struct {
		name               string
		queryParams        url.Values
		expectedStatusCode int
		expectedBody       tagsAPIResponse
		expectedBodyErr    *errcode.ErrorCode
		expectedLinkHeader string
	}{
		{
			name:               "no query parameters",
			expectedStatusCode: http.StatusOK,
			expectedBody:       tagsAPIResponse{Name: imageName.Name(), Tags: tags},
		},
		{
			name:               "empty last query parameter",
			queryParams:        url.Values{"last": []string{""}},
			expectedStatusCode: http.StatusOK,
			expectedBody:       tagsAPIResponse{Name: imageName.Name(), Tags: tags},
		},
		{
			name:               "empty n query parameter",
			queryParams:        url.Values{"n": []string{""}},
			expectedStatusCode: http.StatusOK,
			expectedBody:       tagsAPIResponse{Name: imageName.Name(), Tags: tags},
		},
		{
			name:               "empty last and n query parameters",
			queryParams:        url.Values{"last": []string{""}, "n": []string{""}},
			expectedStatusCode: http.StatusOK,
			expectedBody:       tagsAPIResponse{Name: imageName.Name(), Tags: tags},
		},
		{
			name:               "negative n query parameter",
			queryParams:        url.Values{"n": []string{"-1"}},
			expectedStatusCode: http.StatusBadRequest,
			expectedBodyErr:    &errcode.ErrorCodePaginationNumberInvalid,
		},
		{
			name:               "non integer n query parameter",
			queryParams:        url.Values{"n": []string{"foo"}},
			expectedStatusCode: http.StatusBadRequest,
			expectedBodyErr:    &errcode.ErrorCodePaginationNumberInvalid,
		},
		{
			name:               "1st page",
			queryParams:        url.Values{"n": []string{"2"}},
			expectedStatusCode: http.StatusOK,
			expectedBody: tagsAPIResponse{Name: imageName.Name(), Tags: []string{
				"2j2ar",
				"asj9e",
			}},
			expectedLinkHeader: `</v2/test/tags/list?last=asj9e&n=2>; rel="next"`,
		},
		{
			name:               "nth page",
			queryParams:        url.Values{"last": []string{"asj9e"}, "n": []string{"1"}},
			expectedStatusCode: http.StatusOK,
			expectedBody: tagsAPIResponse{Name: imageName.Name(), Tags: []string{
				"jyi7b",
			}},
			expectedLinkHeader: `</v2/test/tags/list?last=jyi7b&n=1>; rel="next"`,
		},
		{
			name:               "last page",
			queryParams:        url.Values{"last": []string{"jyi7b"}, "n": []string{"3"}},
			expectedStatusCode: http.StatusOK,
			expectedBody: tagsAPIResponse{Name: imageName.Name(), Tags: []string{
				"kb0j5",
				"sb71y",
			}},
		},
		{
			name:               "page size bigger than full list",
			queryParams:        url.Values{"n": []string{"100"}},
			expectedStatusCode: http.StatusOK,
			expectedBody:       tagsAPIResponse{Name: imageName.Name(), Tags: tags},
		},
		{
			name:               "after marker",
			queryParams:        url.Values{"last": []string{"jyi7b"}},
			expectedStatusCode: http.StatusOK,
			expectedBody: tagsAPIResponse{Name: imageName.Name(), Tags: []string{
				"kb0j5",
				"sb71y",
			}},
		},
		{
			name:               "after non existent marker",
			queryParams:        url.Values{"last": []string{"does-not-exist"}, "n": []string{"3"}},
			expectedStatusCode: http.StatusOK,
			expectedBody: tagsAPIResponse{Name: imageName.Name(), Tags: []string{
				"kb0j5",
				"sb71y",
			}},
		},
	}

	for _, test := range tt {
		t.Run(test.name, func(t *testing.T) {
			tagsURL, err := env.builder.BuildTagsURL(imageName, test.queryParams)
			if err != nil {
				t.Fatalf("unexpected error building tags URL: %v", err)
			}

			resp, err := http.Get(tagsURL)
			if err != nil {
				t.Fatalf("unexpected error issuing request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != test.expectedStatusCode {
				t.Fatalf("expected response status code to be %d, got %d", test.expectedStatusCode, resp.StatusCode)
			}

			if test.expectedBodyErr != nil {
				// nolint:errcheck
				checkBodyHasErrorCodes(t, "invalid number of results requested", resp, *test.expectedBodyErr)
			} else {
				var body tagsAPIResponse
				dec := json.NewDecoder(resp.Body)
				if err := dec.Decode(&body); err != nil {
					t.Fatalf("unexpected error decoding response body: %v", err)
				}
				if !reflect.DeepEqual(body, test.expectedBody) {
					t.Fatalf("expected response body to be:\n%+v\ngot:\n%+v", test.expectedBody, body)
				}
			}

			if resp.Header.Get("Link") != test.expectedLinkHeader {
				t.Fatalf("expected response Link header to be %q, got %q", test.expectedLinkHeader, resp.Header.Get("Link"))
			}
		})
	}
}

func checkLink(t *testing.T, urlStr string, numEntries int, last string) url.Values {
	re := regexp.MustCompile("<(/v2/_catalog.*)>; rel=\"next\"")
	matches := re.FindStringSubmatch(urlStr)

	if len(matches) != 2 {
		t.Fatal("Catalog link address response was incorrect")
	}
	linkURL, _ := url.Parse(matches[1])
	urlValues := linkURL.Query()

	if urlValues.Get("n") != strconv.Itoa(numEntries) {
		t.Fatalf("Catalog link entry size is incorrect (expected: %v, returned: %v)", urlValues.Get("n"), strconv.Itoa(numEntries))
	}

	if urlValues.Get("last") != last {
		t.Fatal("Catalog link last entry is incorrect")
	}

	return urlValues
}

func contains(elems []string, e string) bool {
	return slices.Contains(elems, e)
}

func TestURLPrefix(t *testing.T) {
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
	}
	config.HTTP.Prefix = "/test/"
	config.HTTP.Headers = headerConfig

	env := newTestEnvWithConfig(t, &config)
	defer env.Shutdown()

	baseURL, err := env.builder.BuildBaseURL()
	if err != nil {
		t.Fatalf("unexpected error building base url: %v", err)
	}

	parsed, _ := url.Parse(baseURL)
	if !strings.HasPrefix(parsed.Path, config.HTTP.Prefix) {
		t.Fatalf("Prefix %v not included in test url %v", config.HTTP.Prefix, baseURL)
	}

	resp, err := http.Get(baseURL)
	if err != nil {
		t.Fatalf("unexpected error issuing request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "issuing api base check", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Type":   []string{"application/json"},
		"Content-Length": []string{"2"},
	})
}

type blobArgs struct {
	imageName   reference.Named
	layerFile   io.ReadSeeker
	layerDigest digest.Digest
}

func makeBlobArgs(t *testing.T) blobArgs {
	layerFile, layerDigest, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random layer file: %v", err)
	}

	args := blobArgs{
		layerFile:   layerFile,
		layerDigest: layerDigest,
	}
	args.imageName, _ = reference.WithName("foo/bar")
	return args
}

// TestBlobAPI conducts a full test of the of the blob api.
func TestBlobAPI(t *testing.T) {
	deleteEnabled := false
	env1 := newTestEnv(t, deleteEnabled)
	defer env1.Shutdown()
	args := makeBlobArgs(t)
	testBlobAPI(t, env1, args)

	deleteEnabled = true
	env2 := newTestEnv(t, deleteEnabled)
	defer env2.Shutdown()
	args = makeBlobArgs(t)
	testBlobAPI(t, env2, args)
}

func TestBlobDelete(t *testing.T) {
	deleteEnabled := true
	env := newTestEnv(t, deleteEnabled)
	defer env.Shutdown()

	args := makeBlobArgs(t)
	env = testBlobAPI(t, env, args)
	testBlobDelete(t, env, args)
}

func TestRelativeURL(t *testing.T) {
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
	}
	config.HTTP.Headers = headerConfig
	config.HTTP.RelativeURLs = false
	env := newTestEnvWithConfig(t, &config)
	defer env.Shutdown()
	ref, _ := reference.WithName("foo/bar")
	uploadURLBaseAbs, _ := startPushLayer(t, env, ref)

	u, err := url.Parse(uploadURLBaseAbs)
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAbs() {
		t.Fatal("Relative URL returned from blob upload chunk with non-relative configuration")
	}

	args := makeBlobArgs(t)
	resp, err := doPushLayer(t, env.builder, ref, args.layerDigest, uploadURLBaseAbs, args.layerFile)
	if err != nil {
		t.Fatalf("unexpected error doing layer push relative url: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "relativeurl blob upload", resp, http.StatusCreated)
	u, err = url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAbs() {
		t.Fatal("Relative URL returned from blob upload with non-relative configuration")
	}

	config.HTTP.RelativeURLs = true
	args = makeBlobArgs(t)
	uploadURLBaseRelative, _ := startPushLayer(t, env, ref)
	u, err = url.Parse(uploadURLBaseRelative)
	if err != nil {
		t.Fatal(err)
	}
	if u.IsAbs() {
		t.Fatal("Absolute URL returned from blob upload chunk with relative configuration")
	}

	// Start a new upload in absolute mode to get a valid base URL
	config.HTTP.RelativeURLs = false
	uploadURLBaseAbs, _ = startPushLayer(t, env, ref)
	u, err = url.Parse(uploadURLBaseAbs)
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAbs() {
		t.Fatal("Relative URL returned from blob upload chunk with non-relative configuration")
	}

	// Complete upload with relative URLs enabled to ensure the final location is relative
	config.HTTP.RelativeURLs = true
	resp, err = doPushLayer(t, env.builder, ref, args.layerDigest, uploadURLBaseAbs, args.layerFile)
	if err != nil {
		t.Fatalf("unexpected error doing layer push relative url: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "relativeurl blob upload", resp, http.StatusCreated)
	u, err = url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if u.IsAbs() {
		t.Fatal("Relative URL returned from blob upload with non-relative configuration")
	}
}

func TestBlobDeleteDisabled(t *testing.T) {
	deleteEnabled := false
	env := newTestEnv(t, deleteEnabled)
	defer env.Shutdown()
	args := makeBlobArgs(t)

	imageName := args.imageName
	layerDigest := args.layerDigest
	ref, _ := reference.WithDigest(imageName, layerDigest)
	layerURL, err := env.builder.BuildBlobURL(ref)
	if err != nil {
		t.Fatalf("error building url: %v", err)
	}

	resp, err := httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting when disabled: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "status of disabled delete", resp, http.StatusMethodNotAllowed)
}

func testBlobAPI(t *testing.T, env *testEnv, args blobArgs) *testEnv {
	// TODO(stevvooe): This test code is complete junk but it should cover the
	// complete flow. This must be broken down and checked against the
	// specification *before* we submit the final to docker core.
	imageName := args.imageName
	layerFile := args.layerFile
	layerDigest := args.layerDigest

	// -----------------------------------
	// Test fetch for non-existent content
	ref, _ := reference.WithDigest(imageName, layerDigest)
	layerURL, err := env.builder.BuildBlobURL(ref)
	if err != nil {
		t.Fatalf("error building url: %v", err)
	}

	resp, err := http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching non-existent layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching non-existent content", resp, http.StatusNotFound)

	// ------------------------------------------
	// Test head request for non-existent content
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on non-existent layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "checking head on non-existent layer", resp, http.StatusNotFound)

	// ------------------------------------------
	// Start an upload, check the status then cancel
	uploadURLBase, uploadUUID := startPushLayer(t, env, imageName)

	// A status check should work
	resp, err = http.Get(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error getting upload status: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "status of deleted upload", resp, http.StatusNoContent)
	checkHeaders(t, resp, http.Header{
		"Location":           []string{"*"},
		"Range":              []string{"0-0"},
		"Docker-Upload-UUID": []string{uploadUUID},
	})

	req, err := http.NewRequest(http.MethodDelete, uploadURLBase, nil)
	if err != nil {
		t.Fatalf("unexpected error creating delete request: %v", err)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error sending delete request: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "deleting upload", resp, http.StatusNoContent)

	// A status check should result in 404
	resp, err = http.Get(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error getting upload status: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "status of deleted upload", resp, http.StatusNotFound)

	// -----------------------------------------
	// Do layer push with an empty body and different digest
	uploadURLBase, _ = startPushLayer(t, env, imageName)
	resp, err = doPushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("unexpected error doing bad layer push: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "bad layer push", resp, http.StatusBadRequest)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, "bad layer push", resp, errcode.ErrorCodeDigestInvalid)

	// -----------------------------------------
	// Do layer push with an empty body and correct digest
	zeroDigest, err := digest.FromReader(bytes.NewReader([]byte{}))
	if err != nil {
		t.Fatalf("unexpected error digesting empty buffer: %v", err)
	}

	uploadURLBase, _ = startPushLayer(t, env, imageName)
	pushLayer(t, env.builder, imageName, zeroDigest, uploadURLBase, bytes.NewReader([]byte{}))

	// -----------------------------------------
	// Do layer push with an empty body and correct digest

	// This is a valid but empty tarfile!
	emptyTar := bytes.Repeat([]byte("\x00"), 1024)
	emptyDigest, err := digest.FromReader(bytes.NewReader(emptyTar))
	if err != nil {
		t.Fatalf("unexpected error digesting empty tar: %v", err)
	}

	uploadURLBase, _ = startPushLayer(t, env, imageName)
	pushLayer(t, env.builder, imageName, emptyDigest, uploadURLBase, bytes.NewReader(emptyTar))

	// ------------------------------------------
	// Now, actually do successful upload.
	layerLength, err := layerFile.Seek(0, io.SeekEnd)
	if err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}
	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}

	uploadURLBase, _ = startPushLayer(t, env, imageName)
	pushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, layerFile)

	// ------------------------------------------
	// Now, push just a chunk
	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}

	canonicalDigester := digest.Canonical.Digester()
	if _, err := io.Copy(canonicalDigester.Hash(), layerFile); err != nil {
		t.Fatalf("error copying to digest: %v", err)
	}
	canonicalDigest := canonicalDigester.Digest()

	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}
	uploadURLBase, _ = startPushLayer(t, env, imageName)
	uploadURLBase, dgst := pushChunk(t, env.builder, imageName, uploadURLBase, layerFile, layerLength)

	// -----------------------------------------
	// Check the chunk upload status
	_, end, err := getUploadStatus(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error doing chunk upload check: %v", err)
	}
	if end+1 != layerLength {
		t.Fatalf("getting wrong chunk upload status: %d", end)
	}

	finishUpload(t, env.builder, imageName, uploadURLBase, dgst)

	// -----------------------------------------
	// Do layer push with invalid content range
	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}

	uploadURLBase, _ = startPushLayer(t, env, imageName)
	sizeInvalid := chunkOptions{
		contentRange: "0-20",
	}
	resp, err = doPushChunk(t, uploadURLBase, layerFile, sizeInvalid)
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "putting size invalid chunk", resp, http.StatusBadRequest)

	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}
	uploadURLBase, _ = startPushLayer(t, env, imageName)
	outOfOrder := chunkOptions{
		contentRange: "3-22",
	}
	resp, err = doPushChunk(t, uploadURLBase, layerFile, outOfOrder)
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "putting range out of order chunk", resp, http.StatusRequestedRangeNotSatisfiable)

	// ------------------------
	// Use a head request to see if the layer exists.
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on existing layer: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "checking head on existing layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
	})

	// ----------------
	// Fetch the layer!
	resp, err = http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "fetching layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
	})

	// Verify the body
	verifier := layerDigest.Verifier()
	if _, err := io.Copy(verifier, resp.Body); err != nil {
		t.Fatalf("unexpected error reading response body: %v", err)
	}

	if !verifier.Verified() {
		t.Fatal("response body did not pass verification")
	}

	// ----------------
	// Fetch the layer with an invalid digest
	badURL := strings.Replace(layerURL, "sha256", "sha257", 1)
	resp, err = http.Get(badURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "fetching layer bad digest", resp, http.StatusBadRequest)

	// Cache headers
	resp, err = http.Get(layerURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "fetching layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, canonicalDigest)},
		"Cache-Control":         []string{"max-age=31536000"},
	})

	// Matching etag, gives 304
	etag := resp.Header.Get("Etag")
	req, err = http.NewRequest(http.MethodGet, layerURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "fetching layer with etag", resp, http.StatusNotModified)

	// Non-matching etag, gives 200
	req, err = http.NewRequest(http.MethodGet, layerURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", "")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "fetching layer with invalid etag", resp, http.StatusOK)

	// Missing tests:
	// 	- Upload the same tar file under and different repository and
	//       ensure the content remains uncorrupted.
	return env
}

func testBlobDelete(t *testing.T, env *testEnv, args blobArgs) {
	// Upload a layer
	imageName := args.imageName
	layerFile := args.layerFile
	layerDigest := args.layerDigest

	ref, _ := reference.WithDigest(imageName, layerDigest)
	layerURL, err := env.builder.BuildBlobURL(ref)
	if err != nil {
		t.Fatal(err)
	}
	// ---------------
	// Delete a layer
	resp, err := httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "deleting layer", resp, http.StatusAccepted)
	checkHeaders(t, resp, http.Header{
		"Content-Length": []string{"0"},
	})

	// ---------------
	// Try and get it back
	// Use a head request to see if the layer exists.
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on existing layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "checking existence of deleted layer", resp, http.StatusNotFound)

	// Delete already deleted layer
	resp, err = httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "deleting layer", resp, http.StatusNotFound)

	// ----------------
	// Attempt to delete a layer with an invalid digest
	badURL := strings.Replace(layerURL, "sha256", "sha257", 1)
	resp, err = httpDelete(badURL)
	if err != nil {
		t.Fatalf("unexpected error fetching layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "deleting layer bad digest", resp, http.StatusBadRequest)

	// ----------------
	// Reupload previously deleted blob
	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}

	uploadURLBase, _ := startPushLayer(t, env, imageName)
	pushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, layerFile)

	if _, err := layerFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("unexpected error seeking layer: %v", err)
	}
	canonicalDigester := digest.Canonical.Digester()
	if _, err := io.Copy(canonicalDigester.Hash(), layerFile); err != nil {
		t.Fatalf("error copying to digest: %v", err)
	}
	canonicalDigest := canonicalDigester.Digest()

	// ------------------------
	// Use a head request to see if it exists
	resp, err = http.Head(layerURL)
	if err != nil {
		t.Fatalf("unexpected error checking head on existing layer: %v", err)
	}
	defer resp.Body.Close()

	layerLength, _ := layerFile.Seek(0, io.SeekEnd)
	checkResponse(t, "checking head on reuploaded layer", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Content-Length":        []string{fmt.Sprint(layerLength)},
		"Docker-Content-Digest": []string{canonicalDigest.String()},
	})
}

func TestDeleteDisabled(t *testing.T) {
	env := newTestEnv(t, false)
	defer env.Shutdown()

	imageName, _ := reference.WithName("foo/bar")
	// "build" our layer file
	layerFile, layerDigest, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random layer file: %v", err)
	}

	ref, _ := reference.WithDigest(imageName, layerDigest)
	layerURL, err := env.builder.BuildBlobURL(ref)
	if err != nil {
		t.Fatal("Error building blob URL")
	}
	uploadURLBase, _ := startPushLayer(t, env, imageName)
	pushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, layerFile)

	resp, err := httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "deleting layer with delete disabled", resp, http.StatusMethodNotAllowed)
}

func TestDeleteReadOnly(t *testing.T) {
	env := newTestEnv(t, true)
	defer env.Shutdown()

	imageName, _ := reference.WithName("foo/bar")
	// "build" our layer file
	layerFile, layerDigest, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random layer file: %v", err)
	}

	ref, _ := reference.WithDigest(imageName, layerDigest)
	layerURL, err := env.builder.BuildBlobURL(ref)
	if err != nil {
		t.Fatal("Error building blob URL")
	}
	uploadURLBase, _ := startPushLayer(t, env, imageName)
	pushLayer(t, env.builder, imageName, layerDigest, uploadURLBase, layerFile)

	env.app.readOnly = true

	resp, err := httpDelete(layerURL)
	if err != nil {
		t.Fatalf("unexpected error deleting layer: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "deleting layer in read-only mode", resp, http.StatusMethodNotAllowed)
}

func TestStartPushReadOnly(t *testing.T) {
	env := newTestEnv(t, true)
	defer env.Shutdown()
	env.app.readOnly = true

	imageName, _ := reference.WithName("foo/bar")

	layerUploadURL, err := env.builder.BuildBlobUploadURL(imageName)
	if err != nil {
		t.Fatalf("unexpected error building layer upload url: %v", err)
	}

	resp, err := http.Post(layerUploadURL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error starting layer push: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "starting push in read-only mode", resp, http.StatusMethodNotAllowed)
}

func httpDelete(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, err
}

type manifestArgs struct {
	imageName reference.Named
	mediaType string
	manifest  distribution.Manifest
	dgst      digest.Digest
}

func TestManifestAPI(t *testing.T) {
	schema2Repo, _ := reference.WithName("foo/schema2")

	deleteEnabled := false
	env1 := newTestEnv(t, deleteEnabled)
	defer env1.Shutdown()
	schema2Args := testManifestAPISchema2(t, env1, schema2Repo, "schema2tag")
	testManifestAPIManifestList(t, env1, schema2Args)

	deleteEnabled = true
	env2 := newTestEnv(t, deleteEnabled)
	defer env2.Shutdown()
	schema2Args = testManifestAPISchema2(t, env2, schema2Repo, "schema2tag")
	testManifestAPIManifestList(t, env2, schema2Args)
}

func TestManifestAPI_DeleteTag(t *testing.T) {
	env := newTestEnv(t, false)
	defer env.Shutdown()

	imageName, err := reference.WithName("foo/bar")
	checkErr(t, err, "building image name")

	tag := "latest"
	dgst := createRepository(env, t, imageName.Name(), tag)

	ref, err := reference.WithTag(imageName, tag)
	checkErr(t, err, "building tag reference")

	u, err := env.builder.BuildManifestURL(ref)
	checkErr(t, err, "building tag URL")

	resp, err := httpDelete(u)
	m := "deleting tag"
	checkErr(t, err, m)
	defer resp.Body.Close()

	checkResponse(t, m, resp, http.StatusAccepted)
	if resp.Body != http.NoBody {
		t.Fatal("unexpected response body")
	}

	msg := "checking tag no longer exists"
	resp, err = http.Get(u)
	checkErr(t, err, msg)
	defer resp.Body.Close()
	checkResponse(t, msg, resp, http.StatusNotFound)

	digestRef, err := reference.WithDigest(imageName, dgst)
	checkErr(t, err, "building manifest digest reference")

	u, err = env.builder.BuildManifestURL(digestRef)
	checkErr(t, err, "building manifest URL")

	msg = "checking manifest still exists"
	resp, err = http.Head(u)
	checkErr(t, err, msg)
	defer resp.Body.Close()
	checkResponse(t, msg, resp, http.StatusOK)
}

func TestManifestAPI_DeleteTag_Unknown(t *testing.T) {
	env := newTestEnv(t, false)
	defer env.Shutdown()

	imageName, err := reference.WithName("foo/bar")
	checkErr(t, err, "building named object")

	ref, err := reference.WithTag(imageName, "latest")
	checkErr(t, err, "building tag reference")

	u, err := env.builder.BuildManifestURL(ref)
	checkErr(t, err, "building tag URL")

	resp, err := httpDelete(u)
	msg := "deleting unknown tag"
	checkErr(t, err, msg)
	defer resp.Body.Close()

	checkResponse(t, msg, resp, http.StatusNotFound)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, msg, resp, errcode.ErrorCodeManifestUnknown)
}

func TestManifestAPI_DeleteTag_ReadOnly(t *testing.T) {
	env := newTestEnv(t, false)
	defer env.Shutdown()
	env.app.readOnly = true

	imageName, err := reference.WithName("foo/bar")
	checkErr(t, err, "building named object")

	ref, err := reference.WithTag(imageName, "latest")
	checkErr(t, err, "building tag reference")

	u, err := env.builder.BuildManifestURL(ref)
	checkErr(t, err, "building URL")

	resp, err := httpDelete(u)
	msg := "deleting tag"
	checkErr(t, err, msg)
	defer resp.Body.Close()

	checkResponse(t, msg, resp, http.StatusMethodNotAllowed)
}

// storageManifestErrDriverFactory implements the factory.StorageDriverFactory interface.
type storageManifestErrDriverFactory struct{}

const (
	repositoryWithManifestNotFound    = "manifesttagnotfound"
	repositoryWithManifestInvalidPath = "manifestinvalidpath"
	repositoryWithManifestBadLink     = "manifestbadlink"
	repositoryWithGenericStorageError = "genericstorageerr"
)

func (factory *storageManifestErrDriverFactory) Create(ctx context.Context, parameters map[string]any) (storagedriver.StorageDriver, error) {
	// Initialize the mock driver
	errGenericStorage := errors.New("generic storage error")
	return &mockErrorDriver{
		returnErrs: []mockErrorMapping{
			{
				pathMatch: fmt.Sprintf("%s/_manifests/tags", repositoryWithManifestNotFound),
				content:   nil,
				err:       storagedriver.PathNotFoundError{},
			},
			{
				pathMatch: fmt.Sprintf("%s/_manifests/tags", repositoryWithManifestInvalidPath),
				content:   nil,
				err:       storagedriver.InvalidPathError{},
			},
			{
				pathMatch: fmt.Sprintf("%s/_manifests/tags", repositoryWithManifestBadLink),
				content:   []byte("this is a bad sha"),
				err:       nil,
			},
			{
				pathMatch: fmt.Sprintf("%s/_manifests/tags", repositoryWithGenericStorageError),
				content:   nil,
				err:       errGenericStorage,
			},
		},
	}, nil
}

type mockErrorMapping struct {
	pathMatch string
	content   []byte
	err       error
}

// mockErrorDriver implements StorageDriver to force storage error on manifest request
type mockErrorDriver struct {
	storagedriver.StorageDriver
	returnErrs []mockErrorMapping
}

func (dr *mockErrorDriver) GetContent(ctx context.Context, path string) ([]byte, error) {
	for _, returns := range dr.returnErrs {
		if strings.Contains(path, returns.pathMatch) {
			return returns.content, returns.err
		}
	}
	return nil, errors.New("Unknown storage error")
}

func TestGetManifestWithStorageError(t *testing.T) {
	factory.Register("storagemanifesterror", &storageManifestErrDriverFactory{})
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"storagemanifesterror": configuration.Parameters{},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
	}
	config.HTTP.Headers = headerConfig
	env1 := newTestEnvWithConfig(t, &config)
	defer env1.Shutdown()

	repo, _ := reference.WithName(repositoryWithManifestNotFound)
	testManifestWithStorageError(t, env1, repo, http.StatusNotFound, errcode.ErrorCodeManifestUnknown)

	repo, _ = reference.WithName(repositoryWithGenericStorageError)
	testManifestWithStorageError(t, env1, repo, http.StatusInternalServerError, errcode.ErrorCodeUnknown)

	repo, _ = reference.WithName(repositoryWithManifestInvalidPath)
	testManifestWithStorageError(t, env1, repo, http.StatusInternalServerError, errcode.ErrorCodeUnknown)

	repo, _ = reference.WithName(repositoryWithManifestBadLink)
	testManifestWithStorageError(t, env1, repo, http.StatusInternalServerError, errcode.ErrorCodeUnknown)
}

func TestManifestDelete(t *testing.T) {
	schema2Repo, _ := reference.WithName("foo/schema2")

	deleteEnabled := true
	env := newTestEnv(t, deleteEnabled)
	defer env.Shutdown()
	schema2Args := testManifestAPISchema2(t, env, schema2Repo, "schema2tag")
	testManifestDelete(t, env, schema2Args)
}

func TestManifestDeleteDisabled(t *testing.T) {
	schema2Repo, _ := reference.WithName("foo/schema2")
	deleteEnabled := false
	env := newTestEnv(t, deleteEnabled)
	defer env.Shutdown()
	testManifestDeleteDisabled(t, env, schema2Repo)
}

func testManifestDeleteDisabled(t *testing.T, env *testEnv, imageName reference.Named) {
	ref, _ := reference.WithDigest(imageName, digestSha256EmptyTar)
	manifestURL, err := env.builder.BuildManifestURL(ref)
	if err != nil {
		t.Fatalf("unexpected error getting manifest url: %v", err)
	}

	resp, err := httpDelete(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error deleting manifest %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "status of disabled delete of manifest", resp, http.StatusMethodNotAllowed)
}

func testManifestWithStorageError(t *testing.T, env *testEnv, imageName reference.Named, expectedStatusCode int, expectedErrorCode errcode.ErrorCode) {
	tag := "latest"
	tagRef, _ := reference.WithTag(imageName, tag)
	manifestURL, err := env.builder.BuildManifestURL(tagRef)
	if err != nil {
		t.Fatalf("unexpected error getting manifest url: %v", err)
	}

	// -----------------------------
	// Attempt to fetch the manifest
	resp, err := http.Get(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error getting manifest: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "getting non-existent manifest", resp, expectedStatusCode)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, "getting non-existent manifest", resp, expectedErrorCode)
}

func testManifestAPISchema2(t *testing.T, env *testEnv, imageName reference.Named, tag string) manifestArgs {
	args := manifestArgs{
		imageName: imageName,
		mediaType: schema2.MediaTypeManifest,
	}

	tagRef, _ := reference.WithTag(imageName, tag)
	manifestURL, err := env.builder.BuildManifestURL(tagRef)
	if err != nil {
		t.Fatalf("unexpected error getting manifest url: %v", err)
	}

	// -----------------------------
	// Attempt to fetch the manifest
	resp, err := http.Get(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error getting manifest: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "getting non-existent manifest", resp, http.StatusNotFound)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, "getting non-existent manifest", resp, errcode.ErrorCodeManifestUnknown)

	tagsURL, err := env.builder.BuildTagsURL(imageName)
	if err != nil {
		t.Fatalf("unexpected error building tags url: %v", err)
	}

	resp, err = http.Get(tagsURL)
	if err != nil {
		t.Fatalf("unexpected error getting unknown tags: %v", err)
	}
	defer resp.Body.Close()

	// Check that we get an unknown repository error when asking for tags
	checkResponse(t, "getting unknown manifest tags", resp, http.StatusNotFound)
	// nolint:errcheck
	checkBodyHasErrorCodes(t, "getting unknown manifest tags", resp, errcode.ErrorCodeNameUnknown)

	// --------------------------------
	// Attempt to push manifest with missing config and missing layers
	manifest := &schema2.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: schema2.MediaTypeManifest,
		Config: v1.Descriptor{
			Digest:    "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:      3253,
			MediaType: schema2.MediaTypeImageConfig,
		},
		Layers: []v1.Descriptor{
			{
				Digest:    "sha256:463434349086340864309863409683460843608348608934092322395278926a",
				Size:      6323,
				MediaType: schema2.MediaTypeLayer,
			},
			{
				Digest:    "sha256:630923423623623423352523525237238023652897356239852383652aaaaaaa",
				Size:      6863,
				MediaType: schema2.MediaTypeLayer,
			},
		},
	}

	resp = putManifest(t, "putting missing config manifest", manifestURL, schema2.MediaTypeManifest, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting missing config manifest", resp, http.StatusBadRequest)
	_, p, counts := checkBodyHasErrorCodes(t, "putting missing config manifest", resp, errcode.ErrorCodeManifestBlobUnknown)

	expectedCounts := map[errcode.ErrorCode]int{
		errcode.ErrorCodeManifestBlobUnknown: 3,
	}

	if !reflect.DeepEqual(counts, expectedCounts) {
		t.Fatalf("unexpected number of error codes encountered: %v\n!=\n%v\n---\n%s", counts, expectedCounts, string(p))
	}

	// Push a config, and reference it in the manifest
	sampleConfig := []byte(`{
		"architecture": "amd64",
		"history": [
		  {
		    "created": "2015-10-31T22:22:54.690851953Z",
		    "created_by": "/bin/sh -c #(nop) ADD file:a3bc1e842b69636f9df5256c49c5374fb4eef1e281fe3f282c65fb853ee171c5 in /"
		  },
		  {
		    "created": "2015-10-31T22:22:55.613815829Z",
		    "created_by": "/bin/sh -c #(nop) CMD [\"sh\"]"
		  }
		],
		"rootfs": {
		  "diff_ids": [
		    "sha256:c6f988f4874bb0add23a778f753c65efe992244e148a1d2ec2a8b664fb66bbd1",
		    "sha256:5f70bf18a086007016e948b04aed3b82103a36bea41755b6cddfaf10ace3c6ef"
		  ],
		  "type": "layers"
		}
	}`)
	sampleConfigDigest := digest.FromBytes(sampleConfig)

	uploadURLBase, _ := startPushLayer(t, env, imageName)
	pushLayer(t, env.builder, imageName, sampleConfigDigest, uploadURLBase, bytes.NewReader(sampleConfig))
	manifest.Config.Digest = sampleConfigDigest
	manifest.Config.Size = int64(len(sampleConfig))

	// The manifest should still be invalid, because its layer doesn't exist
	resp = putManifest(t, "putting missing layer manifest", manifestURL, schema2.MediaTypeManifest, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting missing layer manifest", resp, http.StatusBadRequest)
	_, p, counts = checkBodyHasErrorCodes(t, "getting unknown manifest tags", resp, errcode.ErrorCodeManifestBlobUnknown)

	expectedCounts = map[errcode.ErrorCode]int{
		errcode.ErrorCodeManifestBlobUnknown: 2,
	}

	if !reflect.DeepEqual(counts, expectedCounts) {
		t.Fatalf("unexpected number of error codes encountered: %v\n!=\n%v\n---\n%s", counts, expectedCounts, string(p))
	}

	// Push 2 random layers
	expectedLayers := make(map[digest.Digest]io.ReadSeeker)

	for i := range manifest.Layers {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("error creating random layer %d: %v", i, err)
		}

		expectedLayers[dgst] = rs
		manifest.Layers[i].Digest = dgst

		uploadURLBase, _ := startPushLayer(t, env, imageName)
		pushLayer(t, env.builder, imageName, dgst, uploadURLBase, rs)
	}

	// -------------------
	// Push the manifest with all layers pushed.
	deserializedManifest, err := schema2.FromStruct(*manifest)
	if err != nil {
		t.Fatalf("could not create DeserializedManifest: %v", err)
	}
	_, canonical, err := deserializedManifest.Payload()
	if err != nil {
		t.Fatalf("could not get manifest payload: %v", err)
	}
	dgst := digest.FromBytes(canonical)
	args.dgst = dgst
	args.manifest = deserializedManifest

	digestRef, _ := reference.WithDigest(imageName, dgst)
	manifestDigestURL, err := env.builder.BuildManifestURL(digestRef)
	checkErr(t, err, "building manifest url")

	resp = putManifest(t, "putting manifest no error", manifestURL, schema2.MediaTypeManifest, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting manifest no error", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// --------------------
	// Push by digest -- should get same result
	resp = putManifest(t, "putting manifest by digest", manifestDigestURL, schema2.MediaTypeManifest, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting manifest by digest", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// ------------------
	// Fetch by tag name

	// HEAD requests should emit a logging entry and not contain a body
	hook := hookstest.NewGlobal()
	defer hook.Reset()

	headReq, err := http.NewRequest(http.MethodHead, manifestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	headResp, err := http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatalf("unexpected error head manifest: %v", err)
	}
	defer headResp.Body.Close()

	checkResponse(t, "head uploaded manifest", headResp, http.StatusOK)
	checkHeaders(t, headResp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	lastMsg := hook.LastEntry()
	if v := lastMsg.Data["http.request.method"]; v != http.MethodHead {
		t.Errorf("expected http.request.method to be %q, got %q", http.MethodHead, v)
	}
	if v := lastMsg.Data["http.response.status"]; v != http.StatusOK {
		t.Errorf("expected http.response.status to be %d, got %d", http.StatusOK, v)
	}

	headBody, err := io.ReadAll(headResp.Body)
	if err != nil {
		t.Fatalf("reading body for head manifest: %v", err)
	}

	if len(headBody) > 0 {
		t.Fatalf("unexpected body length for head manifest: %d", len(headBody))
	}

	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("Accept", schema2.MediaTypeManifest)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching uploaded manifest", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	var fetchedManifest schema2.DeserializedManifest
	dec := json.NewDecoder(resp.Body)

	if err := dec.Decode(&fetchedManifest); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	_, fetchedCanonical, err := fetchedManifest.Payload()
	if err != nil {
		t.Fatalf("error getting manifest payload: %v", err)
	}

	if !bytes.Equal(fetchedCanonical, canonical) {
		t.Fatal("manifests do not match")
	}

	// ---------------
	// Fetch by digest

	// HEAD requests should not contain a body
	headReq, err = http.NewRequest(http.MethodHead, manifestDigestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	headResp, err = http.DefaultClient.Do(headReq)
	if err != nil {
		t.Fatalf("unexpected error head manifest: %v", err)
	}
	defer headResp.Body.Close()

	checkResponse(t, "head uploaded manifest by digest", headResp, http.StatusOK)
	checkHeaders(t, headResp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	headBody, err = io.ReadAll(headResp.Body)
	if err != nil {
		t.Fatalf("reading body for head manifest by digest: %v", err)
	}

	if len(headBody) > 0 {
		t.Fatalf("unexpected body length for head manifest: %d", len(headBody))
	}
	req, err = http.NewRequest(http.MethodGet, manifestDigestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("Accept", schema2.MediaTypeManifest)
	resp, err = http.DefaultClient.Do(req)
	checkErr(t, err, "fetching manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "fetching uploaded manifest", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	var fetchedManifestByDigest schema2.DeserializedManifest
	dec = json.NewDecoder(resp.Body)
	if err := dec.Decode(&fetchedManifestByDigest); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	_, fetchedCanonical, err = fetchedManifest.Payload()
	if err != nil {
		t.Fatalf("error getting manifest payload: %v", err)
	}

	if !bytes.Equal(fetchedCanonical, canonical) {
		t.Fatal("manifests do not match")
	}

	// Get by name with etag, gives 304
	etag := resp.Header.Get("Etag")
	req, err = http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching manifest by name with etag", resp, http.StatusNotModified)

	// Get by digest with etag, gives 304
	req, err = http.NewRequest(http.MethodGet, manifestDigestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching manifest by dgst with etag", resp, http.StatusNotModified)

	// Ensure that the tag is listed.
	resp, err = http.Get(tagsURL)
	if err != nil {
		t.Fatalf("unexpected error getting unknown tags: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "getting unknown manifest tags", resp, http.StatusOK)
	dec = json.NewDecoder(resp.Body)

	var tagsResponse tagsAPIResponse

	if err := dec.Decode(&tagsResponse); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if tagsResponse.Name != imageName.Name() {
		t.Fatalf("tags name should match image name: %v != %v", tagsResponse.Name, imageName)
	}

	if len(tagsResponse.Tags) != 1 {
		t.Fatalf("expected some tags in response: %v", tagsResponse.Tags)
	}

	if tagsResponse.Tags[0] != tag {
		t.Fatalf("tag not as expected: %q != %q", tagsResponse.Tags[0], tag)
	}

	return args
}

func testManifestAPIManifestList(t *testing.T, env *testEnv, args manifestArgs) {
	imageName := args.imageName
	tag := "manifestlisttag"

	tagRef, _ := reference.WithTag(imageName, tag)
	manifestURL, err := env.builder.BuildManifestURL(tagRef)
	if err != nil {
		t.Fatalf("unexpected error getting manifest url: %v", err)
	}

	// --------------------------------
	// Attempt to push manifest list that refers to an unknown manifest
	manifestList := &manifestlist.ManifestList{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: manifestlist.MediaTypeManifestList,
		Manifests: []manifestlist.ManifestDescriptor{
			{
				Descriptor: v1.Descriptor{
					Digest:    "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
					Size:      3253,
					MediaType: schema2.MediaTypeManifest,
				},
				Platform: manifestlist.PlatformSpec{
					Architecture: "amd64",
					OS:           "linux",
				},
			},
		},
	}

	resp := putManifest(t, "putting missing manifest manifestlist", manifestURL, manifestlist.MediaTypeManifestList, manifestList)
	defer resp.Body.Close()
	checkResponse(t, "putting missing manifest manifestlist", resp, http.StatusBadRequest)
	_, p, counts := checkBodyHasErrorCodes(t, "putting missing manifest manifestlist", resp, errcode.ErrorCodeManifestBlobUnknown)

	expectedCounts := map[errcode.ErrorCode]int{
		errcode.ErrorCodeManifestBlobUnknown: 1,
	}

	if !reflect.DeepEqual(counts, expectedCounts) {
		t.Fatalf("unexpected number of error codes encountered: %v\n!=\n%v\n---\n%s", counts, expectedCounts, string(p))
	}

	// -------------------
	// Push a manifest list that references an actual manifest
	manifestList.Manifests[0].Digest = args.dgst
	deserializedManifestList, err := manifestlist.FromDescriptors(manifestList.Manifests)
	if err != nil {
		t.Fatalf("could not create DeserializedManifestList: %v", err)
	}
	_, canonical, err := deserializedManifestList.Payload()
	if err != nil {
		t.Fatalf("could not get manifest list payload: %v", err)
	}
	dgst := digest.FromBytes(canonical)

	digestRef, _ := reference.WithDigest(imageName, dgst)
	manifestDigestURL, err := env.builder.BuildManifestURL(digestRef)
	checkErr(t, err, "building manifest url")

	resp = putManifest(t, "putting manifest list no error", manifestURL, manifestlist.MediaTypeManifestList, deserializedManifestList)
	defer resp.Body.Close()
	checkResponse(t, "putting manifest list no error", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// --------------------
	// Push by digest -- should get same result
	resp = putManifest(t, "putting manifest list by digest", manifestDigestURL, manifestlist.MediaTypeManifestList, deserializedManifestList)
	defer resp.Body.Close()
	checkResponse(t, "putting manifest list by digest", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// ------------------
	// Fetch by tag name
	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	// multiple headers in mixed list format to ensure we parse correctly server-side
	req.Header.Set("Accept", fmt.Sprintf(` %s ; q=0.8 `, manifestlist.MediaTypeManifestList))
	req.Header.Add("Accept", schema2.MediaTypeManifest)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest list: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching uploaded manifest list", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	var fetchedManifestList manifestlist.DeserializedManifestList
	dec := json.NewDecoder(resp.Body)

	if err := dec.Decode(&fetchedManifestList); err != nil {
		t.Fatalf("error decoding fetched manifest list: %v", err)
	}

	_, fetchedCanonical, err := fetchedManifestList.Payload()
	if err != nil {
		t.Fatalf("error getting manifest list payload: %v", err)
	}

	if !bytes.Equal(fetchedCanonical, canonical) {
		t.Fatal("manifest lists do not match")
	}

	// ---------------
	// Fetch by digest
	req, err = http.NewRequest(http.MethodGet, manifestDigestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("Accept", manifestlist.MediaTypeManifestList)
	resp, err = http.DefaultClient.Do(req)
	checkErr(t, err, "fetching manifest list by digest")
	defer resp.Body.Close()

	checkResponse(t, "fetching uploaded manifest list", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
		"ETag":                  []string{fmt.Sprintf(`"%s"`, dgst)},
	})

	var fetchedManifestListByDigest manifestlist.DeserializedManifestList
	dec = json.NewDecoder(resp.Body)
	if err := dec.Decode(&fetchedManifestListByDigest); err != nil {
		t.Fatalf("error decoding fetched manifest: %v", err)
	}

	_, fetchedCanonical, err = fetchedManifestListByDigest.Payload()
	if err != nil {
		t.Fatalf("error getting manifest list payload: %v", err)
	}

	if !bytes.Equal(fetchedCanonical, canonical) {
		t.Fatal("manifests do not match")
	}

	// Get by name with etag, gives 304
	etag := resp.Header.Get("Etag")
	req, err = http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching manifest by name with etag", resp, http.StatusNotModified)

	// Get by digest with etag, gives 304
	req, err = http.NewRequest(http.MethodGet, manifestDigestURL, nil)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	req.Header.Set("If-None-Match", etag)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Error constructing request: %s", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "fetching manifest by dgst with etag", resp, http.StatusNotModified)
}

func testManifestDelete(t *testing.T, env *testEnv, args manifestArgs) {
	imageName := args.imageName
	dgst := args.dgst
	manifest := args.manifest

	ref, _ := reference.WithDigest(imageName, dgst)
	manifestDigestURL, _ := env.builder.BuildManifestURL(ref)
	// ---------------
	// Delete by digest
	resp, err := httpDelete(manifestDigestURL)
	checkErr(t, err, "deleting manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "deleting manifest", resp, http.StatusAccepted)
	checkHeaders(t, resp, http.Header{
		"Content-Length": []string{"0"},
	})

	// ---------------
	// Attempt to fetch deleted manifest
	resp, err = http.Get(manifestDigestURL)
	checkErr(t, err, "fetching deleted manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "fetching deleted manifest", resp, http.StatusNotFound)

	// ---------------
	// Delete already deleted manifest by digest
	resp, err = httpDelete(manifestDigestURL)
	checkErr(t, err, "re-deleting manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "re-deleting manifest", resp, http.StatusNotFound)

	// --------------------
	// Re-upload manifest by digest
	resp = putManifest(t, "putting manifest", manifestDigestURL, args.mediaType, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting manifest", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// ---------------
	// Attempt to fetch re-uploaded deleted digest
	resp, err = http.Get(manifestDigestURL)
	checkErr(t, err, "fetching re-uploaded manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "fetching re-uploaded manifest", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// ---------------
	// Attempt to delete an unknown manifest
	unknownDigest := digest.Digest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	unknownRef, _ := reference.WithDigest(imageName, unknownDigest)
	unknownManifestDigestURL, err := env.builder.BuildManifestURL(unknownRef)
	checkErr(t, err, "building unknown manifest url")

	resp, err = httpDelete(unknownManifestDigestURL)
	checkErr(t, err, "delting unknown manifest by digest")
	defer resp.Body.Close()
	checkResponse(t, "fetching deleted manifest", resp, http.StatusNotFound)

	// --------------------
	// Upload manifest by tag
	tag := "atag"
	tagRef, _ := reference.WithTag(imageName, tag)
	manifestTagURL, _ := env.builder.BuildManifestURL(tagRef)
	resp = putManifest(t, "putting manifest by tag", manifestTagURL, args.mediaType, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting manifest by tag", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	tagsURL, err := env.builder.BuildTagsURL(imageName)
	if err != nil {
		t.Fatalf("unexpected error building tags url: %v", err)
	}

	// Ensure that the tag is listed.
	resp, err = http.Get(tagsURL)
	if err != nil {
		t.Fatalf("unexpected error getting unknown tags: %v", err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var tagsResponse tagsAPIResponse
	if err := dec.Decode(&tagsResponse); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if tagsResponse.Name != imageName.Name() {
		t.Fatalf("tags name should match image name: %v != %v", tagsResponse.Name, imageName)
	}

	if len(tagsResponse.Tags) != 1 {
		t.Fatalf("expected some tags in response: %v", tagsResponse.Tags)
	}

	if tagsResponse.Tags[0] != tag {
		t.Fatalf("tag not as expected: %q != %q", tagsResponse.Tags[0], tag)
	}

	// ---------------
	// Delete by digest
	resp, err = httpDelete(manifestDigestURL)
	checkErr(t, err, "deleting manifest by digest")
	defer resp.Body.Close()

	checkResponse(t, "deleting manifest with tag", resp, http.StatusAccepted)
	checkHeaders(t, resp, http.Header{
		"Content-Length": []string{"0"},
	})

	// Ensure that the tag is not listed.
	resp, err = http.Get(tagsURL)
	if err != nil {
		t.Fatalf("unexpected error getting unknown tags: %v", err)
	}
	defer resp.Body.Close()

	dec = json.NewDecoder(resp.Body)
	if err := dec.Decode(&tagsResponse); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if tagsResponse.Name != imageName.Name() {
		t.Fatalf("tags name should match image name: %v != %v", tagsResponse.Name, imageName)
	}

	if len(tagsResponse.Tags) != 0 {
		t.Fatalf("expected 0 tags in response: %v", tagsResponse.Tags)
	}
}

type testEnv struct {
	ctx     context.Context
	config  configuration.Configuration
	app     *App
	server  *httptest.Server
	builder *v2.URLBuilder
}

func newTestEnvMirror(t *testing.T, deleteEnabled bool) *testEnv {
	upstreamEnv := newTestEnv(t, deleteEnabled)
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"delete":   configuration.Parameters{"enabled": deleteEnabled},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
		Proxy: configuration.Proxy{
			RemoteURL: upstreamEnv.server.URL,
		},
		Catalog: configuration.Catalog{
			MaxEntries: 5,
		},
	}

	return newTestEnvWithConfig(t, &config)
}

func newTestEnv(t *testing.T, deleteEnabled bool) *testEnv {
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"delete":   configuration.Parameters{"enabled": deleteEnabled},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
		Catalog: configuration.Catalog{
			MaxEntries: 5,
		},
	}

	config.HTTP.Headers = headerConfig

	return newTestEnvWithConfig(t, &config)
}

func newTestEnvWithConfig(t *testing.T, config *configuration.Configuration) *testEnv {
	ctx := context.Background()

	app := NewApp(ctx, config)
	server := httptest.NewServer(handlers.CombinedLoggingHandler(os.Stderr, app))
	builder, err := v2.NewURLBuilderFromString(server.URL+config.HTTP.Prefix, false)
	if err != nil {
		t.Fatalf("error creating url builder: %v", err)
	}

	return &testEnv{
		ctx:     ctx,
		config:  *config,
		app:     app,
		server:  server,
		builder: builder,
	}
}

func (t *testEnv) Shutdown() {
	t.server.CloseClientConnections()
	t.server.Close()
}

func putManifest(t *testing.T, msg, url, contentType string, v any) *http.Response {
	var body []byte

	switch m := v.(type) {
	case *manifestlist.DeserializedManifestList:
		_, pl, err := m.Payload()
		if err != nil {
			t.Fatalf("error getting payload: %v", err)
		}
		body = pl
	default:
		var err error
		body, err = json.MarshalIndent(v, "", "   ")
		if err != nil {
			t.Fatalf("unexpected error marshaling %v: %v", v, err)
		}
	}

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("error creating request for %s: %v", msg, err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("error doing put request while %s: %v", msg, err)
	}

	return resp
}

func startPushLayer(t *testing.T, env *testEnv, name reference.Named) (location string, uuid string) {
	layerUploadURL, err := env.builder.BuildBlobUploadURL(name)
	if err != nil {
		t.Fatalf("unexpected error building layer upload url: %v", err)
	}

	u, err := url.Parse(layerUploadURL)
	if err != nil {
		t.Fatalf("error parsing layer upload URL: %v", err)
	}

	base, err := url.Parse(env.server.URL)
	if err != nil {
		t.Fatalf("error parsing server URL: %v", err)
	}

	layerUploadURL = base.ResolveReference(u).String()
	resp, err := http.Post(layerUploadURL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error starting layer push: %v", err)
	}

	defer resp.Body.Close()

	checkResponse(t, fmt.Sprintf("pushing starting layer push %v", name.String()), resp, http.StatusAccepted)

	u, err = url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("error parsing location header: %v", err)
	}

	uuid = path.Base(u.Path)
	checkHeaders(t, resp, http.Header{
		"Location":           []string{"*"},
		"Content-Length":     []string{"0"},
		"Docker-Upload-UUID": []string{uuid},
	})

	return resp.Header.Get("Location"), uuid
}

// doPushLayer pushes the layer content returning the url on success returning
// the response. If you're only expecting a successful response, use pushLayer.
func doPushLayer(t *testing.T, ub *v2.URLBuilder, name reference.Named, dgst digest.Digest, uploadURLBase string, body io.Reader) (*http.Response, error) {
	u, err := url.Parse(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error parsing pushLayer url: %v", err)
	}

	u.RawQuery = url.Values{
		"_state": u.Query()["_state"],
		"digest": []string{dgst.String()},
	}.Encode()

	uploadURL := u.String()

	// Just do a monolithic upload
	req, err := http.NewRequest(http.MethodPut, uploadURL, body)
	if err != nil {
		t.Fatalf("unexpected error creating new request: %v", err)
	}

	return http.DefaultClient.Do(req)
}

// pushLayer pushes the layer content returning the url on success.
func pushLayer(t *testing.T, ub *v2.URLBuilder, name reference.Named, dgst digest.Digest, uploadURLBase string, body io.Reader) string {
	digester := digest.Canonical.Digester()

	resp, err := doPushLayer(t, ub, name, dgst, uploadURLBase, io.TeeReader(body, digester.Hash()))
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "putting monolithic chunk", resp, http.StatusCreated)

	if err != nil {
		t.Fatal("error generating sha256 digest of body")
	}

	sha256Dgst := digester.Digest()

	ref, _ := reference.WithDigest(name, sha256Dgst)
	expectedLayerURL, err := ub.BuildBlobURL(ref)
	if err != nil {
		t.Fatalf("error building expected layer url: %v", err)
	}

	checkHeaders(t, resp, http.Header{
		"Location":              []string{expectedLayerURL},
		"Content-Length":        []string{"0"},
		"Docker-Content-Digest": []string{sha256Dgst.String()},
	})

	return resp.Header.Get("Location")
}

func getUploadStatus(location string) (string, int64, error) {
	req, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		return location, -1, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return location, -1, err
	}

	defer resp.Body.Close()

	_, end, err := parseContentRange(resp.Header.Get("Range"))
	if err != nil {
		return location, -1, err
	}

	return resp.Header.Get("Location"), end, nil
}

func finishUpload(t *testing.T, ub *v2.URLBuilder, name reference.Named, uploadURLBase string, dgst digest.Digest) string {
	resp, err := doPushLayer(t, ub, name, dgst, uploadURLBase, nil)
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "putting monolithic chunk", resp, http.StatusCreated)

	ref, _ := reference.WithDigest(name, dgst)
	expectedLayerURL, err := ub.BuildBlobURL(ref)
	if err != nil {
		t.Fatalf("error building expected layer url: %v", err)
	}

	checkHeaders(t, resp, http.Header{
		"Location":              []string{expectedLayerURL},
		"Content-Length":        []string{"0"},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	return resp.Header.Get("Location")
}

type chunkOptions struct {
	// Content-Range header to set when pushing chunks
	contentRange string
}

func doPushChunk(t *testing.T, uploadURLBase string, body io.Reader, options chunkOptions) (*http.Response, error) {
	u, err := url.Parse(uploadURLBase)
	if err != nil {
		t.Fatalf("unexpected error parsing pushLayer url: %v", err)
	}

	u.RawQuery = url.Values{
		"_state": u.Query()["_state"],
	}.Encode()

	uploadURL := u.String()

	req, err := http.NewRequest(http.MethodPatch, uploadURL, body)
	if err != nil {
		t.Fatalf("unexpected error creating new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if options.contentRange != "" {
		req.Header.Set("Content-Range", options.contentRange)
	}

	resp, err := http.DefaultClient.Do(req)

	return resp, err
}

func pushChunk(t *testing.T, ub *v2.URLBuilder, name reference.Named, uploadURLBase string, body io.Reader, length int64) (string, digest.Digest) {
	digester := digest.Canonical.Digester()

	resp, err := doPushChunk(t, uploadURLBase, io.TeeReader(body, digester.Hash()), chunkOptions{})
	if err != nil {
		t.Fatalf("unexpected error doing push layer request: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, "putting chunk", resp, http.StatusAccepted)

	if err != nil {
		t.Fatal("error generating sha256 digest of body")
	}

	checkHeaders(t, resp, http.Header{
		"Range":          []string{fmt.Sprintf("0-%d", length-1)},
		"Content-Length": []string{"0"},
	})

	return resp.Header.Get("Location"), digester.Digest()
}

func checkResponse(t *testing.T, msg string, resp *http.Response, expectedStatus int) {
	if resp.StatusCode != expectedStatus {
		t.Logf("unexpected status %s: expected %v, got %v", msg, resp.StatusCode, expectedStatus)
		maybeDumpResponse(t, resp)
		t.FailNow()
	}

	// We expect the headers included in the configuration, unless the
	// status code is 405 (Method Not Allowed), which means the handler
	// doesn't even get called.
	if resp.StatusCode != 405 && !reflect.DeepEqual(resp.Header["X-Content-Type-Options"], []string{"nosniff"}) {
		t.Logf("missing or incorrect header X-Content-Type-Options %s", msg)
		maybeDumpResponse(t, resp)
		t.FailNow()
	}
}

// checkBodyHasErrorCodes ensures the body is an error body and has the
// expected error codes, returning the error structure, the json slice and a
// count of the errors by code.
func checkBodyHasErrorCodes(t *testing.T, msg string, resp *http.Response, errorCodes ...errcode.ErrorCode) (errcode.Errors, []byte, map[errcode.ErrorCode]int) {
	p, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unexpected error reading body %s: %v", msg, err)
	}

	var errs errcode.Errors
	if err := json.Unmarshal(p, &errs); err != nil {
		t.Fatalf("unexpected error decoding error response: %v", err)
	}

	if len(errs) == 0 {
		t.Fatal("expected errors in response")
	}

	// TODO(stevvooe): Shoot. The error setup is not working out. The content-
	// type headers are being set after writing the status code.
	// if resp.Header.Get("Content-Type") != "application/json" {
	// 	t.Fatalf("unexpected content type: %v != 'application/json'",
	// 		resp.Header.Get("Content-Type"))
	// }

	expected := map[errcode.ErrorCode]struct{}{}
	counts := map[errcode.ErrorCode]int{}

	// Initialize map with zeros for expected
	for _, code := range errorCodes {
		expected[code] = struct{}{}
		counts[code] = 0
	}

	for _, e := range errs {
		err, ok := e.(errcode.ErrorCoder)
		if !ok {
			t.Fatalf("not an ErrorCoder: %#v", e)
		}
		if _, ok := expected[err.ErrorCode()]; !ok {
			t.Fatalf("unexpected error code %v encountered during %s: %s ", err.ErrorCode(), msg, string(p))
		}
		counts[err.ErrorCode()]++
	}

	// Ensure that counts of expected errors were all non-zero
	for code := range expected {
		if counts[code] == 0 {
			t.Fatalf("expected error code %v not encountered during %s: %s", code, msg, string(p))
		}
	}

	return errs, p, counts
}

func maybeDumpResponse(t *testing.T, resp *http.Response) {
	if d, err := httputil.DumpResponse(resp, true); err != nil {
		t.Logf("error dumping response: %v", err)
	} else {
		t.Logf("response:\n%s", string(d))
	}
}

// matchHeaders checks that the response has at least the headers. If not, the
// test will fail. If a passed in header value is "*", any non-zero value will
// suffice as a match.
func checkHeaders(t *testing.T, resp *http.Response, headers http.Header) {
	t.Helper()

	for k, vs := range headers {
		if resp.Header.Get(k) == "" {
			t.Fatalf("response missing header %q", k)
		}

		for _, v := range vs {
			if v == "*" {
				// Just ensure there is some value.
				if len(resp.Header[http.CanonicalHeaderKey(k)]) > 0 {
					continue
				}
			}

			for _, hv := range resp.Header[http.CanonicalHeaderKey(k)] {
				if hv != v {
					t.Fatalf("%+v %v header value not matched in response: %q != %q", resp.Header, k, hv, v)
				}
			}
		}
	}
}

func checkErr(t *testing.T, err error, msg string) {
	if err != nil {
		t.Fatalf("unexpected error %s: %v", msg, err)
	}
}

func createRepository(env *testEnv, t *testing.T, imageName string, tag string) digest.Digest {
	imageNameRef, err := reference.WithName(imageName)
	if err != nil {
		t.Fatalf("unable to parse reference: %v", err)
	}

	tagRef, _ := reference.WithTag(imageNameRef, tag)
	manifestURL, err := env.builder.BuildManifestURL(tagRef)
	if err != nil {
		t.Fatalf("unexpected error getting manifest url: %v", err)
	}

	manifest := &schema2.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: schema2.MediaTypeManifest,
		Config: v1.Descriptor{
			Digest:    "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:      3253,
			MediaType: schema2.MediaTypeImageConfig,
		},
		Layers: []v1.Descriptor{
			{
				Digest:    "sha256:463434349086340864309863409683460843608348608934092322395278926a",
				Size:      6323,
				MediaType: schema2.MediaTypeLayer,
			},
		},
	}

	// Push a config, and reference it in the manifest
	sampleConfig := []byte(`{
		"architecture": "amd64",
		"history": [
		  {
		    "created": "2015-10-31T22:22:54.690851953Z",
		    "created_by": "/bin/sh -c #(nop) ADD file:a3bc1e842b69636f9df5256c49c5374fb4eef1e281fe3f282c65fb853ee171c5 in /"
		  },
		],
		"rootfs": {
		  "diff_ids": [
		    "sha256:c6f988f4874bb0add23a778f753c65efe992244e148a1d2ec2a8b664fb66bbd1",
		  ],
		  "type": "layers"
		}
	}`)
	sampleConfigDigest := digest.FromBytes(sampleConfig)

	uploadURLBase, _ := startPushLayer(t, env, imageNameRef)
	pushLayer(t, env.builder, imageNameRef, sampleConfigDigest, uploadURLBase, bytes.NewReader(sampleConfig))
	manifest.Config.Digest = sampleConfigDigest
	manifest.Config.Size = int64(len(sampleConfig))

	// Push random layers

	for i := range manifest.Layers {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("error creating random layer %d: %v", i, err)
		}
		manifest.Layers[i].Digest = dgst

		uploadURLBase, _ := startPushLayer(t, env, imageNameRef)
		pushLayer(t, env.builder, imageNameRef, dgst, uploadURLBase, rs)
	}

	// -------------------
	// Push the manifest with all layers pushed.
	deserializedManifest, err := schema2.FromStruct(*manifest)
	if err != nil {
		t.Fatalf("could not create DeserializedManifest: %v", err)
	}
	_, canonical, err := deserializedManifest.Payload()
	if err != nil {
		t.Fatalf("could not get manifest payload: %v", err)
	}
	dgst := digest.FromBytes(canonical)
	digestRef, _ := reference.WithDigest(imageNameRef, dgst)
	manifestDigestURL, err := env.builder.BuildManifestURL(digestRef)
	checkErr(t, err, "building manifest url")

	resp := putManifest(t, "putting manifest no error", manifestURL, schema2.MediaTypeManifest, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting manifest no error", resp, http.StatusCreated)
	checkHeaders(t, resp, http.Header{
		"Location":              []string{manifestDigestURL},
		"Docker-Content-Digest": []string{dgst.String()},
	})

	return dgst
}

// Test mutation operations on a registry configured as a cache.  Ensure that they return
// appropriate errors.
func TestRegistryAsCacheMutationAPIs(t *testing.T) {
	deleteEnabled := true
	env := newTestEnvMirror(t, deleteEnabled)
	defer env.Shutdown()

	imageName, _ := reference.WithName("foo/bar")
	tag := "latest"
	tagRef, _ := reference.WithTag(imageName, tag)
	manifestURL, err := env.builder.BuildManifestURL(tagRef)
	if err != nil {
		t.Fatalf("unexpected error building base url: %v", err)
	}

	// Manifest upload
	manifest := &schema2.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: schema2.MediaTypeManifest,
		Config: v1.Descriptor{
			Digest:    "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:      3253,
			MediaType: schema2.MediaTypeImageConfig,
		},
		Layers: []v1.Descriptor{
			{
				Digest:    "sha256:463434349086340864309863409683460843608348608934092322395278926a",
				Size:      6323,
				MediaType: schema2.MediaTypeLayer,
			},
			{
				Digest:    "sha256:630923423623623423352523525237238023652897356239852383652aaaaaaa",
				Size:      6863,
				MediaType: schema2.MediaTypeLayer,
			},
		},
	}

	resp := putManifest(t, "putting missing config manifest", manifestURL, schema2.MediaTypeManifest, manifest)
	defer resp.Body.Close()
	checkResponse(t, "putting missing config manifest", resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)

	// Manifest Delete
	resp, err = httpDelete(manifestURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "deleting config manifest from cache", resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)

	// Blob upload initialization
	layerUploadURL, err := env.builder.BuildBlobUploadURL(imageName)
	if err != nil {
		t.Fatalf("unexpected error building layer upload url: %v", err)
	}

	resp, err = http.Post(layerUploadURL, "", nil)
	if err != nil {
		t.Fatalf("unexpected error starting layer push: %v", err)
	}
	defer resp.Body.Close()

	checkResponse(t, fmt.Sprintf("starting layer push to cache %v", imageName), resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)

	// Blob Delete
	ref, _ := reference.WithDigest(imageName, digestSha256EmptyTar)
	blobURL, _ := env.builder.BuildBlobURL(ref)
	resp, err = httpDelete(blobURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	checkResponse(t, "deleting blob from cache", resp, errcode.ErrorCodeUnsupported.Descriptor().HTTPStatusCode)
}

func TestProxyManifestGetByTag(t *testing.T) {
	truthConfig := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
	}
	truthConfig.HTTP.Headers = headerConfig

	imageName, _ := reference.WithName("foo/bar")
	tag := "latest"

	truthEnv := newTestEnvWithConfig(t, &truthConfig)
	defer truthEnv.Shutdown()
	// create a repository in the truth registry
	dgst := createRepository(truthEnv, t, imageName.Name(), tag)

	proxyConfig := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
		Proxy: configuration.Proxy{
			RemoteURL: truthEnv.server.URL,
		},
	}
	proxyConfig.HTTP.Headers = headerConfig

	proxyEnv := newTestEnvWithConfig(t, &proxyConfig)
	defer proxyEnv.Shutdown()

	digestRef, _ := reference.WithDigest(imageName, dgst)
	manifestDigestURL, err := proxyEnv.builder.BuildManifestURL(digestRef)
	checkErr(t, err, "building manifest url")

	resp, err := http.Get(manifestDigestURL)
	checkErr(t, err, "fetching manifest from proxy by digest")
	defer resp.Body.Close()

	tagRef, _ := reference.WithTag(imageName, tag)
	manifestTagURL, err := proxyEnv.builder.BuildManifestURL(tagRef)
	checkErr(t, err, "building manifest url")

	resp, err = http.Get(manifestTagURL)
	checkErr(t, err, "fetching manifest from proxy by tag (error check 1)")
	defer resp.Body.Close()
	checkResponse(t, "fetching manifest from proxy by tag (response check 1)", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{dgst.String()},
	})

	// Create another manifest in the remote with the same image/tag pair
	newDigest := createRepository(truthEnv, t, imageName.Name(), tag)
	if dgst == newDigest {
		t.Fatal("non-random test data")
	}

	// fetch it with the same proxy URL as before.  Ensure the updated content is at the same tag
	resp, err = http.Get(manifestTagURL)
	checkErr(t, err, "fetching manifest from proxy by tag (error check 2)")
	defer resp.Body.Close()
	checkResponse(t, "fetching manifest from proxy by tag (response check 2)", resp, http.StatusOK)
	checkHeaders(t, resp, http.Header{
		"Docker-Content-Digest": []string{newDigest.String()},
	})
}

func TestArtifactManifest(t *testing.T) {
	for name, test := range map[string]struct {
		manifest      func(*testing.T, *testEnv, reference.Named) distribution.Manifest
		deleteSubject bool
	}{
		// The link is made when the subject already exists and is kept if the
		// subject is deleted
		"subject_exists": {
			manifest: func(t *testing.T, testEnv *testEnv, repo reference.Named) distribution.Manifest {
				args := testManifestAPISchema2(t, testEnv, repo, "schema2tag")
				_, payload, err := args.manifest.Payload()
				if err != nil {
					t.Fatalf("Failed to get subject payload: %s", err)
				}

				pushScratch(t, testEnv, repo)

				manifest, err := ocischema.FromStruct(ocischema.Manifest{
					Versioned:    specs.Versioned{SchemaVersion: 2},
					ArtifactType: "application/vnd.example.sbom.v1",
					Config:       emptyJsonDescriptor,
					Subject: &distribution.Descriptor{
						MediaType: args.mediaType,
						Digest:    args.dgst,
						Size:      int64(len(payload)),
					},
				})
				if err != nil {
					t.Fatalf("Failed to create manifest: %s", err)
				}
				return manifest
			},
			deleteSubject: true,
		},
		// When an OCI Image Manifest with a subject field is PUT before its
		// subject, the subject's referrers link will be made in advance.
		"image_manifest_with_subject": {
			manifest: func(t *testing.T, testEnv *testEnv, repo reference.Named) distribution.Manifest {
				config, configDigest, err := testutil.CreateRandomTarFile()
				if err != nil {
					t.Fatalf("Failed to create test blob: %s", err)
				}
				url, _ := startPushLayer(t, testEnv, repo)
				pushLayer(t, testEnv.builder, repo, configDigest, url, config)
				manifest, err := ocischema.FromStruct(ocischema.Manifest{
					Versioned: specs.Versioned{SchemaVersion: 2},
					Config: distribution.Descriptor{
						MediaType: v1.MediaTypeImageConfig,
						Digest:    configDigest,
					},
					Subject: &distribution.Descriptor{
						MediaType: v1.MediaTypeImageManifest,
						Digest:    "sha256:ebe054f08821294feee7bc442014fdd38b4836d83781d8ba99d38eb50d0c9d85",
						Size:      99,
					},
				})
				if err != nil {
					t.Fatalf("Failed to create manifest: %s", err)
				}
				return manifest
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			testEnv := newTestEnv(t, true)
			defer testEnv.Shutdown()

			repo, err := reference.WithName("myrepo/myimage")
			if err != nil {
				t.Fatalf("failed to make repo: %s", err)
			}

			manifest := test.manifest(t, testEnv, repo)

			contentType, payload, err := manifest.Payload()
			if err != nil {
				t.Fatalf("Failed to get raw manifest: %s", err)
			}
			ref, err := reference.WithDigest(repo, digest.FromBytes(payload))
			if err != nil {
				t.Fatalf("failed to make reference: %s", err)
			}

			manifestURL, err := testEnv.builder.BuildManifestURL(ref)
			if err != nil {
				t.Fatalf("Failed to build manifest URL: %s", err)
			}

			req, err := http.NewRequest(http.MethodPut, manifestURL, bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("Failed to create artifact PUT request: %s", err)
			}
			req.Header.Set("Content-Type", contentType)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to PUT manifest: %s", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusCreated {
				t.Fatalf("Incorrect status code for manifest PUT: %d, expected: %d", res.StatusCode, http.StatusCreated)
			}
			if res.Header.Get("Docker-Content-Digest") != ref.Digest().String() {
				t.Errorf("Incorrect Docker-Content-Digest header: %q, expected %q", res.Header.Get("Docker-Content-Digest"), ref.Digest().String())
			}

			// TODO(brackendawson): We should now try to get referrers for the subject
			// (which should also eventually exist for that to work), but those APIs
			// don't exist yet so for now just check the link was made.
			referrer, ok := manifest.(distribution.Referrer)
			if !ok {
				t.Fatalf("Manifest should implement distribution.Referrer: %T", manifest)
			}
			link, err := testEnv.app.driver.GetContent(context.Background(), fmt.Sprintf("/docker/registry/v2/repositories/%s/_manifests/revisions/sha256/%s/_referrers/_%s/sha256/%s/link",
				repo.Name(), referrer.Subject().Digest.Hex(), url.QueryEscape(referrer.Type()), ref.Digest().Hex()))
			if err != nil {
				t.Fatalf("Failed to get expected referrers link from subject with error: %s", err)
			}
			if string(link) != ref.Digest().String() {
				t.Errorf("Subject's referrers link has incorrect content:\n%s\nexpected:\n%s", string(link), ref.Digest().String())
			}

			// When an artifact manifest has been PUT it can be retrieved with GET.
			location := res.Header.Get("Location")
			req, err = http.NewRequest(http.MethodGet, location, nil)
			if err != nil {
				t.Fatalf("Failed to create artifact GET request: %s", err)
			}

			res, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to GET manifest: %s", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusNotAcceptable {
				t.Fatalf("Incorrect status code for manifest GET: %d, expected: %d", res.StatusCode, http.StatusNotAcceptable)
			}

			req.Header.Set("Accept", contentType)
			res, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to GET manifest: %s", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("Incorrect status code for manifest GET: %d, expected: %d", res.StatusCode, http.StatusOK)
			}
			if res.Header.Get("Content-Type") != contentType {
				t.Errorf("Incorrect mediaType for manifest GET: %q, expected: %q", res.Header.Get("Content-Type"), contentType)
			}
			gotManifest, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatalf("Failed to read manifest GET body: %s", err)
			}
			if !reflect.DeepEqual(gotManifest, payload) {
				t.Errorf("Pulled manifest does not match pushed manifest, got:\n%s\nexpected:\n%s", string(gotManifest), string(payload))
			}

			if test.deleteSubject {
				// When a subject is deleted, it's referrers link remains
				subjectRef, err := reference.WithDigest(repo, referrer.Subject().Digest)
				if err != nil {
					t.Fatalf("Failed to build subject reference: %s", err)
				}
				subjectURL, err := testEnv.builder.BuildManifestURL(subjectRef)
				if err != nil {
					t.Errorf("Failed to build subject URL: %s", err)
				}

				req, err := http.NewRequest(http.MethodDelete, subjectURL, nil)
				if err != nil {
					t.Fatalf("Failed to create subject DELETE request: %s", err)
				}
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatalf("Failed to DELETE subject: %s", err)
				}
				defer res.Body.Close()
				if res.StatusCode != http.StatusAccepted {
					t.Fatalf("Incorrect status code for subject DELETE: %s", err)
				}

				// TODO(brackendawson): We should now try to get referrers for the subject
				// (which should also eventually exist for that to work), but those APIs
				// don't exist yet so for now just check the link still exists.
				link, err := testEnv.app.driver.GetContent(context.Background(), fmt.Sprintf("/docker/registry/v2/repositories/%s/_manifests/revisions/sha256/%s/_referrers/_%s/sha256/%s/link",
					repo.Name(), referrer.Subject().Digest.Hex(), url.QueryEscape(referrer.Type()), ref.Digest().Hex()))
				if err != nil {
					t.Fatalf("Failed to get expected referrers link from subject with error: %s", err)
				}
				if string(link) != ref.Digest().String() {
					t.Errorf("Subject's referrers link has incorrect content:\n%s\nexpected:\n%s", string(link), ref.Digest().String())
				}
			}

			// When an artifact manifest is DELETEd then it will not be found if you GET
			// it. Its subject's referrer link will be left dangling.
			req, err = http.NewRequest(http.MethodDelete, location, nil)
			if err != nil {
				t.Fatalf("Failed to create artifact DELETE request: %s", err)
			}

			res, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to DELETE manifest: %s", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusAccepted {
				t.Fatalf("Incorrect status code for manifest DELETE: %d, expected: %d", res.StatusCode, http.StatusAccepted)
			}

			req, err = http.NewRequest(http.MethodGet, location, nil)
			if err != nil {
				t.Fatalf("Failed to create artifact GET request: %s", err)
			}
			req.Header.Set("Accept", v1.MediaTypeImageManifest)
			res, err = http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to GET manifest: %s", err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusNotFound {
				t.Fatalf("Incorrect status code for manifest GET: %d, expected: %d", res.StatusCode, http.StatusNotFound)
			}
		})
	}
}

func TestDockerManifestWithSubject(t *testing.T) {
	// When a docker image manifest containing a "subject" field is uploaded
	// then no referrer links are made for that invalid subject.
	testEnv := newTestEnv(t, true)
	defer testEnv.Shutdown()

	repo, err := reference.WithName("test/repo")
	if err != nil {
		t.Fatalf("Failed to build repo: %s", err)
	}

	config, configDigest, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("Failed to make config blob: %s", err)
	}
	url, _ := startPushLayer(t, testEnv, repo)
	pushLayer(t, testEnv.builder, repo, configDigest, url, config)
	layer, layerDigest, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("Failed to make layer blob: %s", err)
	}
	url, _ = startPushLayer(t, testEnv, repo)
	pushLayer(t, testEnv.builder, repo, layerDigest, url, layer)

	manifest, err := schema2.FromStruct(schema2.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: schema2.MediaTypeManifest,
		Config: distribution.Descriptor{
			MediaType: schema2.MediaTypeImageConfig,
			Digest:    configDigest,
			Size:      testutil.MustSeekerLen(config),
		},
		Layers: []distribution.Descriptor{
			{
				MediaType: schema2.MediaTypeLayer,
				Digest:    layerDigest,
				Size:      testutil.MustSeekerLen(layer),
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to make base manifest: %s", err)
	}
	var manifestFields map[string]interface{}
	_, payload, err := manifest.Payload()
	if err != nil {
		t.Fatalf("Failed to get base manifest payload: %s", err)
	}
	if err = json.Unmarshal(payload, &manifestFields); err != nil {
		t.Fatalf("Failed to unmarshal base manifest: %s", err)
	}
	manifestFields["subject"] = distribution.Descriptor{
		MediaType: schema2.MediaTypeManifest,
		Digest:    "sha256:118011bef6c697f7107cc0d788664a0f8c7d0316ce8d17673634155f5ecdba39",
		Size:      56,
	}
	rawManifest, _ := json.Marshal(manifestFields)
	if err = manifest.UnmarshalJSON(rawManifest); err != nil {
		t.Fatalf("Failed to re-build manifest: %s", err)
	}

	ref, err := reference.WithTag(repo, "latest")
	if err != nil {
		t.Fatalf("Failed to build reference: %s", err)
	}
	if url, err = testEnv.builder.BuildManifestURL(ref); err != nil {
		t.Fatalf("Failed to build manifest url: %s", err)
	}
	res := putManifest(t, "putting manifest", url, schema2.MediaTypeManifest, manifest)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("Incorrect status code from manifest PUT: %d, expected: %d", res.StatusCode, http.StatusCreated)
	}
	defer res.Body.Close()

	// TODO(brackendawson): We should now try to get referrers for the subject
	// (which should also eventually exist for that to work), but those APIs
	// don't exist yet so for now just check the link was not made.
	_, err = testEnv.app.driver.Stat(context.Background(), fmt.Sprintf("/docker/registry/v2/repositories/%s/_manifests/revisions/sha256/%s/_referrers",
		repo.Name(), res.Header.Get("Docker-Content-Digest")))
	var expectedErr storagedriver.InvalidPathError
	if !errors.As(err, &expectedErr) {
		t.Fatalf("Should have got invalid path error for referrer directory: %s", err)
	}
}

func TestArtifactManifestValidation(t *testing.T) {
	for name, test := range map[string]struct {
		config   func(*testing.T, *testEnv, reference.Named) distribution.Descriptor
		layers   func(*testing.T, *testEnv, reference.Named) []distribution.Descriptor
		wantCode int
	}{
		"layers_must_exist": {
			config: func(t *testing.T, testEnv *testEnv, repo reference.Named) distribution.Descriptor {
				pushScratch(t, testEnv, repo)
				return emptyJsonDescriptor
			},
			layers: func(t *testing.T, te *testEnv, n reference.Named) []distribution.Descriptor {
				// a layer which has not been uploaded
				return []distribution.Descriptor{
					{
						MediaType: v1.MediaTypeImageLayer,
						Digest:    "sha256:7688b6ef52555962d008fff894223582c484517cea7da49ee67800adc7fc8866",
						Size:      56,
					},
				}
			},
			wantCode: 400,
		},
		"config_must_be_set": {
			config: func(t *testing.T, testEnv *testEnv, repo reference.Named) distribution.Descriptor {
				return distribution.Descriptor{} // zero value
			},
			layers: func(t *testing.T, testEnv *testEnv, repo reference.Named) []distribution.Descriptor {
				layers := int64(10)
				digests := make([]distribution.Descriptor, layers)
				for i := int64(0); i < layers; i++ {
					rs, digest, err := testutil.CreateRandomTarFile()
					if err != nil {
						t.Fatalf("Failed to create test blob: %s", err)
					}
					url, _ := startPushLayer(t, testEnv, repo)
					pushLayer(t, testEnv.builder, repo, digest, url, rs)
					digests[i] = distribution.Descriptor{
						MediaType: v1.MediaTypeImageLayer,
						Digest:    digest,
						Size:      testutil.MustSeekerLen(rs),
					}
				}
				return digests
			},
			wantCode: 400,
		},
		"config_must_exist": {
			config: func(t *testing.T, testEnv *testEnv, repo reference.Named) distribution.Descriptor {
				return emptyJsonDescriptor // not uploaded
			},
			layers: func(t *testing.T, testEnv *testEnv, repo reference.Named) []distribution.Descriptor {
				layers := int64(10)
				digests := make([]distribution.Descriptor, layers)
				for i := int64(0); i < layers; i++ {
					rs, digest, err := testutil.CreateRandomTarFile()
					if err != nil {
						t.Fatalf("Failed to create test blob: %s", err)
					}
					url, _ := startPushLayer(t, testEnv, repo)
					pushLayer(t, testEnv.builder, repo, digest, url, rs)
					digests[i] = distribution.Descriptor{
						MediaType: v1.MediaTypeImageLayer,
						Digest:    digest,
						Size:      testutil.MustSeekerLen(rs),
					}
				}
				return digests
			},
			wantCode: 400,
		},
		"valid_blobs": {
			config: func(t *testing.T, testEnv *testEnv, repo reference.Named) distribution.Descriptor {
				pushScratch(t, testEnv, repo)
				return emptyJsonDescriptor
			},
			layers: func(t *testing.T, testEnv *testEnv, repo reference.Named) []distribution.Descriptor {
				layers := int64(10)
				digests := make([]distribution.Descriptor, layers)
				for i := int64(0); i < layers; i++ {
					rs, digest, err := testutil.CreateRandomTarFile()
					if err != nil {
						t.Fatalf("Failed to create test blob: %s", err)
					}
					url, _ := startPushLayer(t, testEnv, repo)
					pushLayer(t, testEnv.builder, repo, digest, url, rs)
					digests[i] = distribution.Descriptor{
						MediaType: v1.MediaTypeImageLayer,
						Digest:    digest,
						Size:      testutil.MustSeekerLen(rs),
					}
				}
				return digests
			},
			wantCode: 201,
		},
	} {
		t.Run(name, func(t *testing.T) {
			testEnv := newTestEnv(t, true)
			defer testEnv.Shutdown()

			repo, err := reference.WithName("myrepo/myimage")
			if err != nil {
				t.Fatalf("failed to make repo: %s", err)
			}

			manifest, err := ocischema.FromStruct(ocischema.Manifest{
				Versioned:    specs.Versioned{SchemaVersion: 2},
				Config:       test.config(t, testEnv, repo),
				ArtifactType: "application/vnd.example.sbom.v1",
				Layers:       test.layers(t, testEnv, repo),
			})
			if err != nil {
				t.Fatalf("Failed to make manifest: %s", err)
			}

			contentType, payload, err := manifest.Payload()
			if err != nil {
				t.Fatalf("Failed to get raw manifest: %s", err)
			}
			ref, err := reference.WithDigest(repo, digest.FromBytes(payload))
			if err != nil {
				t.Fatalf("failed to make reference: %s", err)
			}

			manifestURL, err := testEnv.builder.BuildManifestURL(ref)
			if err != nil {
				t.Fatalf("Failed to build manifest URL: %s", err)
			}

			req, err := http.NewRequest(http.MethodPut, manifestURL, bytes.NewReader(payload))
			if err != nil {
				t.Fatalf("Failed to create artifact PUT request: %s", err)
			}
			req.Header.Set("Content-Type", contentType)

			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to PUT manifest: %s", err)
			}
			defer res.Body.Close()
			if res.StatusCode != test.wantCode {
				t.Fatalf("Incorrect status code for manifest PUT: %d, expected: %d", res.StatusCode, test.wantCode)
			}
		})
	}
}

func pushScratch(t *testing.T, testEnv *testEnv, repo reference.Named) {
	url, _ := startPushLayer(t, testEnv, repo)
	pushLayer(t, testEnv.builder, repo, v1.DescriptorEmptyJSON.Digest, url, bytes.NewBuffer(v1.DescriptorEmptyJSON.Data))
}

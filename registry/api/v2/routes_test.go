package v2

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/mux"
)

type routeTestCase struct {
	RequestURI  string
	ExpectedURI string
	Vars        map[string]string
	RouteName   string
	StatusCode  int
}

// TestRouter registers a test handler with all the routes and ensures that
// each route returns the expected path variables. Not method verification is
// present. This not meant to be exhaustive but as check to ensure that the
// expected variables are extracted.
//
// This may go away as the application structure comes together.
func TestRouter(t *testing.T) {
	t.Parallel()
	tests := []routeTestCase{
		{
			RouteName:  RouteNameBase,
			RequestURI: "/v2/",
			Vars:       map[string]string{},
		},
		{
			RouteName:  RouteNameManifest,
			RequestURI: "/v2/foo/manifests/bar",
			Vars: map[string]string{
				"name":      "foo",
				"reference": "bar",
			},
		},
		{
			RouteName:  RouteNameManifest,
			RequestURI: "/v2/foo/bar/manifests/tag",
			Vars: map[string]string{
				"name":      "foo/bar",
				"reference": "tag",
			},
		},
		{
			RouteName:  RouteNameManifest,
			RequestURI: "/v2/foo/bar/manifests/sha256:abcdef01234567890",
			Vars: map[string]string{
				"name":      "foo/bar",
				"reference": "sha256:abcdef01234567890",
			},
		},
		{
			RouteName:  RouteNameTags,
			RequestURI: "/v2/foo/bar/tags/list",
			Vars: map[string]string{
				"name": "foo/bar",
			},
		},
		{
			RouteName:  RouteNameTags,
			RequestURI: "/v2/docker.com/foo/tags/list",
			Vars: map[string]string{
				"name": "docker.com/foo",
			},
		},
		{
			RouteName:  RouteNameTags,
			RequestURI: "/v2/docker.com/foo/bar/tags/list",
			Vars: map[string]string{
				"name": "docker.com/foo/bar",
			},
		},
		{
			RouteName:  RouteNameTags,
			RequestURI: "/v2/docker.com/foo/bar/baz/tags/list",
			Vars: map[string]string{
				"name": "docker.com/foo/bar/baz",
			},
		},
		{
			RouteName:  RouteNameBlob,
			RequestURI: "/v2/foo/bar/blobs/sha256:abcdef0919234",
			Vars: map[string]string{
				"name":   "foo/bar",
				"digest": "sha256:abcdef0919234",
			},
		},
		{
			RouteName:  RouteNameBlobUpload,
			RequestURI: "/v2/foo/bar/blobs/uploads/",
			Vars: map[string]string{
				"name": "foo/bar",
			},
		},
		{
			RouteName:  RouteNameBlobUploadChunk,
			RequestURI: "/v2/foo/bar/blobs/uploads/uuid",
			Vars: map[string]string{
				"name": "foo/bar",
				"uuid": "uuid",
			},
		},
		{
			// support uuid proper
			RouteName:  RouteNameBlobUploadChunk,
			RequestURI: "/v2/foo/bar/blobs/uploads/D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			Vars: map[string]string{
				"name": "foo/bar",
				"uuid": "D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			},
		},
		{
			RouteName:  RouteNameBlobUploadChunk,
			RequestURI: "/v2/foo/bar/blobs/uploads/RDk1MzA2RkEtRkFEMy00RTM2LThENDEtQ0YxQzkzRUY4Mjg2IA==",
			Vars: map[string]string{
				"name": "foo/bar",
				"uuid": "RDk1MzA2RkEtRkFEMy00RTM2LThENDEtQ0YxQzkzRUY4Mjg2IA==",
			},
		},
		{
			// supports urlsafe base64
			RouteName:  RouteNameBlobUploadChunk,
			RequestURI: "/v2/foo/bar/blobs/uploads/RDk1MzA2RkEtRkFEMy00RTM2LThENDEtQ0YxQzkzRUY4Mjg2IA_-==",
			Vars: map[string]string{
				"name": "foo/bar",
				"uuid": "RDk1MzA2RkEtRkFEMy00RTM2LThENDEtQ0YxQzkzRUY4Mjg2IA_-==",
			},
		},
		{
			// does not match
			RouteName:  RouteNameBlobUploadChunk,
			RequestURI: "/v2/foo/bar/blobs/uploads/totalandcompletejunk++$$-==",
			StatusCode: http.StatusNotFound,
		},
		{
			// Check ambiguity: ensure we can distinguish between tags for
			// "foo/bar/image/image" and image for "foo/bar/image" with tag
			// "tags"
			RouteName:  RouteNameManifest,
			RequestURI: "/v2/foo/bar/manifests/manifests/tags",
			Vars: map[string]string{
				"name":      "foo/bar/manifests",
				"reference": "tags",
			},
		},
		{
			// This case presents an ambiguity between foo/bar with tag="tags"
			// and list tags for "foo/bar/manifest"
			RouteName:  RouteNameTags,
			RequestURI: "/v2/foo/bar/manifests/tags/list",
			Vars: map[string]string{
				"name": "foo/bar/manifests",
			},
		},
		{
			RouteName:  RouteNameManifest,
			RequestURI: "/v2/locahost:8080/foo/bar/baz/manifests/tag",
			Vars: map[string]string{
				"name":      "locahost:8080/foo/bar/baz",
				"reference": "tag",
			},
		},
	}

	checkTestRouter(t, tests, "", true)
	checkTestRouter(t, tests, "/prefix/", true)
}

func TestRouterWithPathTraversals(t *testing.T) {
	t.Parallel()
	tests := []routeTestCase{
		{
			RouteName:   RouteNameBlobUploadChunk,
			RequestURI:  "/v2/foo/../../blobs/uploads/D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			ExpectedURI: "/blobs/uploads/D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			StatusCode:  http.StatusNotFound,
		},
		{
			// Testing for path traversal attack handling
			RouteName:   RouteNameTags,
			RequestURI:  "/v2/foo/../bar/baz/tags/list",
			ExpectedURI: "/v2/bar/baz/tags/list",
			Vars: map[string]string{
				"name": "bar/baz",
			},
		},
	}
	checkTestRouter(t, tests, "", false)
}

func TestRouterWithBadCharacters(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		tests := []routeTestCase{
			{
				RouteName:  RouteNameBlobUploadChunk,
				RequestURI: "/v2/foo/blobs/uploads/不95306FA-FAD3-4E36-8D41-CF1C93EF8286",
				StatusCode: http.StatusNotFound,
			},
			{
				// Testing for path traversal attack handling
				RouteName:  RouteNameTags,
				RequestURI: "/v2/foo/不bar/tags/list",
				StatusCode: http.StatusNotFound,
			},
		}
		checkTestRouter(t, tests, "", true)
	} else {
		// in the long version we're going to fuzz the router
		// with random UTF8 characters not in the 128 bit ASCII range.
		// These are not valid characters for the router and we expect
		// 404s on every test.
		tests := make([]routeTestCase, 1000)
		for idx := range tests {
			tests[idx] = routeTestCase{
				RouteName:  RouteNameTags,
				RequestURI: fmt.Sprintf("/v2/%v/%v/tags/list", randomString(10), randomString(10)),
				StatusCode: http.StatusNotFound,
			}
		}
		checkTestRouter(t, tests, "", true)
	}
}

func checkTestRouter(t *testing.T, tests []routeTestCase, prefix string, deeplyEqual bool) {
	router := RouterWithPrefix(prefix)

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testCase := routeTestCase{
			RequestURI: r.RequestURI,
			Vars:       mux.Vars(r),
			RouteName:  mux.CurrentRoute(r).GetName(),
		}

		enc := json.NewEncoder(w)

		if err := enc.Encode(testCase); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	// Startup test server
	server := httptest.NewServer(router)

	for _, tc := range tests {
		tc := tc
		requestURI := strings.TrimSuffix(prefix, "/") + tc.RequestURI
		t.Run("("+tc.RouteName+")"+requestURI, func(t *testing.T) {
			t.Parallel()
			tc.RequestURI = requestURI
			// Register the endpoint
			route := router.GetRoute(tc.RouteName)
			if route == nil {
				t.Fatalf("route for name %q not found", tc.RouteName)
			}

			route.Handler(testHandler)

			u := server.URL + tc.RequestURI

			resp, err := http.Get(u)
			if err != nil {
				t.Fatalf("error issuing get request: %v", err)
			}

			if tc.StatusCode == 0 {
				// Override default, zero-value
				tc.StatusCode = http.StatusOK
			}
			if tc.ExpectedURI == "" {
				// Override default, zero-value
				tc.ExpectedURI = tc.RequestURI
			}

			if resp.StatusCode != tc.StatusCode {
				t.Fatalf("unexpected status for %s: %v %v", u, resp.Status, resp.StatusCode)
			}

			if tc.StatusCode != http.StatusOK {
				resp.Body.Close()
				// We don't care about json response.
				return
			}

			dec := json.NewDecoder(resp.Body)

			var actualRouteInfo routeTestCase
			if err := dec.Decode(&actualRouteInfo); err != nil {
				t.Fatalf("error reading json response: %v", err)
			}
			// Needs to be set out of band
			actualRouteInfo.StatusCode = resp.StatusCode

			if actualRouteInfo.RequestURI != tc.ExpectedURI {
				t.Fatalf("URI %v incorrectly parsed, expected %v", actualRouteInfo.RequestURI, tc.ExpectedURI)
			}

			if actualRouteInfo.RouteName != tc.RouteName {
				t.Fatalf("incorrect route %q matched, expected %q", actualRouteInfo.RouteName, tc.RouteName)
			}

			// when testing deep equality, the actualRouteInfo has an empty ExpectedURI, we don't want
			// that to make the comparison fail. We're otherwise done with the testcase so empty the
			// testcase.ExpectedURI
			tc.ExpectedURI = ""
			if deeplyEqual && !reflect.DeepEqual(actualRouteInfo, tc) {
				t.Fatalf("actual does not equal expected: %#v != %#v", actualRouteInfo, tc)
			}

			resp.Body.Close()
		})
	}
}

// -------------- START LICENSED CODE --------------
// The following code is derivative of https://github.com/google/gofuzz
// gofuzz is licensed under the Apache License, Version 2.0, January 2004,
// a copy of which can be found in the LICENSE file at the root of this
// repository.

// These functions allow us to generate strings containing only multibyte
// characters that are invalid in our URLs. They are used above for fuzzing
// to ensure we always get 404s on these invalid strings
type charRange struct {
	first, last rune
}

// choose returns a random unicode character from the given range, using the
// given randomness source.
func (r *charRange) choose() rune {
	count := int64(r.last - r.first)
	return r.first + rune(rand.Int63n(count))
}

var unicodeRanges = []charRange{
	{'\u00a0', '\u02af'}, // Multi-byte encoded characters
	{'\u4e00', '\u9fff'}, // Common CJK (even longer encodings)
}

func randomString(length int) string {
	runes := make([]rune, length)
	for i := range runes {
		runes[i] = unicodeRanges[rand.Intn(len(unicodeRanges))].choose()
	}
	return string(runes)
}

// -------------- END LICENSED CODE --------------

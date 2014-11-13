package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gorilla/mux"
)

type routeTestCase struct {
	RequestURI string
	Vars       map[string]string
	RouteName  string
	StatusCode int
}

// TestRouter registers a test handler with all the routes and ensures that
// each route returns the expected path variables. Not method verification is
// present. This not meant to be exhaustive but as check to ensure that the
// expected variables are extracted.
//
// This may go away as the application structure comes together.
func TestRouter(t *testing.T) {

	router := v2APIRouter()

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

	for _, testcase := range []routeTestCase{
		{
			RouteName:  routeNameImageManifest,
			RequestURI: "/v2/foo/bar/image/tag",
			Vars: map[string]string{
				"name": "foo/bar",
				"tag":  "tag",
			},
		},
		{
			RouteName:  routeNameTags,
			RequestURI: "/v2/foo/bar/tags/list",
			Vars: map[string]string{
				"name": "foo/bar",
			},
		},
		{
			RouteName:  routeNameLayer,
			RequestURI: "/v2/foo/bar/layer/tarsum.dev+foo:abcdef0919234",
			Vars: map[string]string{
				"name":   "foo/bar",
				"tarsum": "tarsum.dev+foo:abcdef0919234",
			},
		},
		{
			RouteName:  routeNameLayerUpload,
			RequestURI: "/v2/foo/bar/layer/tarsum.dev+foo:abcdef0919234/upload/",
			Vars: map[string]string{
				"name":   "foo/bar",
				"tarsum": "tarsum.dev+foo:abcdef0919234",
			},
		},
		{
			RouteName:  routeNameLayerUploadResume,
			RequestURI: "/v2/foo/bar/layer/tarsum.dev+foo:abcdef0919234/upload/uuid",
			Vars: map[string]string{
				"name":   "foo/bar",
				"tarsum": "tarsum.dev+foo:abcdef0919234",
				"uuid":   "uuid",
			},
		},
		{
			RouteName:  routeNameLayerUploadResume,
			RequestURI: "/v2/foo/bar/layer/tarsum.dev+foo:abcdef0919234/upload/D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			Vars: map[string]string{
				"name":   "foo/bar",
				"tarsum": "tarsum.dev+foo:abcdef0919234",
				"uuid":   "D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			},
		},

		{
			// Check ambiguity: ensure we can distinguish between tags for
			// "foo/bar/image/image" and image for "foo/bar/image" with tag
			// "tags"
			RouteName:  routeNameImageManifest,
			RequestURI: "/v2/foo/bar/image/image/tags",
			Vars: map[string]string{
				"name": "foo/bar/image",
				"tag":  "tags",
			},
		},
		{
			// This case presents an ambiguity between foo/bar with tag="tags"
			// and list tags for "foo/bar/image"
			RouteName:  routeNameTags,
			RequestURI: "/v2/foo/bar/image/tags/list",
			Vars: map[string]string{
				"name": "foo/bar/image",
			},
		},
		{
			RouteName:  routeNameLayerUploadResume,
			RequestURI: "/v2/foo/../../layer/tarsum.dev+foo:abcdef0919234/upload/D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			Vars: map[string]string{
				"name":   "foo/bar",
				"tarsum": "tarsum.dev+foo:abcdef0919234",
				"uuid":   "D95306FA-FAD3-4E36-8D41-CF1C93EF8286",
			},
			StatusCode: http.StatusNotFound,
		},
	} {
		// Register the endpoint
		router.GetRoute(testcase.RouteName).Handler(testHandler)
		u := server.URL + testcase.RequestURI

		resp, err := http.Get(u)

		if err != nil {
			t.Fatalf("error issuing get request: %v", err)
		}

		if testcase.StatusCode == 0 {
			// Override default, zero-value
			testcase.StatusCode = http.StatusOK
		}

		if resp.StatusCode != testcase.StatusCode {
			t.Fatalf("unexpected status for %s: %v %v", u, resp.Status, resp.StatusCode)
		}

		if testcase.StatusCode != http.StatusOK {
			// We don't care about json response.
			continue
		}

		dec := json.NewDecoder(resp.Body)

		var actualRouteInfo routeTestCase
		if err := dec.Decode(&actualRouteInfo); err != nil {
			t.Fatalf("error reading json response: %v", err)
		}
		// Needs to be set out of band
		actualRouteInfo.StatusCode = resp.StatusCode

		if actualRouteInfo.RouteName != testcase.RouteName {
			t.Fatalf("incorrect route %q matched, expected %q", actualRouteInfo.RouteName, testcase.RouteName)
		}

		if !reflect.DeepEqual(actualRouteInfo, testcase) {
			t.Fatalf("actual does not equal expected: %#v != %#v", actualRouteInfo, testcase)
		}
	}

}

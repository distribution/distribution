package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gorilla/mux"
)

type routeInfo struct {
	RequestURI string
	Vars       map[string]string
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
		routeInfo := routeInfo{
			RequestURI: r.RequestURI,
			Vars:       mux.Vars(r),
		}

		enc := json.NewEncoder(w)

		if err := enc.Encode(routeInfo); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	// Startup test server
	server := httptest.NewServer(router)

	for _, testcase := range []struct {
		routeName         string
		expectedRouteInfo routeInfo
	}{
		{
			routeName: routeNameImageManifest,
			expectedRouteInfo: routeInfo{
				RequestURI: "/v2/foo/bar/image/tag",
				Vars: map[string]string{
					"name": "foo/bar",
					"tag":  "tag",
				},
			},
		},
		{
			routeName: routeNameTags,
			expectedRouteInfo: routeInfo{
				RequestURI: "/v2/foo/bar/tags",
				Vars: map[string]string{
					"name": "foo/bar",
				},
			},
		},
		{
			routeName: routeNameLayer,
			expectedRouteInfo: routeInfo{
				RequestURI: "/v2/foo/bar/layer/tarsum",
				Vars: map[string]string{
					"name":   "foo/bar",
					"tarsum": "tarsum",
				},
			},
		},
		{
			routeName: routeNameStartLayerUpload,
			expectedRouteInfo: routeInfo{
				RequestURI: "/v2/foo/bar/layer/tarsum/upload/",
				Vars: map[string]string{
					"name":   "foo/bar",
					"tarsum": "tarsum",
				},
			},
		},
		{
			routeName: routeNameLayerUpload,
			expectedRouteInfo: routeInfo{
				RequestURI: "/v2/foo/bar/layer/tarsum/upload/uuid",
				Vars: map[string]string{
					"name":   "foo/bar",
					"tarsum": "tarsum",
					"uuid":   "uuid",
				},
			},
		},
	} {
		// Register the endpoint
		router.GetRoute(testcase.routeName).Handler(testHandler)
		u := server.URL + testcase.expectedRouteInfo.RequestURI

		resp, err := http.Get(u)

		if err != nil {
			t.Fatalf("error issuing get request: %v", err)
		}

		dec := json.NewDecoder(resp.Body)

		var actualRouteInfo routeInfo
		if err := dec.Decode(&actualRouteInfo); err != nil {
			t.Fatalf("error reading json response: %v", err)
		}

		if !reflect.DeepEqual(actualRouteInfo, testcase.expectedRouteInfo) {
			t.Fatalf("actual does not equal expected: %v != %v", actualRouteInfo, testcase.expectedRouteInfo)
		}
	}

}

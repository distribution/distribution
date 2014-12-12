package registry

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/docker/docker-registry/api/urls"
	"github.com/docker/docker-registry/configuration"
)

// TestAppDispatcher builds an application with a test dispatcher and ensures
// that requests are properly dispatched and the handlers are constructed.
// This only tests the dispatch mechanism. The underlying dispatchers must be
// tested individually.
func TestAppDispatcher(t *testing.T) {
	app := &App{
		Config: configuration.Configuration{},
		router: urls.Router(),
	}
	server := httptest.NewServer(app)
	router := urls.Router()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("error parsing server url: %v", err)
	}

	varCheckingDispatcher := func(expectedVars map[string]string) dispatchFunc {
		return func(ctx *Context, r *http.Request) http.Handler {
			// Always checks the same name context
			if ctx.Name != ctx.vars["name"] {
				t.Fatalf("unexpected name: %q != %q", ctx.Name, "foo/bar")
			}

			// Check that we have all that is expected
			for expectedK, expectedV := range expectedVars {
				if ctx.vars[expectedK] != expectedV {
					t.Fatalf("unexpected %s in context vars: %q != %q", expectedK, ctx.vars[expectedK], expectedV)
				}
			}

			// Check that we only have variables that are expected
			for k, v := range ctx.vars {
				_, ok := expectedVars[k]

				if !ok { // name is checked on context
					// We have an unexpected key, fail
					t.Fatalf("unexpected key %q in vars with value %q", k, v)
				}
			}

			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
		}
	}

	// unflatten a list of variables, suitable for gorilla/mux, to a map[string]string
	unflatten := func(vars []string) map[string]string {
		m := make(map[string]string)
		for i := 0; i < len(vars)-1; i = i + 2 {
			m[vars[i]] = vars[i+1]
		}

		return m
	}

	for _, testcase := range []struct {
		endpoint string
		vars     []string
	}{
		{
			endpoint: urls.RouteNameManifest,
			vars: []string{
				"name", "foo/bar",
				"tag", "sometag",
			},
		},
		{
			endpoint: urls.RouteNameTags,
			vars: []string{
				"name", "foo/bar",
			},
		},
		{
			endpoint: urls.RouteNameBlob,
			vars: []string{
				"name", "foo/bar",
				"digest", "tarsum.v1+bogus:abcdef0123456789",
			},
		},
		{
			endpoint: urls.RouteNameBlobUpload,
			vars: []string{
				"name", "foo/bar",
			},
		},
		{
			endpoint: urls.RouteNameBlobUploadChunk,
			vars: []string{
				"name", "foo/bar",
				"uuid", "theuuid",
			},
		},
	} {
		app.register(testcase.endpoint, varCheckingDispatcher(unflatten(testcase.vars)))
		route := router.GetRoute(testcase.endpoint).Host(serverURL.Host)
		u, err := route.URL(testcase.vars...)

		if err != nil {
			t.Fatal(err)
		}

		resp, err := http.Get(u.String())

		if err != nil {
			t.Fatal(err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: %v != %v", resp.StatusCode, http.StatusOK)
		}
	}
}

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/auth"
	_ "github.com/distribution/distribution/v3/registry/auth/silly"
	"github.com/distribution/distribution/v3/registry/proxy"
	"github.com/distribution/distribution/v3/registry/storage"
	memorycache "github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

// TestAppDispatcher builds an application with a test dispatcher and ensures
// that requests are properly dispatched and the handlers are constructed.
// This only tests the dispatch mechanism. The underlying dispatchers must be
// tested individually.
func TestAppDispatcher(t *testing.T) {
	driver := inmemory.New()
	ctx := dcontext.Background()
	registry, err := storage.NewRegistry(ctx, driver, storage.BlobDescriptorCacheProvider(memorycache.NewInMemoryBlobDescriptorCacheProvider(0)), storage.EnableDelete, storage.EnableRedirect)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	app := &App{
		Config:   &configuration.Configuration{},
		Context:  ctx,
		router:   v2.Router(),
		driver:   driver,
		registry: registry,
	}
	server := httptest.NewServer(app)
	defer server.Close()
	router := v2.Router()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("error parsing server url: %v", err)
	}

	varCheckingDispatcher := func(expectedVars map[string]string) dispatchFunc {
		return func(ctx *Context, r *http.Request) http.Handler {
			// Always checks the same name context
			if ctx.Repository.Named().Name() != getName(ctx) {
				t.Fatalf("unexpected name: %q != %q", ctx.Repository.Named().Name(), "foo/bar")
			}

			// Check that we have all that is expected
			for expectedK, expectedV := range expectedVars {
				if ctx.Value(expectedK) != expectedV {
					t.Fatalf("unexpected %s in context vars: %q != %q", expectedK, ctx.Value(expectedK), expectedV)
				}
			}

			// Check that we only have variables that are expected
			for k, v := range ctx.Value("vars").(map[string]string) {
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
			endpoint: v2.RouteNameManifest,
			vars: []string{
				"name", "foo/bar",
				"reference", "sometag",
			},
		},
		{
			endpoint: v2.RouteNameTags,
			vars: []string{
				"name", "foo/bar",
			},
		},
		{
			endpoint: v2.RouteNameBlobUpload,
			vars: []string{
				"name", "foo/bar",
			},
		},
		{
			endpoint: v2.RouteNameBlobUploadChunk,
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
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: %v != %v", resp.StatusCode, http.StatusOK)
		}
	}
}

// TestNewApp covers the creation of an application via NewApp with a
// configuration.
func TestNewApp(t *testing.T) {
	ctx := dcontext.Background()
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": nil,
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
		Auth: configuration.Auth{
			// For now, we simply test that new auth results in a viable
			// application.
			"silly": {
				"realm":   "realm-test",
				"service": "service-test",
			},
		},
	}

	// Mostly, with this test, given a sane configuration, we are simply
	// ensuring that NewApp doesn't panic. We might want to tweak this
	// behavior.
	app := NewApp(ctx, &config)

	server := httptest.NewServer(app)
	defer server.Close()
	builder, err := v2.NewURLBuilderFromString(server.URL, false)
	if err != nil {
		t.Fatalf("error creating urlbuilder: %v", err)
	}

	baseURL, err := builder.BuildBaseURL()
	if err != nil {
		t.Fatalf("error creating baseURL: %v", err)
	}

	// TODO(stevvooe): The rest of this test might belong in the API tests.

	// Just hit the app and make sure we get a 401 Unauthorized error.
	req, err := http.Get(baseURL)
	if err != nil {
		t.Fatalf("unexpected error during GET: %v", err)
	}
	defer req.Body.Close()

	if req.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status code during request: %v", err)
	}

	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content-type: %v != %v", req.Header.Get("Content-Type"), "application/json")
	}

	expectedAuthHeader := "Bearer realm=\"realm-test\",service=\"service-test\""
	if e, a := expectedAuthHeader, req.Header.Get("WWW-Authenticate"); e != a {
		t.Fatalf("unexpected WWW-Authenticate header: %q != %q", e, a)
	}

	var errs errcode.Errors
	dec := json.NewDecoder(req.Body)
	if err := dec.Decode(&errs); err != nil {
		t.Fatalf("error decoding error response: %v", err)
	}

	err2, ok := errs[0].(errcode.ErrorCoder)
	if !ok {
		t.Fatalf("not an ErrorCoder: %#v", errs[0])
	}
	if err2.ErrorCode() != errcode.ErrorCodeUnauthorized {
		t.Fatalf("unexpected error code: %v != %v", err2.ErrorCode(), errcode.ErrorCodeUnauthorized)
	}
}

type testProxyCloser struct {
	distribution.Namespace
	closeCalled         bool
	ctxCancelledOnClose bool
	app                 *App
}

func (m *testProxyCloser) Close() error {
	m.closeCalled = true
	select {
	case <-m.app.Context.Done():
		m.ctxCancelledOnClose = true
	default:
		m.ctxCancelledOnClose = false
	}
	return nil
}

func TestAppShutdownStopsUploadPurger(t *testing.T) {
	ctx := dcontext.Background()
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"maintenance": configuration.Parameters{
				"uploadpurging": map[any]any{
					"enabled":  true,
					"age":      "168h",
					"interval": "24h",
					"dryrun":   false,
				},
			},
		},
	}
	app := NewApp(ctx, &config)
	if app.purgerDone == nil {
		t.Fatal("expected purgerDone channel to be initialized")
	}

	if err := app.Shutdown(); err != nil {
		t.Fatalf("unexpected error on Shutdown: %v", err)
	}

	select {
	case <-app.purgerDone:
		// upload purger goroutine exited
	case <-time.After(5 * time.Second):
		t.Fatal("upload purger goroutine did not exit within timeout after Shutdown")
	}

	select {
	case <-app.Context.Done():
	default:
		t.Fatal("expected app context to be cancelled after Shutdown")
	}
}

func TestAppShutdownDefersCancelAfterRegistryClose(t *testing.T) {
	ctx := dcontext.Background()
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
	}
	app := NewApp(ctx, &config)
	closer := &testProxyCloser{
		Namespace: app.registry,
		app:       app,
	}
	app.registry = closer

	if err := app.Shutdown(); err != nil {
		t.Fatalf("unexpected error on Shutdown: %v", err)
	}

	if !closer.closeCalled {
		t.Fatal("expected registry Close to be called")
	}
	if closer.ctxCancelledOnClose {
		t.Fatal("expected context NOT to be cancelled when registry Close is called")
	}
	select {
	case <-app.Context.Done():
	default:
		t.Fatal("expected app context to be cancelled after Shutdown completes")
	}
}

// Test the access record accumulator
func TestAppendAccessRecords(t *testing.T) {
	repo := "testRepo"

	expectedResource := auth.Resource{
		Type: "repository",
		Name: repo,
	}

	expectedPullRecord := auth.Access{
		Resource: expectedResource,
		Action:   "pull",
	}
	expectedPushRecord := auth.Access{
		Resource: expectedResource,
		Action:   "push",
	}
	expectedDeleteRecord := auth.Access{
		Resource: expectedResource,
		Action:   "delete",
	}

	records := []auth.Access{}
	result := appendAccessRecords(records, http.MethodGet, repo)
	expectedResult := []auth.Access{expectedPullRecord}
	if ok := reflect.DeepEqual(result, expectedResult); !ok {
		t.Fatal("Actual access record differs from expected")
	}

	records = []auth.Access{}
	result = appendAccessRecords(records, http.MethodHead, repo)
	expectedResult = []auth.Access{expectedPullRecord}
	if ok := reflect.DeepEqual(result, expectedResult); !ok {
		t.Fatal("Actual access record differs from expected")
	}

	records = []auth.Access{}
	result = appendAccessRecords(records, http.MethodPost, repo)
	expectedResult = []auth.Access{expectedPullRecord, expectedPushRecord}
	if ok := reflect.DeepEqual(result, expectedResult); !ok {
		t.Fatal("Actual access record differs from expected")
	}

	records = []auth.Access{}
	result = appendAccessRecords(records, http.MethodPut, repo)
	expectedResult = []auth.Access{expectedPullRecord, expectedPushRecord}
	if ok := reflect.DeepEqual(result, expectedResult); !ok {
		t.Fatal("Actual access record differs from expected")
	}

	records = []auth.Access{}
	result = appendAccessRecords(records, http.MethodPatch, repo)
	expectedResult = []auth.Access{expectedPullRecord, expectedPushRecord}
	if ok := reflect.DeepEqual(result, expectedResult); !ok {
		t.Fatal("Actual access record differs from expected")
	}

	records = []auth.Access{}
	result = appendAccessRecords(records, http.MethodDelete, repo)
	expectedResult = []auth.Access{expectedDeleteRecord}
	if ok := reflect.DeepEqual(result, expectedResult); !ok {
		t.Fatal("Actual access record differs from expected")
	}
}

// TestPullThroughCacheBlobTTLReclaimsStorage ensures that when the registry is
// configured as a pull through cache, an expired blob is unlinked from the
// repository and removed from the blob store.
func TestPullThroughCacheBlobTTLReclaimsStorage(t *testing.T) {
	ctx := dcontext.Background()

	content := []byte("cached layer content")
	dgst := digest.FromBytes(content)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/foo/bar/blobs/"+dgst.String() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Docker-Content-Digest", dgst.String())
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		if r.Method == http.MethodHead {
			return
		}
		w.Write(content)
	}))
	defer upstream.Close()

	ttl := 10 * time.Millisecond
	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": nil,
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{
				"enabled": false,
			}},
		},
	}
	config.Proxy.RemoteURL = upstream.URL
	config.Proxy.TTL = &ttl

	app := NewApp(ctx, &config)
	defer func() {
		if closer, ok := app.registry.(proxy.Closer); ok {
			closer.Close()
		}
	}()

	name, err := reference.WithName("foo/bar")
	if err != nil {
		t.Fatalf("error parsing repository name: %v", err)
	}

	repo, err := app.registry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("error getting repository: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v2/foo/bar/blobs/"+dgst.String(), nil)
	if err := repo.Blobs(ctx).ServeBlob(ctx, httptest.NewRecorder(), req, dgst); err != nil {
		t.Fatalf("error serving blob: %v", err)
	}

	if err := repo.Blobs(ctx).Delete(ctx, dgst); !errors.Is(err, distribution.ErrUnsupported) {
		t.Fatalf("expected a client-initiated blob delete on a cache to be unsupported, got: %v", err)
	}

	paths := []string{
		"/docker/registry/v2/repositories/foo/bar/_layers/sha256/" + dgst.Encoded() + "/link",
		"/docker/registry/v2/blobs/sha256/" + dgst.Encoded()[:2] + "/" + dgst.Encoded() + "/data",
	}
	for _, p := range paths {
		if _, err := app.driver.Stat(ctx, p); err != nil {
			t.Fatalf("expected %s to exist after caching the blob: %v", p, err)
		}
	}

	for _, p := range paths {
		removed := false
		for i := 0; i < 100; i++ {
			if _, err := app.driver.Stat(ctx, p); err != nil {
				removed = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !removed {
			t.Errorf("expected %s to be removed once the blob TTL expired", p)
		}
	}
}

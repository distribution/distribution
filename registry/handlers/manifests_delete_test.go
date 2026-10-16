package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// deleteFaultDriver uses real in-memory storage; hooks affect individual storage
// operations rather than replacing manifest or tag service semantics.
type deleteFaultDriver struct {
	driver.StorageDriver
	mu         sync.Mutex
	beforeGet  func(context.Context, string) error
	beforeList func(context.Context, string) error
	deleteHook func(context.Context, string) (bool, error)
	deletes    []string
}

func (d *deleteFaultDriver) GetContent(ctx context.Context, path string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.beforeGet != nil {
		if err := d.beforeGet(ctx, path); err != nil {
			return nil, err
		}
	}
	return d.StorageDriver.GetContent(ctx, path)
}

func (d *deleteFaultDriver) List(ctx context.Context, path string) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.beforeList != nil {
		if err := d.beforeList(ctx, path); err != nil {
			return nil, err
		}
	}
	return d.StorageDriver.List(ctx, path)
}

func (d *deleteFaultDriver) Delete(ctx context.Context, path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deletes = append(d.deletes, path)
	if d.deleteHook != nil {
		if handled, err := d.deleteHook(ctx, path); handled {
			return err
		}
	}
	return d.StorageDriver.Delete(ctx, path)
}

type manifestDeleteEnv struct {
	app        *App
	driver     *deleteFaultDriver
	repository distribution.Repository
	manifests  distribution.ManifestService
	digest     digest.Digest
	other      digest.Digest
}

func newManifestDeleteEnv(t *testing.T, deleteEnabled bool) *manifestDeleteEnv {
	t.Helper()
	ctx := context.Background()
	app := NewApp(ctx, &configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory":    configuration.Parameters{},
			"delete":      configuration.Parameters{"enabled": deleteEnabled},
			"maintenance": configuration.Parameters{"uploadpurging": map[any]any{"enabled": false}},
		},
	})
	d := &deleteFaultDriver{StorageDriver: app.driver}
	var options []storage.RegistryOption
	if deleteEnabled {
		options = append(options, storage.EnableDelete)
	}
	registry, err := storage.NewRegistry(ctx, d, options...)
	if err != nil {
		t.Fatal(err)
	}
	app.registry = registry
	app.driver = d
	name, err := reference.WithName("delete/retry")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := registry.Repository(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	put := func(label string) digest.Digest {
		var index ocischema.DeserializedImageIndex
		if err := json.Unmarshal([]byte(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[],"annotations":{"test":"`+label+`"}}`), &index); err != nil {
			t.Fatal(err)
		}
		dgst, err := ms.Put(ctx, &index)
		if err != nil {
			t.Fatal(err)
		}
		return dgst
	}
	env := &manifestDeleteEnv{app: app, driver: d, repository: repo, manifests: ms, digest: put("target"), other: put("other")}
	for tag, dgst := range map[string]digest.Digest{"a": env.digest, "b": env.digest, "other": env.other} {
		if err := repo.Tags(ctx).Tag(ctx, tag, v1.Descriptor{Digest: dgst}); err != nil {
			t.Fatal(err)
		}
	}
	return env
}

func (e *manifestDeleteEnv) request(t *testing.T, ref string, status int, code errcode.ErrorCode) {
	t.Helper()
	w := httptest.NewRecorder()
	e.app.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v2/delete/retry/manifests/"+ref, nil))
	if w.Code != status {
		t.Fatalf("DELETE %s: status=%d, want %d, body=%s", ref, w.Code, status, w.Body)
	}
	if status == http.StatusAccepted && w.Body.Len() != 0 {
		t.Fatalf("accepted response contains an error body: %s", w.Body)
	}
	if code != 0 {
		var body struct {
			Errors []struct {
				Code string `json:"code"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Errors) != 1 || body.Errors[0].Code != code.String() {
			t.Fatalf("expected %s, got %s", code, w.Body)
		}
	}
}

func (e *manifestDeleteEnv) checkState(t *testing.T, exists bool, tags map[string]digest.Digest) {
	t.Helper()
	ctx := context.Background()
	got, err := e.manifests.Exists(ctx, e.digest)
	if err != nil || got != exists {
		t.Fatalf("manifest exists=%v, err=%v; want %v", got, err, exists)
	}
	for _, tag := range []string{"a", "b", "other"} {
		desc, err := e.repository.Tags(ctx).Get(ctx, tag)
		want, present := tags[tag]
		if present {
			if err != nil || desc.Digest != want {
				t.Fatalf("tag %s: %s, %v; want %s", tag, desc.Digest, err, want)
			}
		} else {
			var unknown distribution.ErrTagUnknown
			if !errors.As(err, &unknown) {
				t.Fatalf("tag %s: expected absent, got %s, %v", tag, desc.Digest, err)
			}
		}
	}
	if exists, err := e.manifests.Exists(ctx, e.other); err != nil || !exists {
		t.Fatalf("unrelated manifest lost: %v, %v", exists, err)
	}
}

// Cancellation hooks return no error: real storage operations may finish after
// cancellation, so a nil storage error does not establish successful cleanup.
func TestManifestDeleteCancellation(t *testing.T) {
	for _, phase := range []string{"control", "lookup", "cleanup"} {
		t.Run(phase, func(t *testing.T) {
			e := newManifestDeleteEnv(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			seen := false
			e.driver.beforeList = func(_ context.Context, path string) error {
				if strings.HasSuffix(path, "/tags") && phase != "cleanup" {
					seen = true
					if phase == "lookup" {
						cancel()
					}
				}
				return nil
			}
			e.driver.deleteHook = func(_ context.Context, path string) (bool, error) {
				if phase == "cleanup" && strings.HasSuffix(path, "/tags/b") {
					seen = true
					cancel()
				}
				return false, nil
			}
			w := httptest.NewRecorder()
			e.app.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v2/delete/retry/manifests/"+e.digest.String(), nil).WithContext(ctx))
			e.driver.beforeList, e.driver.deleteHook = nil, nil
			if !seen {
				t.Fatal("storage cancellation boundary was not exercised")
			}
			if phase == "control" {
				if w.Code != http.StatusAccepted || w.Body.Len() != 0 {
					t.Fatalf("control: status=%d body=%s", w.Code, w.Body)
				}
				e.checkState(t, false, map[string]digest.Digest{"other": e.other})
				return
			}
			if ctx.Err() != context.Canceled || w.Code != http.StatusInternalServerError {
				t.Errorf("cancelled cleanup: context=%v status=%d body=%s", ctx.Err(), w.Code, w.Body)
			}
			for _, path := range e.driver.deletes {
				if strings.Contains(path, "/revisions/") || phase == "lookup" {
					t.Errorf("cancelled request attempted deletion: %s", path)
				}
			}
			remaining := map[string]digest.Digest{"other": e.other}
			if phase == "lookup" {
				remaining["a"], remaining["b"] = e.digest, e.digest
			}
			e.checkState(t, true, remaining)
			e.request(t, e.digest.String(), http.StatusAccepted, 0)
			e.checkState(t, false, map[string]digest.Digest{"other": e.other})
		})
	}
}

func TestManifestDeleteStorageFailures(t *testing.T) {
	storageErr := errors.New("injected storage failure")
	for _, phase := range []string{"validation", "lookup-list", "lookup-link", "tag-get", "untag", "untag-applied", "manifest-delete", "manifest-delete-applied"} {
		t.Run(phase, func(t *testing.T) {
			e := newManifestDeleteEnv(t, true)
			gets := 0
			e.driver.beforeGet = func(_ context.Context, path string) error {
				if phase == "validation" && strings.Contains(path, "/revisions/") {
					return storageErr
				}
				if strings.HasSuffix(path, "/tags/b/current/link") {
					gets++
					if phase == "lookup-link" || (phase == "tag-get" && gets == 2) {
						return storageErr
					}
				}
				return nil
			}
			e.driver.beforeList = func(_ context.Context, path string) error {
				if phase == "lookup-list" && strings.HasSuffix(path, "/tags") {
					return storageErr
				}
				return nil
			}
			e.driver.deleteHook = func(ctx context.Context, path string) (bool, error) {
				if strings.HasSuffix(path, "/tags/b") {
					switch phase {
					case "untag":
						return true, storageErr
					case "untag-applied":
						if err := e.driver.StorageDriver.Delete(ctx, path); err != nil {
							return true, err
						}
						return true, storageErr
					}
				}
				if strings.Contains(path, "/revisions/") {
					switch phase {
					case "manifest-delete":
						return true, storageErr
					case "manifest-delete-applied":
						if err := e.driver.StorageDriver.Delete(ctx, path); err != nil {
							return true, err
						}
						return true, storageErr
					}
				}
				return false, nil
			}
			e.request(t, e.digest.String(), http.StatusInternalServerError, errcode.ErrorCodeUnknown)
			e.driver.beforeGet, e.driver.beforeList, e.driver.deleteHook = nil, nil, nil
			remaining := map[string]digest.Digest{"other": e.other}
			switch phase {
			case "validation", "lookup-list", "lookup-link":
				remaining["a"], remaining["b"] = e.digest, e.digest
			case "tag-get", "untag":
				remaining["b"] = e.digest
			}
			e.checkState(t, phase != "manifest-delete-applied", remaining)
			if phase == "manifest-delete-applied" {
				// A backend can report failure after applying Delete. Tags must
				// already be gone even though retry correctly returns unknown.
				e.request(t, e.digest.String(), http.StatusNotFound, errcode.ErrorCodeManifestUnknown)
			} else {
				e.request(t, e.digest.String(), http.StatusAccepted, 0)
			}
			e.checkState(t, false, map[string]digest.Digest{"other": e.other})
		})
	}
}

func TestManifestDeleteTagChanges(t *testing.T) {
	for _, change := range []string{"retarget-before-get", "disappear-before-get", "disappear-before-untag", "missing-current-link"} {
		t.Run(change, func(t *testing.T) {
			e := newManifestDeleteEnv(t, true)
			gets := 0
			changed := false
			e.driver.beforeGet = func(ctx context.Context, path string) error {
				if !strings.HasSuffix(path, "/tags/b/current/link") {
					return nil
				}
				gets++
				if change == "missing-current-link" && gets == 1 {
					changed = true
					return e.driver.StorageDriver.Delete(ctx, path)
				}
				if gets != 2 {
					return nil
				}
				switch change {
				case "retarget-before-get":
					changed = true
					return e.driver.StorageDriver.PutContent(ctx, path, []byte(e.other))
				case "disappear-before-get":
					changed = true
					return e.driver.StorageDriver.Delete(ctx, strings.TrimSuffix(path, "/current/link"))
				}
				return nil
			}
			e.driver.deleteHook = func(ctx context.Context, path string) (bool, error) {
				if change == "disappear-before-untag" && strings.HasSuffix(path, "/tags/b") {
					changed = true
					if err := e.driver.StorageDriver.Delete(ctx, path); err != nil {
						return true, err
					}
					// Let the second Delete return the real PathNotFoundError.
				}
				return false, nil
			}
			e.request(t, e.digest.String(), http.StatusAccepted, 0)
			e.driver.beforeGet, e.driver.deleteHook = nil, nil
			if !changed {
				t.Fatal("storage change was not exercised")
			}
			remaining := map[string]digest.Digest{"other": e.other}
			if change == "retarget-before-get" {
				remaining["b"] = e.other
			}
			e.checkState(t, false, remaining)
			if change == "retarget-before-get" {
				for _, path := range e.driver.deletes {
					if strings.HasSuffix(path, "/tags/b") {
						t.Fatal("retargeted tag passed to Untag")
					}
				}
			}
		})
	}
}

func TestManifestDeleteValidation(t *testing.T) {
	for _, mode := range []string{"disabled", "readonly", "cache", "unknown", "invalid", "unsupported-driver", "readonly-driver"} {
		t.Run(mode, func(t *testing.T) {
			e := newManifestDeleteEnv(t, mode != "disabled")
			ref := e.digest.String()
			status, code := http.StatusMethodNotAllowed, errcode.ErrorCodeUnsupported
			exists := true
			switch mode {
			case "readonly":
				e.app.readOnly = true
				code = 0 // MethodHandler emits no registry JSON.
			case "cache":
				e.app.isCache = true
			case "unknown":
				if err := e.manifests.Delete(context.Background(), e.digest); err != nil {
					t.Fatal(err)
				}
				e.driver.deletes = nil
				exists = false
				status, code = http.StatusNotFound, errcode.ErrorCodeManifestUnknown
			case "invalid":
				ref = "sha256:not-a-digest"
				status, code = http.StatusBadRequest, errcode.ErrorCodeDigestInvalid
			case "unsupported-driver", "readonly-driver":
				err := distribution.ErrUnsupported
				if mode == "readonly-driver" {
					err = errors.New("storage permission denied")
					status, code = http.StatusInternalServerError, errcode.ErrorCodeUnknown
				}
				e.driver.deleteHook = func(context.Context, string) (bool, error) { return true, err }
			}
			e.request(t, ref, status, code)
			e.driver.deleteHook = nil
			e.checkState(t, exists, map[string]digest.Digest{"a": e.digest, "b": e.digest, "other": e.other})
			if mode != "unsupported-driver" && mode != "readonly-driver" && len(e.driver.deletes) != 0 {
				t.Fatalf("validation attempted storage deletion: %v", e.driver.deletes)
			}
		})
	}
}

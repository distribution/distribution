package decorator

import (
	"io"
	"testing"

	"github.com/docker/libtrust"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storage"
	"github.com/docker/distribution/storagedriver/inmemory"
	"github.com/docker/distribution/testutil"
)

func TestRegistryDecorator(t *testing.T) {
	// Initialize the expected decorations. Call counting is a horrible way to
	// test this but should keep this code from being atrocious.
	expected := map[string]int{
		"repository":      1,
		"manifestservice": 1,
		"layerservice":    1,
		"layer":           4,
		"layerupload":     4,
	}
	decorated := map[string]int{}

	decorator := Func(func(v interface{}) interface{} {
		switch v := v.(type) {
		case storage.Repository:
			t.Logf("decorate repository: %T", v)
			decorated["repository"]++
		case storage.ManifestService:
			t.Logf("decorate manifestservice: %T", v)
			decorated["manifestservice"]++
		case storage.LayerService:
			t.Logf("decorate layerservice: %T", v)
			decorated["layerservice"]++
		case storage.Layer:
			t.Logf("decorate layer: %T", v)
			decorated["layer"]++
		case storage.LayerUpload:
			t.Logf("decorate layerupload: %T", v)
			decorated["layerupload"]++
		default:
			t.Fatalf("unexpected object decorated: %v", v)
		}

		return v
	})

	registry := storage.NewRegistryWithDriver(inmemory.New())
	registry = DecorateRegistry(registry, decorator)

	// Now take the registry through a number of operations
	checkExerciseRegistry(t, registry)

	for component, calls := range expected {
		if decorated[component] != calls {
			t.Fatalf("%v was not decorated expected number of times: %d != %d", component, decorated[component], calls)
		}
	}

}

// checkExerciseRegistry takes the registry through all of its operations,
// carrying out generic checks.
func checkExerciseRegistry(t *testing.T, registry storage.Registry) {
	name := "foo/bar"
	tag := "thetag"
	repository := registry.Repository(name)
	m := manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: name,
		Tag:  tag,
	}

	layers := repository.Layers()
	for i := 0; i < 2; i++ {
		rs, ds, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("error creating test layer: %v", err)
		}
		dgst := digest.Digest(ds)
		upload, err := layers.Upload()
		if err != nil {
			t.Fatalf("error creating layer upload: %v", err)
		}

		// Use the resumes, as well!
		upload, err = layers.Resume(upload.UUID())
		if err != nil {
			t.Fatalf("error resuming layer upload: %v", err)
		}

		io.Copy(upload, rs)

		if _, err := upload.Finish(dgst); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}

		m.FSLayers = append(m.FSLayers, manifest.FSLayer{
			BlobSum: dgst,
		})

		// Then fetch the layers
		if _, err := layers.Fetch(dgst); err != nil {
			t.Fatalf("error fetching layer: %v", err)
		}
	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating key: %v", err)
	}

	sm, err := manifest.Sign(&m, pk)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}

	manifests := repository.Manifests()

	if err := manifests.Put(tag, sm); err != nil {
		t.Fatalf("unexpected error putting the manifest: %v", err)
	}

	fetched, err := manifests.Get(tag)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	if fetched.Tag != fetched.Tag {
		t.Fatalf("retrieved unexpected manifest: %v", err)
	}
}

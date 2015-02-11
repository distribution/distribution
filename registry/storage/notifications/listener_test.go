package notifications

import (
	"io"
	"reflect"
	"testing"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/storagedriver/inmemory"
	"github.com/docker/distribution/testutil"
	"github.com/docker/libtrust"
	"golang.org/x/net/context"
)

func TestListener(t *testing.T) {
	registry := storage.NewRegistryWithDriver(inmemory.New())
	tl := &testListener{
		ops: make(map[string]int),
	}
	ctx := context.Background()
	repository := Listen(registry.Repository(ctx, "foo/bar"), tl)

	// Now take the registry through a number of operations
	checkExerciseRepository(t, repository)

	expectedOps := map[string]int{
		"manifest:push": 1,
		"manifest:pull": 1,
		// "manifest:delete": 0, // deletes not supported for now
		"layer:push": 2,
		"layer:pull": 2,
		// "layer:delete":    0, // deletes not supported for now
	}

	if !reflect.DeepEqual(tl.ops, expectedOps) {
		t.Fatalf("counts do not match:\n%v\n !=\n%v", tl.ops, expectedOps)
	}

}

type testListener struct {
	ops map[string]int
}

func (tl *testListener) ManifestPushed(repo storage.Repository, sm *manifest.SignedManifest) error {
	tl.ops["manifest:push"]++

	return nil
}

func (tl *testListener) ManifestPulled(repo storage.Repository, sm *manifest.SignedManifest) error {
	tl.ops["manifest:pull"]++
	return nil
}

func (tl *testListener) ManifestDeleted(repo storage.Repository, sm *manifest.SignedManifest) error {
	tl.ops["manifest:delete"]++
	return nil
}

func (tl *testListener) LayerPushed(repo storage.Repository, layer storage.Layer) error {
	tl.ops["layer:push"]++
	return nil
}

func (tl *testListener) LayerPulled(repo storage.Repository, layer storage.Layer) error {
	tl.ops["layer:pull"]++
	return nil
}

func (tl *testListener) LayerDeleted(repo storage.Repository, layer storage.Layer) error {
	tl.ops["layer:delete"]++
	return nil
}

// checkExerciseRegistry takes the registry through all of its operations,
// carrying out generic checks.
func checkExerciseRepository(t *testing.T, repository storage.Repository) {
	// TODO(stevvooe): This would be a nice testutil function. Basically, it
	// takes the registry through a common set of operations. This could be
	// used to make cross-cutting updates by changing internals that affect
	// update counts. Basically, it would make writing tests a lot easier.

	tag := "thetag"
	m := manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: repository.Name(),
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

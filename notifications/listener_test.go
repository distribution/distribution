package notifications

import (
	"io"
	"reflect"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/testutil"
	"github.com/docker/libtrust"
)

func TestListener(t *testing.T) {
	ctx := context.Background()
	registry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider(), true, true, false)
	tl := &testListener{
		ops: make(map[string]int),
	}

	repository, err := registry.Repository(ctx, "foo/bar")
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	repository = Listen(repository, tl)

	// Now take the registry through a number of operations
	checkExerciseRepository(t, repository)

	expectedOps := map[string]int{
		"manifest:push": 1,
		"manifest:pull": 2,
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

func (tl *testListener) ManifestPushed(repo string, sm *manifest.SignedManifest) error {
	tl.ops["manifest:push"]++

	return nil
}

func (tl *testListener) ManifestPulled(repo string, sm *manifest.SignedManifest) error {
	tl.ops["manifest:pull"]++
	return nil
}

func (tl *testListener) ManifestDeleted(repo string, sm *manifest.SignedManifest) error {
	tl.ops["manifest:delete"]++
	return nil
}

func (tl *testListener) BlobPushed(repo string, desc distribution.Descriptor) error {
	tl.ops["layer:push"]++
	return nil
}

func (tl *testListener) BlobPulled(repo string, desc distribution.Descriptor) error {
	tl.ops["layer:pull"]++
	return nil
}

func (tl *testListener) BlobDeleted(repo string, desc distribution.Descriptor) error {
	tl.ops["layer:delete"]++
	return nil
}

// checkExerciseRegistry takes the registry through all of its operations,
// carrying out generic checks.
func checkExerciseRepository(t *testing.T, repository distribution.Repository) {
	// TODO(stevvooe): This would be a nice testutil function. Basically, it
	// takes the registry through a common set of operations. This could be
	// used to make cross-cutting updates by changing internals that affect
	// update counts. Basically, it would make writing tests a lot easier.
	ctx := context.Background()
	tag := "thetag"
	m := manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: repository.Name(),
		Tag:  tag,
	}

	blobs := repository.Blobs(ctx)
	for i := 0; i < 2; i++ {
		rs, ds, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("error creating test layer: %v", err)
		}
		dgst := digest.Digest(ds)

		wr, err := blobs.Create(ctx)
		if err != nil {
			t.Fatalf("error creating layer upload: %v", err)
		}

		// Use the resumes, as well!
		wr, err = blobs.Resume(ctx, wr.ID())
		if err != nil {
			t.Fatalf("error resuming layer upload: %v", err)
		}

		io.Copy(wr, rs)

		if _, err := wr.Commit(ctx, distribution.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}

		m.FSLayers = append(m.FSLayers, manifest.FSLayer{
			BlobSum: dgst,
		})

		// Then fetch the blobs
		if rc, err := blobs.Open(ctx, dgst); err != nil {
			t.Fatalf("error fetching layer: %v", err)
		} else {
			defer rc.Close()
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

	manifests, err := repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err = manifests.Put(sm); err != nil {
		t.Fatalf("unexpected error putting the manifest: %v", err)
	}

	p, err := sm.Payload()
	if err != nil {
		t.Fatalf("unexpected error getting manifest payload: %v", err)
	}

	dgst, err := digest.FromBytes(p)
	if err != nil {
		t.Fatalf("unexpected error digesting manifest payload: %v", err)
	}

	fetchedByManifest, err := manifests.Get(dgst)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	if fetchedByManifest.Tag != sm.Tag {
		t.Fatalf("retrieved unexpected manifest: %v", err)
	}

	fetched, err := manifests.GetByTag(tag)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	if fetched.Tag != fetchedByManifest.Tag {
		t.Fatalf("retrieved unexpected manifest: %v", err)
	}
}

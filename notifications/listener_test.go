package notifications

import (
	"io"
	"reflect"
	"testing"

	"github.com/distribution/reference"
	"github.com/docker/libtrust"
	"github.com/goharbor/distribution"
	"github.com/goharbor/distribution/context"
	"github.com/goharbor/distribution/manifest"
	"github.com/goharbor/distribution/manifest/schema1"
	"github.com/goharbor/distribution/registry/storage"
	"github.com/goharbor/distribution/registry/storage/cache/memory"
	"github.com/goharbor/distribution/registry/storage/driver/inmemory"
	"github.com/goharbor/distribution/testutil"
)

func TestListener(t *testing.T) {
	ctx := context.Background()
	k, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatal(err)
	}

	registry, err := storage.NewRegistry(ctx, inmemory.New(), storage.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider()), storage.EnableDelete, storage.EnableRedirect, storage.Schema1SigningKey(k), storage.EnableSchema1)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	tl := &testListener{
		ops: make(map[string]int),
	}

	repoRef, _ := reference.WithName("foo/bar")
	repository, err := registry.Repository(ctx, repoRef)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	remover, ok := registry.(distribution.RepositoryRemover)
	if !ok {
		t.Fatal("registry does not implement RepositoryRemover")
	}
	repository, remover = Listen(repository, remover, tl)

	// Now take the registry through a number of operations
	checkExerciseRepository(t, repository, remover)

	expectedOps := map[string]int{
		"manifest:push":   1,
		"manifest:pull":   1,
		"manifest:delete": 1,
		"layer:push":      2,
		"layer:pull":      2,
		"layer:delete":    2,
		"tag:delete":      1,
		"repo:delete":     1,
	}

	if !reflect.DeepEqual(tl.ops, expectedOps) {
		t.Fatalf("counts do not match:\n%v\n !=\n%v", tl.ops, expectedOps)
	}
}

type testListener struct {
	ops map[string]int
}

func (tl *testListener) ManifestPushed(repo reference.Named, m distribution.Manifest, options ...distribution.ManifestServiceOption) error {
	tl.ops["manifest:push"]++
	return nil
}

func (tl *testListener) ManifestPulled(repo reference.Named, m distribution.Manifest, options ...distribution.ManifestServiceOption) error {
	tl.ops["manifest:pull"]++
	return nil
}

func (tl *testListener) ManifestDeleted(repo reference.Named, d digest.Digest) error {
	tl.ops["manifest:delete"]++
	return nil
}

func (tl *testListener) BlobPushed(repo reference.Named, desc distribution.Descriptor) error {
	tl.ops["layer:push"]++
	return nil
}

func (tl *testListener) BlobPulled(repo reference.Named, desc distribution.Descriptor) error {
	tl.ops["layer:pull"]++
	return nil
}

func (tl *testListener) BlobMounted(repo reference.Named, desc distribution.Descriptor, fromRepo reference.Named) error {
	tl.ops["layer:mount"]++
	return nil
}

func (tl *testListener) BlobDeleted(repo reference.Named, d digest.Digest) error {
	tl.ops["layer:delete"]++
	return nil
}

func (tl *testListener) TagDeleted(repo reference.Named, tag string) error {
	tl.ops["tag:delete"]++
	return nil
}

func (tl *testListener) RepoDeleted(repo reference.Named) error {
	tl.ops["repo:delete"]++
	return nil
}

// checkExerciseRegistry takes the registry through all of its operations,
// carrying out generic checks.
func checkExerciseRepository(t *testing.T, repository distribution.Repository, remover distribution.RepositoryRemover) {
	// TODO(stevvooe): This would be a nice testutil function. Basically, it
	// takes the registry through a common set of operations. This could be
	// used to make cross-cutting updates by changing internals that affect
	// update counts. Basically, it would make writing tests a lot easier.

	ctx := context.Background()
	tag := "thetag"
	// todo: change this to use Builder

	m := schema1.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: repository.Named().Name(),
		Tag:  tag,
	}

	var blobDigests []digest.Digest
	blobs := repository.Blobs(ctx)
	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("error creating test layer: %v", err)
		}
		blobDigests = append(blobDigests, dgst)

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

		m.FSLayers = append(m.FSLayers, schema1.FSLayer{
			BlobSum: dgst,
		})
		m.History = append(m.History, schema1.History{
			V1Compatibility: "",
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

	sm, err := schema1.Sign(&m, pk)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}

	manifests, err := repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	var digestPut digest.Digest
	if digestPut, err = manifests.Put(ctx, sm); err != nil {
		t.Fatalf("unexpected error putting the manifest: %v", err)
	}

	dgst := digest.FromBytes(sm.Canonical)
	if dgst != digestPut {
		t.Fatalf("mismatching digest from payload and put")
	}

	_, err = manifests.Get(ctx, dgst)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	err = manifests.Delete(ctx, dgst)
	if err != nil {
		t.Fatalf("unexpected error deleting blob: %v", err)
	}

	for _, d := range blobDigests {
		err = blobs.Delete(ctx, d)
		if err != nil {
			t.Fatalf("unexpected error deleting blob: %v", err)
		}
	}

	err = repository.Tags(ctx).Untag(ctx, m.Tag)
	if err != nil {
		t.Fatalf("unexpected error deleting tag: %v", err)
	}

	err = remover.Remove(ctx, repository.Named())
	if err != nil {
		t.Fatalf("unexpected error deleting repo: %v", err)
	}
}

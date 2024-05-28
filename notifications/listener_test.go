package notifications

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

func TestListener(t *testing.T) {
	ctx := dcontext.Background()

	registry, err := storage.NewRegistry(ctx, inmemory.New(),
		storage.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)),
		storage.EnableDelete, storage.EnableRedirect)
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
	checkTestRepository(t, repository, remover)

	expectedOps := map[string]int{
		"manifest:push":   1,
		"manifest:pull":   1,
		"manifest:delete": 1,
		"layer:push":      3,
		"layer:pull":      3,
		"layer:delete":    3,
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

// checkTestRepository takes the registry through all of its operations,
// carrying out generic checks.
func checkTestRepository(t *testing.T, repository distribution.Repository, remover distribution.RepositoryRemover) {
	// TODO(stevvooe): This would be a nice testutil function. Basically, it
	// takes the registry through a common set of operations. This could be
	// used to make cross-cutting updates by changing internals that affect
	// update counts. Basically, it would make writing tests a lot easier.
	// TODO: change this to use Builder

	ctx := dcontext.Background()
	tag := "thetag"

	config := []byte(`{"name": "foo"}`)
	configDgst := digest.FromBytes(config)
	configReader := bytes.NewReader(config)

	var blobDigests []digest.Digest
	blobDigests = append(blobDigests, configDgst)

	blobs := repository.Blobs(ctx)

	// push config blob
	if err := testutil.PushBlob(ctx, repository, configReader, configDgst); err != nil {
		t.Fatal(err)
	}

	// Then fetch the config blob
	if rc, err := blobs.Open(ctx, configDgst); err != nil {
		t.Fatalf("error fetching config: %v", err)
	} else {
		defer rc.Close()
	}

	m := schema2.Manifest{
		Versioned: schema2.SchemaVersion,
		Config: distribution.Descriptor{
			MediaType: "foo/bar",
			Digest:    configDgst,
		},
	}

	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("error creating test layer: %v", err)
		}
		blobDigests = append(blobDigests, dgst)

		if err := testutil.PushBlob(ctx, repository, rs, dgst); err != nil {
			t.Fatal(err)
		}

		m.Layers = append(m.Layers, distribution.Descriptor{
			MediaType: "application/octet-stream",
			Digest:    dgst,
		})

		// Then fetch the blobs
		if rc, err := blobs.Open(ctx, dgst); err != nil {
			t.Fatalf("error fetching layer: %v", err)
		} else {
			defer rc.Close()
		}
	}

	sm, err := schema2.FromStruct(m)
	if err != nil {
		t.Fatal(err.Error())
	}

	manifests, err := repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	var digestPut digest.Digest
	if digestPut, err = manifests.Put(ctx, sm); err != nil {
		t.Fatalf("unexpected error putting the manifest: %v", err)
	}

	_, canonical, err := sm.Payload()
	if err != nil {
		t.Fatal(err.Error())
	}

	dgst := digest.FromBytes(canonical)
	if dgst != digestPut {
		t.Fatalf("mismatching digest from payload and put")
	}

	if err := repository.Tags(ctx).Tag(ctx, tag, distribution.Descriptor{Digest: dgst}); err != nil {
		t.Fatalf("unexpected error tagging manifest: %v", err)
	}

	_, err = manifests.Get(ctx, dgst)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	err = repository.Tags(ctx).Untag(ctx, tag)
	if err != nil {
		t.Fatalf("unexpected error deleting tag: %v", err)
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

	err = remover.Remove(ctx, repository.Named())
	if err != nil {
		t.Fatalf("unexpected error deleting repo: %v", err)
	}
}

package storage

import (
	"context"
	"reflect"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	digest "github.com/opencontainers/go-digest"
)

type tagsTestEnv struct {
	ts  distribution.TagService
	bs  distribution.BlobStore
	ms  distribution.ManifestService
	gbs distribution.BlobStatter
	ctx context.Context
}

func testTagStore(t *testing.T) *tagsTestEnv {
	ctx := context.Background()
	d := inmemory.New()
	reg, err := NewRegistry(ctx, d)
	if err != nil {
		t.Fatal(err)
	}

	repoRef, _ := reference.WithName("a/b")
	repo, err := reg.Repository(ctx, repoRef)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := repo.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}

	return &tagsTestEnv{
		ctx: ctx,
		ts:  repo.Tags(ctx),
		bs:  repo.Blobs(ctx),
		gbs: reg.BlobStatter(),
		ms:  ms,
	}
}

func TestTagStoreTag(t *testing.T) {
	env := testTagStore(t)
	tags := env.ts
	ctx := env.ctx

	d := distribution.Descriptor{}
	err := tags.Tag(ctx, "latest", d)
	if err == nil {
		t.Errorf("unexpected error putting malformed descriptor : %s", err)
	}

	d.Digest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	err = tags.Tag(ctx, "latest", d)
	if err != nil {
		t.Error(err)
	}

	d1, err := tags.Get(ctx, "latest")
	if err != nil {
		t.Error(err)
	}

	if d1.Digest != d.Digest {
		t.Error("put and get digest differ")
	}

	// Overwrite existing
	d.Digest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	err = tags.Tag(ctx, "latest", d)
	if err != nil {
		t.Error(err)
	}

	d1, err = tags.Get(ctx, "latest")
	if err != nil {
		t.Error(err)
	}

	if d1.Digest != d.Digest {
		t.Error("put and get digest differ")
	}
}

func TestTagStoreUnTag(t *testing.T) {
	env := testTagStore(t)
	tags := env.ts
	ctx := env.ctx
	desc := distribution.Descriptor{Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}

	err := tags.Untag(ctx, "latest")
	if err != nil {
		t.Error(err)
	}

	err = tags.Tag(ctx, "latest", desc)
	if err != nil {
		t.Error(err)
	}

	err = tags.Untag(ctx, "latest")
	if err != nil {
		t.Error(err)
	}

	errExpect := distribution.ErrTagUnknown{Tag: "latest"}.Error()
	_, err = tags.Get(ctx, "latest")
	if err == nil || err.Error() != errExpect {
		t.Error("Expected error getting untagged tag")
	}
}

func TestTagStoreAll(t *testing.T) {
	env := testTagStore(t)
	tagStore := env.ts
	ctx := env.ctx

	alpha := "abcdefghijklmnopqrstuvwxyz"
	for i := 0; i < len(alpha); i++ {
		tag := alpha[i]
		desc := distribution.Descriptor{Digest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}
		err := tagStore.Tag(ctx, string(tag), desc)
		if err != nil {
			t.Error(err)
		}
	}

	all, err := tagStore.All(ctx)
	if err != nil {
		t.Error(err)
	}
	if len(all) != len(alpha) {
		t.Errorf("Unexpected count returned from enumerate")
	}

	for i, c := range all {
		if c != string(alpha[i]) {
			t.Errorf("unexpected tag in enumerate %s", c)
		}
	}

	removed := "a"
	err = tagStore.Untag(ctx, removed)
	if err != nil {
		t.Error(err)
	}

	all, err = tagStore.All(ctx)
	if err != nil {
		t.Error(err)
	}
	for _, tag := range all {
		if tag == removed {
			t.Errorf("unexpected tag in enumerate %s", removed)
		}
	}

}

func TestTagLookup(t *testing.T) {
	env := testTagStore(t)
	tagStore := env.ts
	ctx := env.ctx

	descA := distribution.Descriptor{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	desc0 := distribution.Descriptor{Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}

	tags, err := tagStore.Lookup(ctx, descA)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatalf("Lookup returned > 0 tags from empty store")
	}

	err = tagStore.Tag(ctx, "a", descA)
	if err != nil {
		t.Fatal(err)
	}

	err = tagStore.Tag(ctx, "b", descA)
	if err != nil {
		t.Fatal(err)
	}

	err = tagStore.Tag(ctx, "0", desc0)
	if err != nil {
		t.Fatal(err)
	}

	err = tagStore.Tag(ctx, "1", desc0)
	if err != nil {
		t.Fatal(err)
	}

	tags, err = tagStore.Lookup(ctx, descA)
	if err != nil {
		t.Fatal(err)
	}

	if len(tags) != 2 {
		t.Errorf("Lookup of descA returned %d tags, expected 2", len(tags))
	}

	tags, err = tagStore.Lookup(ctx, desc0)
	if err != nil {
		t.Fatal(err)
	}

	if len(tags) != 2 {
		t.Errorf("Lookup of descB returned %d tags, expected 2", len(tags))
	}
}

func TestTagIndexes(t *testing.T) {
	env := testTagStore(t)
	tagStore := env.ts
	ctx := env.ctx

	indexes, ok := tagStore.(distribution.TagIndexes)
	if !ok {
		t.Fatal("tagStore does not implement TagIndexes interface")
	}

	conf, err := env.bs.Put(ctx, "application/octet-stream", []byte{0})
	if err != nil {
		t.Fatal(err)
	}

	dgstsSet := make(map[digest.Digest]bool)
	for i := 0; i < 3; i++ {
		layer, err := env.bs.Put(ctx, "application/octet-stream", []byte{byte(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		m := schema2.Manifest{
			Versioned: manifest.Versioned{
				SchemaVersion: 2,
				MediaType:     schema2.MediaTypeManifest,
			},
			Config: distribution.Descriptor{
				Digest:    conf.Digest,
				Size:      1,
				MediaType: schema2.MediaTypeImageConfig,
			},
			Layers: []distribution.Descriptor{
				{
					Digest:    layer.Digest,
					Size:      1,
					MediaType: schema2.MediaTypeLayer,
				},
			},
		}
		dm, err := schema2.FromStruct(m)
		if err != nil {
			t.Fatal(err)
		}
		dgst, err := env.ms.Put(ctx, dm)
		if err != nil {
			t.Fatal(err)
		}
		desc, err := env.gbs.Stat(ctx, dgst)
		if err != nil {
			t.Fatal(err)
		}
		err = tagStore.Tag(ctx, "t", desc)
		if err != nil {
			t.Fatal(err)
		}
		dgstsSet[dgst] = true
	}

	gotDgsts, err := indexes.Indexes(ctx, "t")
	if err != nil {
		t.Fatal(err)
	}
	gotDgstsSet := make(map[digest.Digest]bool)
	for _, dgst := range gotDgsts {
		gotDgstsSet[dgst] = true
	}
	if !reflect.DeepEqual(dgstsSet, gotDgstsSet) {
		t.Fatalf("Expected digests: %v but got digests: %v", dgstsSet, gotDgstsSet)
	}
}

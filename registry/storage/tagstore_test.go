package storage

import (
	"context"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
	digest "github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
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

	d := v1.Descriptor{}
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
	desc := v1.Descriptor{Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}

	err := tags.Untag(ctx, "latest")
	if err == nil {
		t.Error("expected error removing unknown tag")
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
		desc := v1.Descriptor{Digest: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"}
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

	descA := v1.Descriptor{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	desc0 := v1.Descriptor{Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000"}

	tags, err := tagStore.Lookup(ctx, descA)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatal("Lookup returned > 0 tags from empty store")
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

	md, ok := tagStore.(distribution.TagManifestsProvider)
	if !ok {
		t.Fatal("tagStore does not implement TagManifestDigests interface")
	}

	conf, err := env.bs.Put(ctx, "application/octet-stream", []byte{0})
	if err != nil {
		t.Fatal(err)
	}

	t1Dgsts := make(map[digest.Digest]struct{})
	t2Dgsts := make(map[digest.Digest]struct{})
	for i := 0; i < 5; i++ {
		layer, err := env.bs.Put(ctx, "application/octet-stream", []byte{byte(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		m := schema2.Manifest{
			Versioned: specs.Versioned{SchemaVersion: 2},
			MediaType: schema2.MediaTypeManifest,
			Config: v1.Descriptor{
				Digest:    conf.Digest,
				Size:      1,
				MediaType: schema2.MediaTypeImageConfig,
			},
			Layers: []v1.Descriptor{
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
		if i < 3 {
			// tag first 3 manifests as "t1"
			err = tagStore.Tag(ctx, "t1", desc)
			if err != nil {
				t.Fatal(err)
			}
			t1Dgsts[dgst] = struct{}{}
		} else {
			// the last two under "t2"
			err = tagStore.Tag(ctx, "t2", desc)
			if err != nil {
				t.Fatal(err)
			}
			t2Dgsts[dgst] = struct{}{}
		}
	}

	gotT1Dgsts, err := md.ManifestDigests(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(t1Dgsts, digestMap(gotT1Dgsts)) {
		t.Fatalf("Expected digests: %v but got digests: %v", t1Dgsts, digestMap(gotT1Dgsts))
	}

	gotT2Dgsts, err := md.ManifestDigests(ctx, "t2")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(t2Dgsts, digestMap(gotT2Dgsts)) {
		t.Fatalf("Expected digests: %v but got digests: %v", t2Dgsts, digestMap(gotT2Dgsts))
	}
}

func digestMap(dgsts []digest.Digest) map[digest.Digest]struct{} {
	set := make(map[digest.Digest]struct{})
	for _, dgst := range dgsts {
		set[dgst] = struct{}{}
	}
	return set
}

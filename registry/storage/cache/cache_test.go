package cache

import (
	"testing"

	"golang.org/x/net/context"
)

// checkLayerInfoCache takes a cache implementation through a common set of
// operations. If adding new tests, please add them here so new
// implementations get the benefit.
func checkLayerInfoCache(t *testing.T, lic LayerInfoCache) {
	ctx := context.Background()

	exists, err := lic.Contains(ctx, "", "fake:abc")
	if err == nil {
		t.Fatalf("expected error checking for cache item with empty repo")
	}

	exists, err = lic.Contains(ctx, "foo/bar", "")
	if err == nil {
		t.Fatalf("expected error checking for cache item with empty digest")
	}

	exists, err = lic.Contains(ctx, "foo/bar", "fake:abc")
	if err != nil {
		t.Fatalf("unexpected error checking for cache item: %v", err)
	}

	if exists {
		t.Fatalf("item should not exist")
	}

	if err := lic.Add(ctx, "", "fake:abc"); err == nil {
		t.Fatalf("expected error adding cache item with empty name")
	}

	if err := lic.Add(ctx, "foo/bar", ""); err == nil {
		t.Fatalf("expected error adding cache item with empty digest")
	}

	if err := lic.Add(ctx, "foo/bar", "fake:abc"); err != nil {
		t.Fatalf("unexpected error adding item: %v", err)
	}

	exists, err = lic.Contains(ctx, "foo/bar", "fake:abc")
	if err != nil {
		t.Fatalf("unexpected error checking for cache item: %v", err)
	}

	if !exists {
		t.Fatalf("item should exist")
	}

	_, err = lic.Meta(ctx, "")
	if err == nil || err == ErrNotFound {
		t.Fatalf("expected error getting meta for cache item with empty digest")
	}

	_, err = lic.Meta(ctx, "fake:abc")
	if err != ErrNotFound {
		t.Fatalf("expected unknown layer error getting meta for cache item with empty digest")
	}

	if err = lic.SetMeta(ctx, "", LayerMeta{}); err == nil {
		t.Fatalf("expected error setting meta for cache item with empty digest")
	}

	if err = lic.SetMeta(ctx, "foo/bar", LayerMeta{}); err == nil {
		t.Fatalf("expected error setting meta for cache item with empty meta")
	}

	expected := LayerMeta{Path: "/foo/bar", Length: 20}
	if err := lic.SetMeta(ctx, "foo/bar", expected); err != nil {
		t.Fatalf("unexpected error setting meta: %v", err)
	}

	meta, err := lic.Meta(ctx, "foo/bar")
	if err != nil {
		t.Fatalf("unexpected error getting meta: %v", err)
	}

	if meta != expected {
		t.Fatalf("retrieved meta data did not match: %v", err)
	}
}

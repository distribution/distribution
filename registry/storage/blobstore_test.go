package storage

import (
	"context"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/opencontainers/go-digest"
)

// TestBlobStorePutSkipsExistingContent verifies that Put skips writing when
// the blob genuinely exists in the backend, even with a cachedBlobStatter.
func TestBlobStorePutSkipsExistingContent(t *testing.T) {
	ctx := context.Background()
	drvr := inmemory.New()

	cacheProvider := memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)
	repoCache, err := cacheProvider.RepositoryScoped("test/repo")
	if err != nil {
		t.Fatalf("RepositoryScoped: %v", err)
	}

	rawStatter := &blobStatter{driver: drvr}
	cachedStatter := cache.NewCachedBlobStatter(repoCache, rawStatter)

	bs := &blobStore{
		driver:  drvr,
		statter: cachedStatter,
	}

	content := []byte("existing-blob-content")
	dgst := digest.FromBytes(content)

	if _, err := bs.Put(ctx, "application/octet-stream", content); err != nil {
		t.Fatalf("Put: %v", err)
	}

	bp, _ := bs.path(dgst)
	fi1, err := drvr.Stat(ctx, bp)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if _, err := bs.Put(ctx, "application/octet-stream", content); err != nil {
		t.Fatalf("second Put: %v", err)
	}

	fi2, _ := drvr.Stat(ctx, bp)
	if fi1.ModTime() != fi2.ModTime() {
		t.Fatal("Put rewrote data that already existed")
	}
}

func isPathNotFound(err error) bool {
	_, ok := err.(driver.PathNotFoundError)
	return ok
}

// TestBlobStorePutAfterGCWithStaleCache verifies that Put writes data back
// after GC deletes blob data from the backend, even when the cachedBlobStatter
// still has a stale cache entry reporting the blob as present. This is the
// actual bug scenario: after GC, the cache says "exists" but the backend data
// is gone, so Put must bypass the cache and check the backend directly.
func TestBlobStorePutAfterGCWithStaleCache(t *testing.T) {
	ctx := context.Background()
	drvr := inmemory.New()

	cacheProvider := memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)
	repoCache, err := cacheProvider.RepositoryScoped("test/repo")
	if err != nil {
		t.Fatalf("RepositoryScoped: %v", err)
	}

	rawStatter := &blobStatter{driver: drvr}
	cachedStatter := cache.NewCachedBlobStatter(repoCache, rawStatter)

	bs := &blobStore{
		driver:  drvr,
		statter: cachedStatter,
	}

	content := []byte("test-blob-content")
	dgst := digest.FromBytes(content)

	// Step 1: Put the blob. This also populates the cache.
	desc, err := bs.Put(ctx, "application/octet-stream", content)
	if err != nil {
		t.Fatalf("initial Put: %v", err)
	}
	if desc.Digest != dgst {
		t.Fatalf("digest: got %q, want %q", desc.Digest, dgst)
	}

	// Step 2: Verify the cached statter reports the blob exists.
	_, err = cachedStatter.Stat(ctx, dgst)
	if err != nil {
		t.Fatalf("cached statter should report blob exists after Put: %v", err)
	}

	// Step 3: Simulate GC — delete blob data from backend.
	bp, _ := bs.path(dgst)
	if err := drvr.Delete(ctx, bp); err != nil {
		t.Fatalf("delete (GC simulation): %v", err)
	}

	// Step 4: Confirm backend has no blob.
	if _, err := drvr.Stat(ctx, bp); !isPathNotFound(err) {
		t.Fatalf("expected PathNotFoundError after GC, got: %v", err)
	}

	// Step 5: Confirm cached statter STILL reports blob exists (stale cache).
	cachedDesc, err := cachedStatter.Stat(ctx, dgst)
	if err != nil {
		t.Fatalf("cached statter should still report blob exists (stale entry): %v", err)
	}
	if cachedDesc.Digest != dgst {
		t.Fatalf("stale cache digest: got %q, want %q", cachedDesc.Digest, dgst)
	}

	// Step 6: Re-put should succeed because Put bypasses the stale cache
	// and checks the backend directly.
	desc, err = bs.Put(ctx, "application/octet-stream", content)
	if err != nil {
		t.Fatalf("re-Put after GC with stale cache: %v", err)
	}
	if desc.Digest != dgst {
		t.Fatalf("digest after re-Put: got %q, want %q", desc.Digest, dgst)
	}

	// Step 7: Verify the content is actually readable.
	got, err := bs.Get(ctx, dgst)
	if err != nil {
		t.Fatalf("Get after re-Put: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("content: got %q, want %q", got, content)
	}
}

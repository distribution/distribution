package memory

import (
	"context"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache/cachecheck"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// TestInMemoryBlobInfoCache checks the in memory implementation is working
// correctly.
func TestInMemoryBlobInfoCache(t *testing.T) {
	cachecheck.CheckBlobDescriptorCache(t, NewInMemoryBlobDescriptorCacheProvider(UnlimitedSize))
}

// TestInMemoryBlobInfoCacheWithTTL checks the in memory implementation is
// working correctly when entries have a TTL.
func TestInMemoryBlobInfoCacheWithTTL(t *testing.T) {
	cachecheck.CheckBlobDescriptorCache(t, NewInMemoryBlobDescriptorCacheProviderWithTTL(UnlimitedSize, time.Hour))
}

// TestInMemoryBlobInfoCacheTTLExpiry checks that descriptors are no longer
// returned once their TTL has elapsed, both globally and repository-scoped.
func TestInMemoryBlobInfoCacheTTLExpiry(t *testing.T) {
	ctx := context.Background()
	ttl := 50 * time.Millisecond
	provider := NewInMemoryBlobDescriptorCacheProviderWithTTL(UnlimitedSize, ttl)

	dgst := digest.FromString("ttl test")
	desc := v1.Descriptor{
		Digest:    dgst,
		Size:      10,
		MediaType: "application/octet-stream",
	}

	repoCache, err := provider.RepositoryScoped("foo/bar")
	if err != nil {
		t.Fatalf("unexpected error getting repository cache: %v", err)
	}
	if err := repoCache.SetDescriptor(ctx, dgst, desc); err != nil {
		t.Fatalf("unexpected error setting descriptor: %v", err)
	}

	if _, err := provider.Stat(ctx, dgst); err != nil {
		t.Fatalf("expected descriptor to be cached, got: %v", err)
	}
	if _, err := repoCache.Stat(ctx, dgst); err != nil {
		t.Fatalf("expected repository descriptor to be cached, got: %v", err)
	}

	time.Sleep(2 * ttl)

	if _, err := provider.Stat(ctx, dgst); err != distribution.ErrBlobUnknown {
		t.Fatalf("expected ErrBlobUnknown after ttl, got: %v", err)
	}
	if _, err := repoCache.Stat(ctx, dgst); err != distribution.ErrBlobUnknown {
		t.Fatalf("expected ErrBlobUnknown for repository after ttl, got: %v", err)
	}
}

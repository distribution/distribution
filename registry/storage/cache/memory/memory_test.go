package memory

import (
	"context"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/cache/cachecheck"
)

// TestInMemoryBlobInfoCache checks the in memory implementation is working
// correctly.
func TestInMemoryBlobInfoCache(t *testing.T) {
	opts := NewCacheOptions(UnlimitedSize)
	cache, err := NewBlobDescriptorCacheProvider(context.Background(), opts)
	if err != nil {
		t.Fatalf("init cache: %v", err)
	}
	cachecheck.CheckBlobDescriptorCache(t, cache)
}

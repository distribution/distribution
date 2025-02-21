package memory

import (
	"testing"

	"github.com/goharbor/distribution/registry/storage/cache/cachecheck"
)

// TestInMemoryBlobInfoCache checks the in memory implementation is working
// correctly.
func TestInMemoryBlobInfoCache(t *testing.T) {
	cachecheck.CheckBlobDescriptorCache(t, NewInMemoryBlobDescriptorCacheProvider())
}

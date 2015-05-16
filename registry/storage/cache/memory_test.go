package cache

import "testing"

// TestInMemoryBlobInfoCache checks the in memory implementation is working
// correctly.
func TestInMemoryBlobInfoCache(t *testing.T) {
	checkBlobDescriptorCache(t, NewInMemoryBlobDescriptorCacheProvider())
}

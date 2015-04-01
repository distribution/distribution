package cache

import "testing"

// TestInMemoryLayerInfoCache checks the in memory implementation is working
// correctly.
func TestInMemoryLayerInfoCache(t *testing.T) {
	checkLayerInfoCache(t, NewInMemoryLayerInfoCache())
}

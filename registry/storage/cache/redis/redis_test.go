package redis

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/cache/cachecheck"
)

var redisAddr string

func init() {
	flag.StringVar(&redisAddr, "test.registry.storage.cache.redis.addr", "", "configure the address of a test instance of redis")
}

func makeOptions(addr string) map[string]interface{} {
	return map[string]interface{}{
		"params": map[interface{}]interface{}{
			"addr": addr,
			"pool": map[interface{}]interface{}{
				"maxactive": 3,
			},
		},
	}
}

// TestRedisLayerInfoCache exercises a live redis instance using the cache
// implementation.
func TestRedisBlobDescriptorCacheProvider(t *testing.T) {
	if redisAddr == "" {
		// fallback to an environement variable
		redisAddr = os.Getenv("TEST_REGISTRY_STORAGE_CACHE_REDIS_ADDR")
	}

	if redisAddr == "" {
		// skip if still not set
		t.Skip("please set -test.registry.storage.cache.redis.addr to test layer info cache against redis")
	}

	// Clear the database
	ctx := context.Background()

	opts := makeOptions(redisAddr)
	cache, err := NewBlobDescriptorCacheProvider(ctx, opts)
	if err != nil {
		t.Fatalf("init redis cache: %v", err)
	}

	// TODO(milosgajdos): figure out how to flush redis DB before test
	cachecheck.CheckBlobDescriptorCache(t, cache)
}

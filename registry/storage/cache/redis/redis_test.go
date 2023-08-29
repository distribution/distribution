package redis

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/cache/cachecheck"
	"github.com/redis/go-redis/v9"
)

var redisAddr string

func init() {
	flag.StringVar(&redisAddr, "test.registry.storage.cache.redis.addr", "", "configure the address of a test instance of redis")
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

	pool := redis.NewClient(&redis.Options{
		Addr: redisAddr,
		OnConnect: func(ctx context.Context, cn *redis.Conn) error {
			res := cn.Ping(ctx)
			return res.Err()
		},
		MaxRetries: 3,
		PoolSize:   2,
	})

	// Clear the database
	ctx := context.Background()
	err := pool.FlushDB(ctx).Err()
	if err != nil {
		t.Fatalf("unexpected error flushing redis db: %v", err)
	}

	cachecheck.CheckBlobDescriptorCache(t, NewRedisBlobDescriptorCacheProvider(pool))
}

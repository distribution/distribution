package redis

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/registry/storage/cache/cachecheck"
	"github.com/garyburd/redigo/redis"
	"github.com/opencontainers/go-digest"
)

var redisAddr string

func init() {
	flag.StringVar(&redisAddr, "test.registry.storage.cache.redis.addr", "", "configure the address of a test instance of redis")
}

// TestRedisLayerInfoCache exercises a live redis instance using the cache
// implementation.
func TestRedisBlobDescriptorCacheProvider(t *testing.T) {
	pool := mustRedisPool(t)

	// Clear the database
	conn := pool.Get()
	if _, err := conn.Do("FLUSHDB"); err != nil {
		t.Fatalf("unexpected error flushing redis db: %v", err)
	}
	conn.Close()

	cachecheck.CheckBlobDescriptorCache(t, NewRedisBlobDescriptorCacheProvider(pool))
}

// TestRepositoryScopedClearRevokesRepositoryMembership ensures Clear removes
// repository-scoped membership so a peer repository warm-up cannot re-enable
// stale access in the cleared repository.
func TestRepositoryScopedClearRevokesRepositoryMembership(t *testing.T) {
	pool := mustRedisPool(t)
	flushRedisDB(t, pool)

	ctx := context.Background()
	cache := &redisBlobDescriptorService{pool: pool}
	repoA := &repositoryScopedRedisBlobDescriptorService{repo: "foo/repo-a", upstream: cache}
	repoB := &repositoryScopedRedisBlobDescriptorService{repo: "foo/repo-b", upstream: cache}

	dgst := digest.FromString("stale-membership-regression")
	desc := distribution.Descriptor{
		Digest:    dgst,
		Size:      1337,
		MediaType: "application/vnd.oci.image.layer.v1.tar",
	}

	if err := repoA.SetDescriptor(ctx, dgst, desc); err != nil {
		t.Fatalf("unexpected error setting descriptor for repo a: %v", err)
	}
	if err := repoB.SetDescriptor(ctx, dgst, desc); err != nil {
		t.Fatalf("unexpected error setting descriptor for repo b: %v", err)
	}
	if _, err := repoA.Stat(ctx, dgst); err != nil {
		t.Fatalf("unexpected error statting descriptor for repo a before clear: %v", err)
	}

	if err := repoA.Clear(ctx, dgst); err != nil {
		t.Fatalf("unexpected error clearing descriptor for repo a: %v", err)
	}
	if _, err := repoA.Stat(ctx, dgst); err != distribution.ErrBlobUnknown {
		t.Fatalf("expected repo a stat after clear to return ErrBlobUnknown, got: %v", err)
	}

	// Simulate another repository repopulating the shared descriptor hash.
	if err := repoB.SetDescriptor(ctx, dgst, desc); err != nil {
		t.Fatalf("unexpected error warming descriptor for repo b: %v", err)
	}
	if _, err := repoB.Stat(ctx, dgst); err != nil {
		t.Fatalf("unexpected error statting descriptor for repo b after warm: %v", err)
	}
	if _, err := repoA.Stat(ctx, dgst); err != distribution.ErrBlobUnknown {
		t.Fatalf("expected repo a stat after peer warm to return ErrBlobUnknown, got: %v", err)
	}

	conn := pool.Get()
	defer conn.Close()

	member, err := redis.Bool(conn.Do("SISMEMBER", repoA.repositoryBlobSetKey(repoA.repo), dgst))
	if err != nil {
		t.Fatalf("unexpected error checking repo a membership: %v", err)
	}
	if member {
		t.Fatal("expected repo a membership to be removed during clear")
	}

	exists, err := redis.Int(conn.Do("EXISTS", repoA.blobDescriptorHashKey(dgst)))
	if err != nil {
		t.Fatalf("unexpected error checking repo a descriptor hash: %v", err)
	}
	if exists != 0 {
		t.Fatal("expected repo a descriptor hash to be removed during clear")
	}
}

// mustRedisPool creates a redis pool for integration tests or skips when no
// redis endpoint is configured.
func mustRedisPool(t *testing.T) *redis.Pool {
	t.Helper()

	if redisAddr == "" {
		// fallback to an environment variable
		redisAddr = os.Getenv("TEST_REGISTRY_STORAGE_CACHE_REDIS_ADDR")
	}

	if redisAddr == "" {
		t.Skip("please set -test.registry.storage.cache.redis.addr to test layer info cache against redis")
	}

	return &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", redisAddr)
		},
		MaxIdle:   1,
		MaxActive: 2,
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
		Wait: false, // if a connection is not available, proceed without cache.
	}
}

// flushRedisDB clears redis before each integration test to isolate test data.
func flushRedisDB(t *testing.T, pool *redis.Pool) {
	t.Helper()

	conn := pool.Get()
	defer conn.Close()

	if _, err := conn.Do("FLUSHDB"); err != nil {
		t.Fatalf("unexpected error flushing redis db: %v", err)
	}
}

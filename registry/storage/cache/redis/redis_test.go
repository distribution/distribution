package redis

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache/cachecheck"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
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
		// fallback to an environment variable
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

func TestRepositoryScopedClearRevokesRepositoryMembership(t *testing.T) {
	ctx := context.Background()

	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("unexpected error starting miniredis: %v", err)
	}
	defer server.Close()

	pool := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer pool.Close()

	cache := &redisBlobDescriptorService{pool: pool}
	repoA := &repositoryScopedRedisBlobDescriptorService{repo: "foo/repo-a", upstream: cache}
	repoB := &repositoryScopedRedisBlobDescriptorService{repo: "foo/repo-b", upstream: cache}

	dgst := digest.FromString("stale-membership-regression")
	desc := v1.Descriptor{
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
		t.Fatalf("unexpected error statting descriptor for repo a before delete: %v", err)
	}

	if err := repoA.Clear(ctx, dgst); err != nil {
		t.Fatalf("unexpected error clearing descriptor for repo a: %v", err)
	}

	if _, err := repoA.Stat(ctx, dgst); err != distribution.ErrBlobUnknown {
		t.Fatalf("expected repo a stat after clear to return ErrBlobUnknown, got: %v", err)
	}

	// Simulate a peer repository repopulating the shared descriptor after a backend miss.
	if err := repoB.SetDescriptor(ctx, dgst, desc); err != nil {
		t.Fatalf("unexpected error warming descriptor for repo b: %v", err)
	}

	if _, err := repoB.Stat(ctx, dgst); err != nil {
		t.Fatalf("unexpected error statting descriptor for repo b after warm: %v", err)
	}
	if _, err := repoA.Stat(ctx, dgst); err != distribution.ErrBlobUnknown {
		t.Fatalf("expected repo a stat after peer warm to return ErrBlobUnknown, got: %v", err)
	}

	member, err := pool.SIsMember(ctx, repoA.repositoryBlobSetKey(repoA.repo), dgst.String()).Result()
	if err != nil {
		t.Fatalf("unexpected error checking repo a membership: %v", err)
	}
	if member {
		t.Fatal("expected repo a membership to be removed during clear")
	}

	exists, err := pool.Exists(ctx, repoA.blobDescriptorHashKey(dgst)).Result()
	if err != nil {
		t.Fatalf("unexpected error checking repo a descriptor hash: %v", err)
	}
	if exists != 0 {
		t.Fatal("expected repo a descriptor hash to be removed during clear")
	}
}

package memcached

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/daangn/minimemcached"
	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache/cachecheck"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var memcachedAddr string

func init() {
	flag.StringVar(&memcachedAddr, "test.registry.storage.cache.memcached.addr", "", "configure the address of a test instance of memcached")
}

// TestMemcachedBlobDescriptorCacheProvider exercises a live memcached
// instance using the cache implementation.
func TestMemcachedBlobDescriptorCacheProvider(t *testing.T) {
	if memcachedAddr == "" {
		// fallback to an environment variable
		memcachedAddr = os.Getenv("TEST_REGISTRY_STORAGE_CACHE_MEMCACHED_ADDR")
	}

	if memcachedAddr == "" {
		// skip if still not set
		t.Skip("please set -test.registry.storage.cache.memcached.addr to test layer info cache against memcached")
	}

	client := memcache.New(memcachedAddr)

	// Clear the cache
	if err := client.FlushAll(); err != nil {
		t.Fatalf("unexpected error flushing memcached: %v", err)
	}

	cachecheck.CheckBlobDescriptorCache(t, NewMemcachedBlobDescriptorCacheProvider(client))
}

// newMemcachedClient starts an in-process minimemcached server and returns a
// client connected to it. The server is shut down when the test finishes.
func newMemcachedClient(t *testing.T) *memcache.Client {
	t.Helper()

	srv, err := minimemcached.Run(&minimemcached.Config{})
	if err != nil {
		t.Fatalf("unexpected error starting minimemcached: %v", err)
	}

	t.Cleanup(srv.Close)

	return memcache.New(fmt.Sprintf("localhost:%d", srv.Port()))
}

func TestMemcachedCache(t *testing.T) {
	provider := NewMemcachedBlobDescriptorCacheProvider(newMemcachedClient(t))

	cachecheck.CheckBlobDescriptorCache(t, provider)
}

func TestRepositoryScopedClearRevokesRepositoryMembership(t *testing.T) {
	client := newMemcachedClient(t)
	ctx := context.Background()

	cache := &memcachedBlobDescriptorService{client: client}
	repoA := &repositoryScopedMemcachedBlobDescriptorService{repo: "foo/repo-a", upstream: cache}
	repoB := &repositoryScopedMemcachedBlobDescriptorService{repo: "foo/repo-b", upstream: cache}

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

	if _, err := client.Get(repoA.blobDescriptorKey(dgst)); err != memcache.ErrCacheMiss {
		t.Fatalf("expected repo a descriptor entry to be removed during clear, got: %v", err)
	}
}

func TestMemcachedCacheMediaTypePreservation(t *testing.T) {
	client := newMemcachedClient(t)
	provider := NewMemcachedBlobDescriptorCacheProvider(client)

	ctx := context.Background()
	localDigest := digest.Digest("sha512:" + strings.Repeat("a", 128))
	first := v1.Descriptor{
		Digest:    digest.Digest("sha256:" + strings.Repeat("b", 64)),
		Size:      10,
		MediaType: "application/octet-stream",
	}

	cache, err := provider.RepositoryScoped("foo/bar")

	if err != nil {
		t.Fatalf("unexpected error getting scoped cache: %v", err)
	}

	if err := cache.SetDescriptor(ctx, localDigest, first); err != nil {
		t.Fatalf("error setting descriptor: %v", err)
	}

	// The repository scoped write must not clobber the global media type.
	second := first
	second.MediaType = "application/json"

	if err := cache.SetDescriptor(ctx, localDigest, second); err != nil {
		t.Fatalf("error setting descriptor: %v", err)
	}

	desc, err := provider.Stat(ctx, localDigest)

	if err != nil {
		t.Fatalf("unexpected error statting descriptor: %v", err)
	}

	if desc.MediaType != "application/octet-stream" {
		t.Fatalf("global media type was clobbered: %#v", desc)
	}

	desc, err = cache.Stat(ctx, localDigest)

	if err != nil {
		t.Fatalf("unexpected error statting descriptor: %v", err)
	}

	if desc.MediaType != "application/json" {
		t.Fatalf("repository media type not preserved: %#v", desc)
	}
}

func TestKeyFor(t *testing.T) {
	short := "repository::foo/bar::blobs::sha256:abc1111111111111111111111111111111111111111111111111111111111111"

	if got := keyFor(short); got != short {
		t.Fatalf("expected short key to be used as-is, got %q", got)
	}

	long := "repository::" + strings.Repeat("a/", 100) + "name::blobs::sha256:abc1111111111111111111111111111111111111111111111111111111111111"
	hashed := keyFor(long)

	if hashed == long {
		t.Fatal("expected long key to be hashed")
	}

	if len(hashed) > 250 {
		t.Fatalf("hashed key exceeds memcached key limit: %d", len(hashed))
	}

	if !legalKey(hashed) {
		t.Fatalf("hashed key is not a legal memcached key: %q", hashed)
	}
}

func legalKey(key string) bool {
	if len(key) > 250 {
		return false
	}

	for _, c := range key {
		if c <= 0x20 || c == 0x7f {
			return false
		}
	}

	return true
}

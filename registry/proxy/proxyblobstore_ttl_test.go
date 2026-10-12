package proxy

import (
	"context"
	"net/http/httptest"
	"path"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
)

type ttlTestEnv struct {
	ctx         context.Context
	localDriver storagedriver.StorageDriver
	local       distribution.Namespace
	remote      distribution.Namespace
}

func makeTTLTestEnv(t *testing.T, ttl time.Duration) *ttlTestEnv {
	t.Helper()
	ctx := context.Background()

	localDriver, err := filesystem.FromParameters(map[string]any{"rootdirectory": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	local, err := storage.NewRegistry(ctx, localDriver,
		storage.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProviderWithTTL(memory.DefaultSize, ttl)),
		storage.DisableDigestResumption)
	if err != nil {
		t.Fatal(err)
	}

	remoteDriver, err := filesystem.FromParameters(map[string]any{"rootdirectory": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	remote, err := storage.NewRegistry(ctx, remoteDriver)
	if err != nil {
		t.Fatal(err)
	}

	return &ttlTestEnv{ctx: ctx, localDriver: localDriver, local: local, remote: remote}
}

func (e *ttlTestEnv) proxy(t *testing.T, name string) proxyBlobStore {
	t.Helper()
	named, err := reference.WithName(name)
	if err != nil {
		t.Fatal(err)
	}
	localRepo, err := e.local.Repository(e.ctx, named)
	if err != nil {
		t.Fatal(err)
	}
	remoteRepo, err := e.remote.Repository(e.ctx, named)
	if err != nil {
		t.Fatal(err)
	}
	return proxyBlobStore{
		repositoryName:    named,
		localStore:        statsBlobStore{stats: map[string]int{}, blobs: localRepo.Blobs(e.ctx)},
		remoteStore:       statsBlobStore{stats: map[string]int{}, blobs: remoteRepo.Blobs(e.ctx)},
		cacheWriteTimeout: time.Minute,
		authChallenger:    &mockChallenger{},
	}
}

func (e *ttlTestEnv) deleteLocalBlobData(t *testing.T, dgst digest.Digest) {
	t.Helper()
	p := path.Join("/docker/registry/v2/blobs", dgst.Algorithm().String(), dgst.Encoded()[:2], dgst.Encoded(), "data")
	if err := e.localDriver.Delete(e.ctx, p); err != nil {
		t.Fatal(err)
	}
}

func serveBlob(pbs proxyBlobStore, ctx context.Context, dgst digest.Digest) error {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	return pbs.ServeBlob(ctx, w, r, dgst)
}

// A blob shared by two repositories and removed from local storage must be
// fetched again from upstream at most one TTL after the removal, even when a
// repository cache entry was populated from the global cache in between.
func TestProxyStoreServeSharedBlobRefetchedWithinTTL(t *testing.T) {
	const ttl = time.Second
	e := makeTTLTestEnv(t, ttl)
	a, b := e.proxy(t, "foo/a"), e.proxy(t, "foo/b")

	blob := makeBlob(1024)
	desc, err := a.remoteStore.Put(e.ctx, "", blob)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.remoteStore.Put(e.ctx, "", blob); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	at := func(fraction float64) {
		time.Sleep(time.Until(start.Add(time.Duration(fraction * float64(ttl)))))
	}

	// Both repositories pull the blob; the global entry dates from repo a.
	if err := serveBlob(a, e.ctx, desc.Digest); err != nil {
		t.Fatal(err)
	}
	at(0.5)
	if err := serveBlob(b, e.ctx, desc.Digest); err != nil {
		t.Fatal(err)
	}

	// Everything from the first pull expired: repo a re-stats the real storage.
	at(1.1)
	if err := serveBlob(a, e.ctx, desc.Digest); err != nil {
		t.Fatal(err)
	}

	e.deleteLocalBlobData(t, desc.Digest)
	deletedAt := time.Now()

	// Repo b's entry expired; it is now refilled from the global entry.
	at(1.6)
	_ = serveBlob(b, e.ctx, desc.Digest)

	// One TTL after the removal every entry describing the blob must be gone.
	time.Sleep(time.Until(deletedAt.Add(ttl + 100*time.Millisecond)))
	remoteGets := b.remoteStore.(statsBlobStore).stats["open"]
	if err := serveBlob(b, e.ctx, desc.Digest); err != nil {
		t.Fatalf("blob removed %s ago (ttl %s) still served from local storage: %v", time.Since(deletedAt).Round(time.Millisecond), ttl, err)
	}
	if b.remoteStore.(statsBlobStore).stats["open"] == remoteGets {
		t.Fatal("blob was not fetched again from upstream")
	}
}

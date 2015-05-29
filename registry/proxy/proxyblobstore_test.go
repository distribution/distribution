package proxy

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
)

type statsBlobStore struct {
	stats map[string]int
	blobs distribution.BlobStore
}

func (sbs statsBlobStore) Put(ctx context.Context, mediaType string, p []byte) (distribution.Descriptor, error) {
	sbs.stats["put"]++
	return sbs.blobs.Put(ctx, mediaType, p)
}

func (sbs statsBlobStore) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	sbs.stats["get"]++
	return sbs.blobs.Get(ctx, dgst)
}

func (sbs statsBlobStore) Create(ctx context.Context) (distribution.BlobWriter, error) {
	return nil, fmt.Errorf("Not Allowed")
}

func (sbs statsBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	return nil, fmt.Errorf("Not Allowed")
}

func (sbs statsBlobStore) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (sbs statsBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	sbs.stats["serveblob"]++
	return sbs.blobs.ServeBlob(ctx, w, r, dgst)
}

func (sbs statsBlobStore) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	sbs.stats["stat"]++
	return sbs.blobs.Stat(ctx, dgst)
}

type testEnv struct {
	inRemote []digest.Digest
	store    proxyBlobStore
	ctx      context.Context
}

func (te testEnv) LocalStats() *map[string]int {
	ls := te.store.localStore.(statsBlobStore).stats
	return &ls
}

func (te testEnv) RemoteStats() *map[string]int {
	rs := te.store.remoteStore.(statsBlobStore).stats
	return &rs
}

// Populate remote store and record the digests
func makeTestEnv(t *testing.T, name string) testEnv {
	ctx := context.Background()
	truthRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider())
	truthRepo, err := truthRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	truthBlobs := statsBlobStore{
		stats: make(map[string]int),
		blobs: truthRepo.Blobs(ctx),
	}

	var inRemote []digest.Digest
	for i := 0; i < 3; i++ {
		bytes := []byte(fmt.Sprintf("blob%d", i))

		desc, err := truthBlobs.Put(ctx, "", bytes)
		if err != nil {
			t.Errorf("Put in store")
		}
		inRemote = append(inRemote, desc.Digest)
	}

	localRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider())
	localRepo, err := localRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	localBlobs := statsBlobStore{
		stats: make(map[string]int),
		blobs: localRepo.Blobs(ctx),
	}

	proxyBlobStore := proxyBlobStore{
		remoteStore: truthBlobs,
		localStore:  localBlobs,
	}

	te := testEnv{
		inRemote: inRemote,
		store:    proxyBlobStore,
		ctx:      ctx,
	}

	return te
}

func TestProxyStore(t *testing.T) {
	te := makeTestEnv(t, "foo/bar")
	remoteBlobCount := len(te.inRemote)

	localStats := te.LocalStats()
	remoteStats := te.RemoteStats()

	// Stat - touches both stores
	for _, d := range te.inRemote {
		_, err := te.store.Stat(te.ctx, d)
		if err != nil {
			t.Fatalf("Error stating proxy store")
		}
	}

	localStatCount := (*localStats)["stat"]
	remoteStatCount := (*remoteStats)["stat"]
	if localStatCount != remoteBlobCount {
		t.Errorf("Unexpected local stat count")
	}

	if remoteStatCount != len(te.inRemote) {
		t.Errorf("Unexpected remote stat count")
	}

	// Get - pulls through blobs
	for _, dr := range te.inRemote {
		b, err := te.store.Get(te.ctx, dr)
		if err != nil {
			t.Fatalf("Error getting from proxy store: %s", err)
		}
		dl, err := digest.FromBytes(b)
		if err != nil {
			t.Fatalf("Error making digest from blob")
		}
		if dl != dr {
			t.Errorf("Mismatching blob fetch from proxy")
		}
	}

	if (*localStats)["get"] != remoteBlobCount {
		t.Errorf("Unexpected local get count")
	}

	if (*remoteStats)["get"] != remoteBlobCount {
		t.Errorf("Unexpected remote get count")
	}

	// This is gross, but the PUT is async, so there is a race with Stat.
	// Try runtime.Gosched or remove this test
	time.Sleep(500 * time.Millisecond)

	// Stat - stats only local
	for _, d := range te.inRemote {
		_, err := te.store.Stat(te.ctx, d)
		if err != nil {
			t.Fatalf("Error stating proxy store")
		}
	}

	if 2*localStatCount != (*localStats)["stat"] {
		t.Errorf("Unexpected local stat count after pull through")
	}

	if remoteBlobCount != (*remoteStats)["stat"] {
		t.Errorf("Unexpected remote stat count after pull through: %#v", te.RemoteStats())
	}
}

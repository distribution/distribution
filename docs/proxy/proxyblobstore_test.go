package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/proxy/scheduler"
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
	sbs.stats["create"]++
	return sbs.blobs.Create(ctx)
}

func (sbs statsBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	sbs.stats["resume"]++
	return sbs.blobs.Resume(ctx, id)
}

func (sbs statsBlobStore) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	sbs.stats["open"]++
	return sbs.blobs.Open(ctx, dgst)
}

func (sbs statsBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	sbs.stats["serveblob"]++
	return sbs.blobs.ServeBlob(ctx, w, r, dgst)
}

func (sbs statsBlobStore) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	sbs.stats["stat"]++
	return sbs.blobs.Stat(ctx, dgst)
}

func (sbs statsBlobStore) Delete(ctx context.Context, dgst digest.Digest) error {
	sbs.stats["delete"]++
	return sbs.blobs.Delete(ctx, dgst)
}

type testEnv struct {
	inRemote []distribution.Descriptor
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

	localRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider(), false, true, true)
	localRepo, err := localRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	truthRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider(), false, false, false)
	truthRepo, err := truthRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	truthBlobs := statsBlobStore{
		stats: make(map[string]int),
		blobs: truthRepo.Blobs(ctx),
	}

	localBlobs := statsBlobStore{
		stats: make(map[string]int),
		blobs: localRepo.Blobs(ctx),
	}

	s := scheduler.New(ctx, inmemory.New(), "/scheduler-state.json")

	proxyBlobStore := proxyBlobStore{
		remoteStore: truthBlobs,
		localStore:  localBlobs,
		scheduler:   s,
	}

	te := testEnv{
		store: proxyBlobStore,
		ctx:   ctx,
	}
	return te
}

func populate(t *testing.T, te *testEnv, blobCount int) {
	var inRemote []distribution.Descriptor
	for i := 0; i < blobCount; i++ {
		bytes := []byte(fmt.Sprintf("blob%d", i))

		desc, err := te.store.remoteStore.Put(te.ctx, "", bytes)
		if err != nil {
			t.Errorf("Put in store")
		}
		inRemote = append(inRemote, desc)
	}

	te.inRemote = inRemote

}

func TestProxyStoreStat(t *testing.T) {
	te := makeTestEnv(t, "foo/bar")
	remoteBlobCount := 1
	populate(t, &te, remoteBlobCount)

	localStats := te.LocalStats()
	remoteStats := te.RemoteStats()

	// Stat - touches both stores
	for _, d := range te.inRemote {
		_, err := te.store.Stat(te.ctx, d.Digest)
		if err != nil {
			t.Fatalf("Error stating proxy store")
		}
	}

	if (*localStats)["stat"] != remoteBlobCount {
		t.Errorf("Unexpected local stat count")
	}

	if (*remoteStats)["stat"] != remoteBlobCount {
		t.Errorf("Unexpected remote stat count")
	}
}

func TestProxyStoreServe(t *testing.T) {
	te := makeTestEnv(t, "foo/bar")
	remoteBlobCount := 1
	populate(t, &te, remoteBlobCount)

	localStats := te.LocalStats()
	remoteStats := te.RemoteStats()

	// Serveblob - pulls through blobs
	for _, dr := range te.inRemote {
		w := httptest.NewRecorder()
		r, err := http.NewRequest("GET", "", nil)
		if err != nil {
			t.Fatal(err)
		}

		err = te.store.ServeBlob(te.ctx, w, r, dr.Digest)
		if err != nil {
			t.Fatalf(err.Error())
		}

		dl, err := digest.FromBytes(w.Body.Bytes())
		if err != nil {
			t.Fatalf("Error making digest from blob")
		}
		if dl != dr.Digest {
			t.Errorf("Mismatching blob fetch from proxy")
		}
	}

	if (*localStats)["stat"] != remoteBlobCount && (*localStats)["create"] != remoteBlobCount {
		t.Fatalf("unexpected local stats")
	}
	if (*remoteStats)["stat"] != remoteBlobCount && (*remoteStats)["open"] != remoteBlobCount {
		t.Fatalf("unexpected local stats")
	}

	// Serveblob - blobs come from local
	for _, dr := range te.inRemote {
		w := httptest.NewRecorder()
		r, err := http.NewRequest("GET", "", nil)
		if err != nil {
			t.Fatal(err)
		}

		err = te.store.ServeBlob(te.ctx, w, r, dr.Digest)
		if err != nil {
			t.Fatalf(err.Error())
		}

		dl, err := digest.FromBytes(w.Body.Bytes())
		if err != nil {
			t.Fatalf("Error making digest from blob")
		}
		if dl != dr.Digest {
			t.Errorf("Mismatching blob fetch from proxy")
		}
	}

	// Stat to find local, but no new blobs were created
	if (*localStats)["stat"] != remoteBlobCount*2 && (*localStats)["create"] != remoteBlobCount*2 {
		t.Fatalf("unexpected local stats")
	}

	// Remote unchanged
	if (*remoteStats)["stat"] != remoteBlobCount && (*remoteStats)["open"] != remoteBlobCount {
		fmt.Printf("\tlocal=%#v, \n\tremote=%#v\n", localStats, remoteStats)
		t.Fatalf("unexpected local stats")
	}

}

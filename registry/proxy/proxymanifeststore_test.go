package proxy

import (
	"io"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/proxy/scheduler"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/cache/memory"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/docker/distribution/testutil"
	"github.com/docker/libtrust"
)

type statsManifest struct {
	manifests distribution.ManifestService
	stats     map[string]int
}

type manifestStoreTestEnv struct {
	manifestDigest digest.Digest // digest of the signed manifest in the local storage
	manifests      proxyManifestStore
}

func (te manifestStoreTestEnv) LocalStats() *map[string]int {
	ls := te.manifests.localManifests.(statsManifest).stats
	return &ls
}

func (te manifestStoreTestEnv) RemoteStats() *map[string]int {
	rs := te.manifests.remoteManifests.(statsManifest).stats
	return &rs
}

func (sm statsManifest) Delete(dgst digest.Digest) error {
	sm.stats["delete"]++
	return sm.manifests.Delete(dgst)
}

func (sm statsManifest) Exists(dgst digest.Digest) (bool, error) {
	sm.stats["exists"]++
	return sm.manifests.Exists(dgst)
}

func (sm statsManifest) ExistsByTag(tag string) (bool, error) {
	sm.stats["existbytag"]++
	return sm.manifests.ExistsByTag(tag)
}

func (sm statsManifest) Get(dgst digest.Digest) (*manifest.SignedManifest, error) {
	sm.stats["get"]++
	return sm.manifests.Get(dgst)
}

func (sm statsManifest) GetByTag(tag string, options ...distribution.ManifestServiceOption) (*manifest.SignedManifest, error) {
	sm.stats["getbytag"]++
	return sm.manifests.GetByTag(tag, options...)
}

func (sm statsManifest) Put(manifest *manifest.SignedManifest) error {
	sm.stats["put"]++
	return sm.manifests.Put(manifest)
}

func (sm statsManifest) Tags() ([]string, error) {
	sm.stats["tags"]++
	return sm.manifests.Tags()
}

func newManifestStoreTestEnv(t *testing.T, name, tag string) *manifestStoreTestEnv {
	ctx := context.Background()
	truthRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider(), false, false, false)
	truthRepo, err := truthRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	tr, err := truthRepo.Manifests(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	truthManifests := statsManifest{
		manifests: tr,
		stats:     make(map[string]int),
	}

	manifestDigest, err := populateRepo(t, ctx, truthRepo, name, tag)
	if err != nil {
		t.Fatalf(err.Error())
	}

	localRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider(), false, true, true)
	localRepo, err := localRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	lr, err := localRepo.Manifests(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	localManifests := statsManifest{
		manifests: lr,
		stats:     make(map[string]int),
	}

	s := scheduler.New(ctx, inmemory.New(), "/scheduler-state.json")
	return &manifestStoreTestEnv{
		manifestDigest: manifestDigest,
		manifests: proxyManifestStore{
			ctx:             ctx,
			localManifests:  localManifests,
			remoteManifests: truthManifests,
			scheduler:       s,
		},
	}
}

func populateRepo(t *testing.T, ctx context.Context, repository distribution.Repository, name, tag string) (digest.Digest, error) {
	m := manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: name,
		Tag:  tag,
	}

	for i := 0; i < 2; i++ {
		wr, err := repository.Blobs(ctx).Create(ctx)
		if err != nil {
			t.Fatalf("unexpected error creating test upload: %v", err)
		}

		rs, ts, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("unexpected error generating test layer file")
		}
		dgst := digest.Digest(ts)
		if _, err := io.Copy(wr, rs); err != nil {
			t.Fatalf("unexpected error copying to upload: %v", err)
		}

		if _, err := wr.Commit(ctx, distribution.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	sm, err := manifest.Sign(&m, pk)
	if err != nil {
		t.Fatalf("error signing manifest: %v", err)
	}

	ms, err := repository.Manifests(ctx)
	if err != nil {
		t.Fatalf(err.Error())
	}
	ms.Put(sm)
	if err != nil {
		t.Fatalf("unexpected errors putting manifest: %v", err)
	}
	pl, err := sm.Payload()
	if err != nil {
		t.Fatal(err)
	}
	return digest.FromBytes(pl)
}

// TestProxyManifests contains basic acceptance tests
// for the pull-through behavior
func TestProxyManifests(t *testing.T) {
	name := "foo/bar"
	env := newManifestStoreTestEnv(t, name, "latest")

	localStats := env.LocalStats()
	remoteStats := env.RemoteStats()

	// Stat - must check local and remote
	exists, err := env.manifests.ExistsByTag("latest")
	if err != nil {
		t.Fatalf("Error checking existance")
	}
	if !exists {
		t.Errorf("Unexpected non-existant manifest")
	}

	if (*localStats)["existbytag"] != 1 && (*remoteStats)["existbytag"] != 1 {
		t.Errorf("Unexpected exists count")
	}

	// Get - should succeed and pull manifest into local
	_, err = env.manifests.Get(env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if (*localStats)["get"] != 1 && (*remoteStats)["get"] != 1 {
		t.Errorf("Unexpected get count")
	}

	if (*localStats)["put"] != 1 {
		t.Errorf("Expected local put")
	}

	// Stat - should only go to local
	exists, err = env.manifests.ExistsByTag("latest")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("Unexpected non-existant manifest")
	}

	if (*localStats)["existbytag"] != 2 && (*remoteStats)["existbytag"] != 1 {
		t.Errorf("Unexpected exists count")

	}

	// Get - should get from remote, to test freshness
	_, err = env.manifests.Get(env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}

	if (*remoteStats)["get"] != 2 && (*remoteStats)["existsbytag"] != 1 && (*localStats)["put"] != 1 {
		t.Errorf("Unexpected get count")
	}

}

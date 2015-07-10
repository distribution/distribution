package proxy

import (
	"io"
	"testing"

	"fmt"
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
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
	manifests proxyManifestStore
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

func (sm statsManifest) GetByTag(tag string) (*manifest.SignedManifest, error) {
	sm.stats["getbytag"]++
	return sm.manifests.GetByTag(tag)
}

func (sm statsManifest) Put(manifest *manifest.SignedManifest, verifyFunc distribution.ManifestVerifyFunc) error {
	sm.stats["put"]++
	return sm.manifests.Put(manifest, verifyFunc)
}

func (sm statsManifest) Tags() ([]string, error) {
	sm.stats["tags"]++
	return sm.manifests.Tags()
}

func newManifestStoreTestEnv(t *testing.T, name, tag string) *manifestStoreTestEnv {
	ctx := context.Background()
	truthRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider())
	truthRepo, err := truthRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	truthManifests := statsManifest{
		manifests: truthRepo.Manifests(),
		stats:     make(map[string]int),
	}

	populateRepo(t, ctx, truthRepo, name, tag)

	localRegistry := storage.NewRegistryWithDriver(ctx, inmemory.New(), memory.NewInMemoryBlobDescriptorCacheProvider())
	localRepo, err := localRegistry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	localManifests := statsManifest{
		manifests: localRepo.Manifests(),
		stats:     make(map[string]int),
	}

	return &manifestStoreTestEnv{
		manifests: proxyManifestStore{
			ctx:             ctx,
			localManifests:  localManifests,
			remoteManifests: truthManifests,
		},
	}
}

func populateRepo(t *testing.T, ctx context.Context, repository distribution.Repository, name, tag string) {
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

	err = repository.Manifests().Put(sm, storage.VerifyLocalManifest)
	if err != nil {
		t.Fatalf("unexpected errors putting manifest: %v", err)
	}
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
	_, err = env.manifests.GetByTag("latest")
	if err != nil {
		t.Fatalf("Unexpected error getting by tag")
	}
	if (*localStats)["getbytag"] != 1 && (*remoteStats)["getbytag"] != 1 {
		t.Errorf("Unexpected get count")
	}

	if (*localStats)["put"] != 1 {
		t.Errorf("Expected local put")
	}

	// Stat - should only go to local
	exists, err = env.manifests.ExistsByTag("latest")
	if err != nil {
		t.Fatalf("Unexpected error getting by tag")
	}
	if !exists {
		t.Errorf("Unexpected non-existant manifest")
	}

	if (*localStats)["existbytag"] != 2 && (*remoteStats)["existbytag"] != 1 {
		t.Errorf("Unexpected exists count")

	}

	// Get - should get from remote, to test freshness
	_, err = env.manifests.GetByTag("latest")
	if err != nil {
		t.Fatalf("Error getting manifest by digest")
	}
	fmt.Printf("\tlocal=%#v, \n\tremote=%#v\n", localStats, remoteStats)

	if (*remoteStats)["getbytag"] != 2 && (*remoteStats)["existsbytag"] != 1 && (*localStats)["put"] != 1 {
		t.Errorf("Unexpected get count")
	}

}

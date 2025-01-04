package proxy

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/client/auth"
	"github.com/distribution/distribution/v3/internal/client/auth/challenge"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/proxy/scheduler"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type statsManifest struct {
	manifests distribution.ManifestService
	stats     map[string]int
}

type manifestStoreTestEnv struct {
	manifestDigest digest.Digest // digest of the signed manifest in the local storage
	manifestSize   uint64
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

func (sm statsManifest) Delete(ctx context.Context, dgst digest.Digest) error {
	sm.stats["delete"]++
	return sm.manifests.Delete(ctx, dgst)
}

func (sm statsManifest) Exists(ctx context.Context, dgst digest.Digest) (bool, error) {
	sm.stats["exists"]++
	return sm.manifests.Exists(ctx, dgst)
}

func (sm statsManifest) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	sm.stats["get"]++
	return sm.manifests.Get(ctx, dgst)
}

func (sm statsManifest) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (digest.Digest, error) {
	sm.stats["put"]++
	return sm.manifests.Put(ctx, manifest)
}

type mockChallenger struct {
	sync.Mutex
	count int
}

// Called for remote operations only
func (m *mockChallenger) tryEstablishChallenges(context.Context) error {
	m.Lock()
	defer m.Unlock()
	m.count++
	return nil
}

func (m *mockChallenger) credentialStore() auth.CredentialStore {
	return nil
}

func (m *mockChallenger) challengeManager() challenge.Manager {
	return nil
}

func newManifestStoreTestEnv(t *testing.T, name, tag string) *manifestStoreTestEnv {
	nameRef, err := reference.WithName(name)
	if err != nil {
		t.Fatalf("unable to parse reference: %s", err)
	}

	ctx := context.Background()
	truthRegistry, err := storage.NewRegistry(ctx, inmemory.New(),
		storage.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)))
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	truthRepo, err := truthRegistry.Repository(ctx, nameRef)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	tr, err := truthRepo.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	truthManifests := statsManifest{
		manifests: tr,
		stats:     make(map[string]int),
	}

	manifestDigest, manifestSize, err := populateRepo(ctx, t, truthRepo, name, tag)
	if err != nil {
		t.Fatal(err)
	}

	localRegistry, err := storage.NewRegistry(ctx, inmemory.New(),
		storage.BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), storage.EnableRedirect, storage.DisableDigestResumption)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	localRepo, err := localRegistry.Repository(ctx, nameRef)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	lr, err := localRepo.Manifests(ctx, storage.SkipLayerVerification())
	if err != nil {
		t.Fatal(err)
	}

	localManifests := statsManifest{
		manifests: lr,
		stats:     make(map[string]int),
	}

	s := scheduler.New(ctx, inmemory.New(), "/scheduler-state.json")
	return &manifestStoreTestEnv{
		manifestDigest: manifestDigest,
		manifestSize:   manifestSize,
		manifests: proxyManifestStore{
			ctx:             ctx,
			localManifests:  localManifests,
			remoteManifests: truthManifests,
			scheduler:       s,
			repositoryName:  nameRef,
			authChallenger:  &mockChallenger{},
		},
	}
}

func populateRepo(ctx context.Context, t *testing.T, repository distribution.Repository, name, tag string) (manifestDigest digest.Digest, manifestSize uint64, _ error) {
	config := []byte(`{"name": "foo"}`)
	configDigest := digest.FromBytes(config)
	configReader := bytes.NewReader(config)

	if err := testutil.PushBlob(ctx, repository, configReader, configDigest); err != nil {
		t.Fatal(err)
	}

	m := schema2.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: schema2.MediaTypeManifest,
		Config: v1.Descriptor{
			MediaType: "foo/bar",
			Digest:    configDigest,
		},
	}

	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatal("unexpected error generating test layer file")
		}

		if err := testutil.PushBlob(ctx, repository, rs, dgst); err != nil {
			t.Fatal(err)
		}
	}

	ms, err := repository.Manifests(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sm, err := schema2.FromStruct(m)
	if err != nil {
		t.Fatal(err)
	}
	smJSON, err := sm.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	dgst, err := ms.Put(ctx, sm)
	if err != nil {
		t.Fatalf("unexpected errors putting manifest: %v", err)
	}

	return dgst, uint64(len(smJSON)), nil
}

// TestProxyManifests contains basic acceptance tests
// for the pull-through behavior
func TestProxyManifests(t *testing.T) {
	name := "foo/bar"
	env := newManifestStoreTestEnv(t, name, "latest")

	localStats := env.LocalStats()
	remoteStats := env.RemoteStats()

	ctx := context.Background()
	// Stat - must check local and remote
	exists, err := env.manifests.Exists(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal("Error checking existence")
	}
	if !exists {
		t.Errorf("Unexpected non-existent manifest")
	}

	if (*localStats)["exists"] != 1 && (*remoteStats)["exists"] != 1 {
		t.Errorf("Unexpected exists count : \n%v \n%v", localStats, remoteStats)
	}

	if env.manifests.authChallenger.(*mockChallenger).count != 1 {
		t.Fatalf("Expected 1 auth challenge, got %#v", env.manifests.authChallenger)
	}

	// Get - should succeed and pull manifest into local
	_, err = env.manifests.Get(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}

	if (*localStats)["get"] != 1 && (*remoteStats)["get"] != 1 {
		t.Errorf("Unexpected get count")
	}

	if (*localStats)["put"] != 1 {
		t.Errorf("Expected local put")
	}

	if env.manifests.authChallenger.(*mockChallenger).count != 2 {
		t.Fatalf("Expected 2 auth challenges, got %#v", env.manifests.authChallenger)
	}

	// Stat - should only go to local
	exists, err = env.manifests.Exists(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Errorf("Unexpected non-existent manifest")
	}

	if (*localStats)["exists"] != 2 && (*remoteStats)["exists"] != 1 {
		t.Errorf("Unexpected exists count")
	}

	if env.manifests.authChallenger.(*mockChallenger).count != 2 {
		t.Fatalf("Expected 2 auth challenges, got %#v", env.manifests.authChallenger)
	}

	// Get proxied - won't require another authchallenge
	_, err = env.manifests.Get(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}

	if env.manifests.authChallenger.(*mockChallenger).count != 2 {
		t.Fatalf("Expected 2 auth challenges, got %#v", env.manifests.authChallenger)
	}
}

func TestProxyManifestsWithoutScheduler(t *testing.T) {
	name := "foo/bar"
	env := newManifestStoreTestEnv(t, name, "latest")
	env.manifests.scheduler = nil

	ctx := context.Background()
	exists, err := env.manifests.Exists(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal("Error checking existence")
	}
	if !exists {
		t.Errorf("Unexpected non-existent manifest")
	}

	// Get - should succeed without scheduler
	_, err = env.manifests.Get(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
}

func TestProxyManifestsMetrics(t *testing.T) {
	proxyMetrics = &proxyMetricsCollector{}
	name := "foo/bar"
	env := newManifestStoreTestEnv(t, name, "latest")

	ctx := context.Background()
	// Get - should succeed and pull manifest into local
	_, err := env.manifests.Get(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}

	if proxyMetrics.manifestMetrics.Requests != 1 {
		t.Errorf("Expected manifestMetrics.Requests %d but got %d", 1, proxyMetrics.manifestMetrics.Requests)
	}
	if proxyMetrics.manifestMetrics.Hits != 0 {
		t.Errorf("Expected manifestMetrics.Hits %d but got %d", 0, proxyMetrics.manifestMetrics.Hits)
	}
	if proxyMetrics.manifestMetrics.Misses != 1 {
		t.Errorf("Expected manifestMetrics.Misses %d but got %d", 1, proxyMetrics.manifestMetrics.Misses)
	}
	if proxyMetrics.manifestMetrics.BytesPulled != env.manifestSize {
		t.Errorf("Expected manifestMetrics.BytesPulled %d but got %d", env.manifestSize, proxyMetrics.manifestMetrics.BytesPulled)
	}
	if proxyMetrics.manifestMetrics.BytesPushed != env.manifestSize {
		t.Errorf("Expected manifestMetrics.BytesPushed %d but got %d", env.manifestSize, proxyMetrics.manifestMetrics.BytesPushed)
	}

	// Get proxied - manifest comes from local
	_, err = env.manifests.Get(ctx, env.manifestDigest)
	if err != nil {
		t.Fatal(err)
	}

	if proxyMetrics.manifestMetrics.Requests != 2 {
		t.Errorf("Expected manifestMetrics.Requests %d but got %d", 2, proxyMetrics.manifestMetrics.Requests)
	}
	if proxyMetrics.manifestMetrics.Hits != 1 {
		t.Errorf("Expected manifestMetrics.Hits %d but got %d", 1, proxyMetrics.manifestMetrics.Hits)
	}
	if proxyMetrics.manifestMetrics.Misses != 1 {
		t.Errorf("Expected manifestMetrics.Misses %d but got %d", 1, proxyMetrics.manifestMetrics.Misses)
	}
	if proxyMetrics.manifestMetrics.BytesPulled != env.manifestSize {
		t.Errorf("Expected manifestMetrics.BytesPulled %d but got %d", env.manifestSize, proxyMetrics.manifestMetrics.BytesPulled)
	}
	if proxyMetrics.manifestMetrics.BytesPushed != (env.manifestSize * 2) {
		t.Errorf("Expected manifestMetrics.BytesPushed %d but got %d", env.manifestSize*2, proxyMetrics.manifestMetrics.BytesPushed)
	}
}

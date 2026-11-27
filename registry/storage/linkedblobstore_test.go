package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestLinkedBlobStoreEnumerator(t *testing.T) {
	fooRepoName, _ := reference.WithName("nm/foo")
	fooEnv := newManifestStoreTestEnv(t, fooRepoName, "thetag")
	ctx := context.Background()

	expected := make([]string, 0, 2)
	for range 2 {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatal("unexpected error generating test layer file")
		}

		expected = append(expected, dgst.String())

		wr, err := fooEnv.repository.Blobs(fooEnv.ctx).Create(fooEnv.ctx)
		if err != nil {
			t.Fatalf("unexpected error creating test upload: %v", err)
		}

		if _, err := io.Copy(wr, rs); err != nil {
			t.Fatalf("unexpected error copying to upload: %v", err)
		}

		if _, err := wr.Commit(fooEnv.ctx, v1.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
	}

	enumerator, ok := fooEnv.repository.Blobs(fooEnv.ctx).(distribution.BlobEnumerator)
	if !ok {
		t.Fatal("Blobs is not a BlobEnumerator")
	}

	var actual []string
	if err := enumerator.Enumerate(ctx, func(dgst digest.Digest) error {
		actual = append(actual, dgst.String())
		return nil
	}); err != nil {
		t.Fatalf("cannot enumerate on repository: %v", err)
	}

	sort.Strings(actual)
	sort.Strings(expected)
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("unexpected array difference (expected: %v actual: %v)", expected, actual)
	}
}

func TestLinkedBlobStoreCreateWithMountFrom(t *testing.T) {
	fooRepoName, _ := reference.WithName("nm/foo")
	fooEnv := newManifestStoreTestEnv(t, fooRepoName, "thetag")
	ctx := context.Background()
	stats, err := mockRegistry(t, fooEnv.registry)
	if err != nil {
		t.Fatal(err)
	}

	// Build up some test layers and add them to the manifest, saving the
	// readseekers for upload later.
	testLayers := map[digest.Digest]io.ReadSeeker{}
	for range 2 {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatal("unexpected error generating test layer file")
		}

		testLayers[dgst] = rs
	}

	// upload the layers to foo/bar
	for dgst, rs := range testLayers {
		wr, err := fooEnv.repository.Blobs(fooEnv.ctx).Create(fooEnv.ctx)
		if err != nil {
			t.Fatalf("unexpected error creating test upload: %v", err)
		}

		if _, err := io.Copy(wr, rs); err != nil {
			t.Fatalf("unexpected error copying to upload: %v", err)
		}

		if _, err := wr.Commit(fooEnv.ctx, v1.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
	}

	// create another repository nm/bar
	barRepoName, _ := reference.WithName("nm/bar")
	barRepo, err := fooEnv.registry.Repository(ctx, barRepoName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	// cross-repo mount the test layers into a nm/bar
	for dgst := range testLayers {
		fooCanonical, _ := reference.WithDigest(fooRepoName, dgst)
		option := WithMountFrom(fooCanonical)
		// ensure we can instrospect it
		createOpts := distribution.CreateOptions{}
		if err := option.Apply(&createOpts); err != nil {
			t.Fatalf("failed to apply MountFrom option: %v", err)
		}
		if !createOpts.Mount.ShouldMount || createOpts.Mount.From.String() != fooCanonical.String() {
			t.Fatalf("unexpected create options: %#+v", createOpts.Mount)
		}

		_, err := barRepo.Blobs(ctx).Create(ctx, WithMountFrom(fooCanonical))
		if err == nil {
			t.Fatalf("unexpected non-error while mounting from %q: %v", fooRepoName.String(), err)
		}
		if _, ok := err.(distribution.ErrBlobMounted); !ok {
			t.Fatalf("expected ErrMountFrom error, not %T: %v", err, err)
		}
	}
	for dgst := range testLayers {
		fooCanonical, _ := reference.WithDigest(fooRepoName, dgst)
		count, exists := stats[fooCanonical.String()]
		if !exists {
			t.Errorf("expected entry %q not found among handled stat calls", fooCanonical.String())
		} else if count != 1 {
			t.Errorf("expected exactly one stat call for entry %q, not %d", fooCanonical.String(), count)
		}
	}

	clearStats(stats)

	// create yet another repository nm/baz
	bazRepoName, _ := reference.WithName("nm/baz")
	bazRepo, err := fooEnv.registry.Repository(ctx, bazRepoName)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}

	// cross-repo mount them into a nm/baz and provide a prepopulated blob descriptor
	for dgst := range testLayers {
		fooCanonical, _ := reference.WithDigest(fooRepoName, dgst)
		size, err := strconv.ParseInt("0x"+dgst.Encoded()[:8], 0, 64)
		if err != nil {
			t.Fatal(err)
		}
		prepolutatedDescriptor := v1.Descriptor{
			Digest:    dgst,
			Size:      size,
			MediaType: "application/octet-stream",
		}
		_, err = bazRepo.Blobs(ctx).Create(ctx, WithMountFrom(fooCanonical), &statCrossMountCreateOption{
			desc: prepolutatedDescriptor,
		})
		blobMounted, ok := err.(distribution.ErrBlobMounted)
		if !ok {
			t.Errorf("expected ErrMountFrom error, not %T: %v", err, err)
			continue
		}
		if !reflect.DeepEqual(blobMounted.Descriptor, prepolutatedDescriptor) {
			t.Errorf("unexpected descriptor: %#+v != %#+v", blobMounted.Descriptor, prepolutatedDescriptor)
		}
	}
	// this time no stat calls will be made
	if len(stats) != 0 {
		t.Errorf("unexpected number of stats made: %d != %d", len(stats), len(testLayers))
	}
}

func clearStats(stats map[string]int) {
	for k := range stats {
		delete(stats, k)
	}
}

// mockRegistry sets a mock blob descriptor service factory that overrides
// statter's Stat method to note each attempt to stat a blob in any repository.
// Returned stats map contains canonical references to blobs with a number of
// attempts.
func mockRegistry(t *testing.T, nm distribution.Namespace) (map[string]int, error) {
	registry, ok := nm.(*registry)
	if !ok {
		return nil, fmt.Errorf("not an expected type of registry: %T", nm)
	}
	stats := make(map[string]int)

	registry.blobDescriptorServiceFactory = &mockBlobDescriptorServiceFactory{
		t:     t,
		stats: stats,
	}

	return stats, nil
}

type mockBlobDescriptorServiceFactory struct {
	t     *testing.T
	stats map[string]int
}

func (f *mockBlobDescriptorServiceFactory) BlobAccessController(svc distribution.BlobDescriptorService) distribution.BlobDescriptorService {
	return &mockBlobDescriptorService{
		BlobDescriptorService: svc,
		t:                     f.t,
		stats:                 f.stats,
	}
}

type mockBlobDescriptorService struct {
	distribution.BlobDescriptorService
	t     *testing.T
	stats map[string]int
}

var _ distribution.BlobDescriptorService = &mockBlobDescriptorService{}

func (bs *mockBlobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	statter, ok := bs.BlobDescriptorService.(*linkedBlobStatter)
	if !ok {
		return v1.Descriptor{}, fmt.Errorf("unexpected blob descriptor service: %T", bs.BlobDescriptorService)
	}

	name := statter.repository.Named()
	canonical, err := reference.WithDigest(name, dgst)
	if err != nil {
		return v1.Descriptor{}, fmt.Errorf("failed to make canonical reference: %v", err)
	}

	bs.stats[canonical.String()]++
	bs.t.Logf("calling Stat on %s", canonical.String())

	return bs.BlobDescriptorService.Stat(ctx, dgst)
}

// statCrossMountCreateOptions ensures the expected options type is passed, and optionally pre-fills the cross-mount stat info
type statCrossMountCreateOption struct {
	desc v1.Descriptor
}

var _ distribution.BlobCreateOption = statCrossMountCreateOption{}

func (f statCrossMountCreateOption) Apply(v any) error {
	opts, ok := v.(*distribution.CreateOptions)
	if !ok {
		return fmt.Errorf("Unexpected create options: %#v", v)
	}

	if !opts.Mount.ShouldMount {
		return nil
	}

	opts.Mount.Stat = &f.desc

	return nil
}

// putErrorDriver fails a selected write before changing storage.
type putErrorDriver struct {
	driver.StorageDriver
	failSuffix string
	err        error
}

func (d *putErrorDriver) PutContent(ctx context.Context, path string, content []byte) error {
	if d.err != nil && strings.HasSuffix(path, d.failSuffix) {
		return d.err
	}
	return d.StorageDriver.PutContent(ctx, path, content)
}

func TestLinkedBlobStorePutFailure(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, mediaType := range []string{
			"application/octet-stream",
			schema2.MediaTypeManifest,
			v1.MediaTypeImageManifest,
			manifestlist.MediaTypeManifestList,
			v1.MediaTypeImageIndex,
		} {
			for _, failSuffix := range []string{"/data", "/link"} {
				t.Run(fmt.Sprintf("cached=%t/%s/fail=%s", cached, mediaType, failSuffix), func(t *testing.T) {
					ctx := context.Background()
					writeErr := errors.New("storage write failed")
					drvr := &putErrorDriver{StorageDriver: inmemory.New(), failSuffix: failSuffix, err: writeErr}
					var options []RegistryOption
					var repoCache distribution.BlobDescriptorService
					if cached {
						provider := memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)
						var err error
						repoCache, err = provider.RepositoryScoped("test/put")
						if err != nil {
							t.Fatal(err)
						}
						options = append(options, BlobDescriptorCacheProvider(provider))
					}
					repo := makeRepository(t, createRegistry(t, drvr, options...), "test/put")
					blobs := repo.Blobs(ctx)
					payload := []byte("small blob")
					put := func() (v1.Descriptor, error) { return blobs.Put(ctx, mediaType, payload) }
					get := func(dgst digest.Digest) ([]byte, error) { return blobs.Get(ctx, dgst) }
					var manifests distribution.ManifestService
					if mediaType != "application/octet-stream" {
						var err error
						manifests, err = repo.Manifests(ctx, SkipLayerVerification())
						if err != nil {
							t.Fatal(err)
						}
						switch mediaType {
						case manifestlist.MediaTypeManifestList, v1.MediaTypeImageIndex:
							payload = fmt.Appendf(nil, `{"schemaVersion":2,"mediaType":%q,"manifests":[]}`, mediaType)
						default:
							payload = fmt.Appendf(nil, `{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":%q,"size":2,"digest":%q},"layers":[]}`, mediaType, v1.MediaTypeImageConfig, digest.FromString("{}"))
						}
						manifest, _, err := distribution.UnmarshalManifest(mediaType, payload)
						if err != nil {
							t.Fatal(err)
						}
						put = func() (v1.Descriptor, error) {
							_, err := manifests.Put(ctx, manifest)
							return v1.Descriptor{}, err
						}
						get = func(dgst digest.Digest) ([]byte, error) {
							m, err := manifests.Get(ctx, dgst)
							if err != nil {
								return nil, err
							}
							_, p, err := m.Payload()
							return p, err
						}
					}
					dgst := digest.FromBytes(payload)
					desc, err := put()
					if !errors.Is(err, writeErr) {
						t.Fatalf("Put error = %v, want %v", err, writeErr)
					}
					if !reflect.DeepEqual(desc, v1.Descriptor{}) {
						t.Errorf("failed Put returned descriptor %#v", desc)
					}
					drvr.err = nil
					if _, err := blobs.Stat(ctx, dgst); err != distribution.ErrBlobUnknown {
						t.Errorf("Stat after failed Put = %v, want ErrBlobUnknown", err)
					}
					if manifests != nil {
						if exists, err := manifests.Exists(ctx, dgst); err != nil || exists {
							t.Errorf("Exists after failed Put = %t, %v, want false, nil", exists, err)
						}
					}
					if p, err := get(dgst); err == nil {
						t.Errorf("Get after failed Put returned %q", p)
					}
					// Retry on the same owner, including the already-written global blob.
					if _, err := put(); err != nil {
						t.Fatal(err)
					}
					if cached {
						if desc, err := repoCache.Stat(ctx, dgst); err != nil || desc.Digest != dgst {
							t.Errorf("successful Put not cached: %#v, %v", desc, err)
						}
					}
					if p, err := get(dgst); err != nil || !bytes.Equal(p, payload) {
						t.Errorf("Get after retry = %q, %v, want %q", p, err, payload)
					}
				})
			}
		}
	}
}

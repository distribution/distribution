package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/opencontainers/go-digest"
)

func TestLinkedBlobStoreEnumerator(t *testing.T) {
	fooRepoName, _ := reference.WithName("nm/foo")
	fooEnv := newManifestStoreTestEnv(t, fooRepoName, "thetag")
	ctx := context.Background()

	var expected []string
	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("unexpected error generating test layer file")
		}

		expected = append(expected, dgst.String())

		wr, err := fooEnv.repository.Blobs(fooEnv.ctx).Create(fooEnv.ctx)
		if err != nil {
			t.Fatalf("unexpected error creating test upload: %v", err)
		}

		if _, err := io.Copy(wr, rs); err != nil {
			t.Fatalf("unexpected error copying to upload: %v", err)
		}

		if _, err := wr.Commit(fooEnv.ctx, distribution.Descriptor{Digest: dgst}); err != nil {
			t.Fatalf("unexpected error finishing upload: %v", err)
		}
	}

	enumerator, ok := fooEnv.repository.Blobs(fooEnv.ctx).(distribution.BlobEnumerator)
	if !ok {
		t.Fatalf("Blobs is not a BlobEnumerator")
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
	for i := 0; i < 2; i++ {
		rs, dgst, err := testutil.CreateRandomTarFile()
		if err != nil {
			t.Fatalf("unexpected error generating test layer file")
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

		if _, err := wr.Commit(fooEnv.ctx, distribution.Descriptor{Digest: dgst}); err != nil {
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
		mount, ok := createOpts.Mount.(distribution.FromMount)
		if !ok {
			t.Fatalf("Expected mount to be FromMount")
		}
		if mount.From.String() != fooCanonical.String() {
			t.Fatalf("unexpected create options: %#+v", createOpts.Mount)
		}

		_, err := barRepo.Blobs(ctx).Create(ctx, WithMountFrom(fooCanonical))
		if err == nil {
			t.Fatalf("unexpected non-error while mounting from %q: %v", fooRepoName.String(), err)
		}
		if !errors.As(err, &distribution.ErrBlobMounted{}) {
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

func (bs *mockBlobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	statter, ok := bs.BlobDescriptorService.(*linkedBlobStatter)
	if !ok {
		return distribution.Descriptor{}, fmt.Errorf("unexpected blob descriptor service: %T", bs.BlobDescriptorService)
	}

	name := statter.repository.Named()
	canonical, err := reference.WithDigest(name, dgst)
	if err != nil {
		return distribution.Descriptor{}, fmt.Errorf("failed to make canonical reference: %v", err)
	}

	bs.stats[canonical.String()]++
	bs.t.Logf("calling Stat on %s", canonical.String())

	return bs.BlobDescriptorService.Stat(ctx, dgst)
}

package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/distribution/reference"
	"github.com/docker/distribution"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
	"github.com/opencontainers/go-digest"
)

// FuzzProxyCachePoisoning drives the pull-through cache against an upstream
// registry that answers badly.
//
// Threat model coverage (registry-threat-model.md), harness 5: handling of the
// upstream registry's responses by the reworked registry/proxy package, and
// TM-06, substitution of the upstream registry when proxying, assessed as
// critical. In Proxy mode this cache is what every node in the cluster pulls
// from, and it is filled by a registry outside the cluster's control.
//
// The invariant is the one a content-addressed cache cannot give up: whatever
// ends up in the local store under a digest must hash to that digest. A cache
// that stores the upstream's bytes under the digest that was *asked for*,
// without checking, is poisoned for every later client -- and the poisoning
// outlives the connection that caused it, because the next pull is served from
// the local copy without ever reaching the upstream again.
//
// The upstream here is hostile in the two ways it can be: the descriptor it
// reports from Stat (a digest, a size, and a media type it chooses) and the
// bytes it serves from Open. Those are independent, so the fuzzer can make the
// bytes disagree with the descriptor, the descriptor disagree with the request,
// or both.
func FuzzProxyCachePoisoning(f *testing.F) {
	// content, digestChoice, sizeDelta
	f.Add([]byte("upstream content"), byte(0), int64(0))
	f.Add([]byte("upstream content"), byte(1), int64(0))
	f.Add([]byte("upstream content"), byte(2), int64(0))
	f.Add([]byte("upstream content"), byte(0), int64(1))
	f.Add([]byte("upstream content"), byte(0), int64(-1))
	f.Add([]byte("upstream content"), byte(1), int64(-1))
	f.Add([]byte(""), byte(0), int64(0))
	f.Add([]byte(""), byte(1), int64(0))
	f.Add([]byte("a"), byte(2), int64(1024))
	f.Add(bytes.Repeat([]byte("x"), 4096), byte(1), int64(0))
	f.Add([]byte("upstream content"), byte(0), int64(-1<<40))

	f.Fuzz(func(t *testing.T, content []byte, digestChoice byte, sizeDelta int64) {
		if len(content) > 1<<16 {
			return
		}

		// The digest the client asks for. It is fixed, so that "what was asked
		// for" is never in question.
		requested := digest.FromString("the blob the client asked for")

		// What the upstream claims when asked to Stat the requested digest.
		var claimed digest.Digest
		switch digestChoice % 3 {
		case 0:
			claimed = requested
		case 1:
			claimed = digest.FromBytes(content)
		default:
			claimed = digest.FromString("something else entirely")
		}

		size := int64(len(content)) + sizeDelta

		env := newCacheEnv(t, &hostileBlobService{
			descriptor: distribution.Descriptor{
				Digest:    claimed,
				Size:      size,
				MediaType: "application/octet-stream",
			},
			content: content,
		})

		// storeLocal is the caching path. Driving it directly rather than through
		// ServeBlob keeps the iteration deterministic: ServeBlob starts it in a
		// goroutine, and the assertion below is about what the store holds when
		// it has finished.
		storeErr := env.store.storeLocal(env.ctx, requested)

		// Whatever the outcome, the store must not hold content under a digest
		// that does not describe it. Check both the digest that was asked for and
		// the one the upstream claimed: committing under either is a cache entry
		// a later client will be served.
		for _, dgst := range dedupeDigests(requested, claimed) {
			stored, err := env.local.Get(env.ctx, dgst)
			if err != nil {
				// Nothing under this digest, which is the expected outcome for
				// every hostile combination.
				continue
			}

			if actual := digest.FromBytes(stored); actual != dgst {
				t.Fatalf("the cache holds %d bytes under %s, but those bytes hash to %s\n"+
					"\tclient asked for: %s\n"+
					"\tupstream claimed: %s (size %d)\n"+
					"\tupstream served:  %d bytes\n"+
					"\tstoreLocal error: %v\n"+
					"every later pull of %s is served this content from the local copy, "+
					"without the upstream being consulted again",
					len(stored), dgst, actual, requested, claimed, size, len(content), storeErr, dgst)
			}

			// A blob stored under the requested digest must be the content the
			// upstream served, not a truncation of it: copyContent copies
			// exactly the claimed size, so a claimed size shorter than the body
			// would otherwise store a prefix under a digest that happens to
			// match nothing.
			if dgst == requested && !bytes.Equal(stored, content) {
				t.Fatalf("the cache holds %d bytes under the requested digest %s, but the "+
					"upstream served %d bytes; the stored copy is not what was fetched",
					len(stored), requested, len(content))
			}
		}
	})
}

func dedupeDigests(digests ...digest.Digest) []digest.Digest {
	seen := make(map[digest.Digest]struct{}, len(digests))
	unique := make([]digest.Digest, 0, len(digests))
	for _, dgst := range digests {
		if _, ok := seen[dgst]; ok {
			continue
		}
		seen[dgst] = struct{}{}
		unique = append(unique, dgst)
	}
	return unique
}

// cacheEnv is a proxy blob store with a real local registry and a caller-chosen
// upstream.
type cacheEnv struct {
	store cachedBlobStore
	local distribution.BlobStore
	ctx   context.Context
}

func newCacheEnv(t *testing.T, remote distribution.BlobService) *cacheEnv {
	t.Helper()

	ctx := context.Background()

	localName, err := reference.WithName("local/cache")
	if err != nil {
		t.Fatalf("cannot parse the local name: %v", err)
	}
	remoteName, err := reference.WithName("remote/truth")
	if err != nil {
		t.Fatalf("cannot parse the remote name: %v", err)
	}

	localRegistry, err := storage.NewRegistry(ctx, inmemory.New(),
		storage.BlobDescriptorCacheProvider(nil),
		storage.EnableRedirect,
		storage.DisableDigestResumption)
	if err != nil {
		t.Fatalf("cannot create the local registry: %v", err)
	}

	localRepo, err := localRegistry.Repository(ctx, localName)
	if err != nil {
		t.Fatalf("cannot create the local repository: %v", err)
	}
	localBlobs := localRepo.Blobs(ctx)

	return &cacheEnv{
		store: cachedBlobStore{
			blobStore: blobStore{
				remoteRepositoryName: remoteName,
				remoteStore:          remote,
				authChallenger:       &mockChallenger{},
			},
			localRepositoryName: localName,
			localStore:          localBlobs,
			// No scheduler: expiry is not what this target is about, and
			// storeLocal does not touch it.
			scheduler: nil,
		},
		local: localBlobs,
		ctx:   ctx,
	}
}

// hostileBlobService is an upstream registry whose descriptor and bytes are
// chosen independently of each other and of what was requested.
type hostileBlobService struct {
	descriptor distribution.Descriptor
	content    []byte
}

var _ distribution.BlobService = &hostileBlobService{}

func (s *hostileBlobService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	return s.descriptor, nil
}

func (s *hostileBlobService) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	return s.content, nil
}

func (s *hostileBlobService) Open(ctx context.Context, dgst digest.Digest) (io.ReadSeekCloser, error) {
	return nopSeekCloser{bytes.NewReader(s.content)}, nil
}

func (s *hostileBlobService) Put(ctx context.Context, mediaType string, p []byte) (distribution.Descriptor, error) {
	return distribution.Descriptor{}, fmt.Errorf("not implemented")
}

func (s *hostileBlobService) Create(ctx context.Context, options ...distribution.BlobCreateOption) (distribution.BlobWriter, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *hostileBlobService) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *hostileBlobService) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	_, err := w.Write(s.content)
	return err
}

func (s *hostileBlobService) Delete(ctx context.Context, dgst digest.Digest) error {
	return fmt.Errorf("not implemented")
}

type nopSeekCloser struct {
	*bytes.Reader
}

func (nopSeekCloser) Close() error { return nil }

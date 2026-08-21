package memcached

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/distribution/v3/registry/storage/cache/metrics"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// memcachedBlobDescriptorService provides an implementation of
// BlobDescriptorCacheProvider based on memcached. Blob descriptors are stored
// as JSON encoded values, keyed by digest. There is also a per-repository
// entry for each blob descriptor, allowing override of the media type on a
// per-repository basis.
//
// Note that there is no implied relationship between these two caches. The
// blob may exist in one, both or none and the code must be written this way.
type memcachedBlobDescriptorService struct {
	client *memcache.Client
}

var _ distribution.BlobDescriptorService = &memcachedBlobDescriptorService{}

// NewMemcachedBlobDescriptorCacheProvider returns a new memcached-based
// BlobDescriptorCacheProvider using the provided memcached client.
func NewMemcachedBlobDescriptorCacheProvider(client *memcache.Client) cache.BlobDescriptorCacheProvider {
	return metrics.NewPrometheusCacheProvider(
		&memcachedBlobDescriptorService{
			client: client,
		},
		"cache_memcached",
		"Number of seconds taken by memcached",
	)
}

// RepositoryScoped returns the scoped cache.
func (mbds *memcachedBlobDescriptorService) RepositoryScoped(repo string) (distribution.BlobDescriptorService, error) {
	if _, err := reference.ParseNormalizedNamed(repo); err != nil {
		if err == reference.ErrNameTooLong {
			return nil, distribution.ErrRepositoryNameInvalid{
				Name:   repo,
				Reason: reference.ErrNameTooLong,
			}
		}

		return nil, err
	}

	return &repositoryScopedMemcachedBlobDescriptorService{
		repo:     repo,
		upstream: mbds,
	}, nil
}

// Stat retrieves the descriptor data from the memcached entry.
func (mbds *memcachedBlobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	if err := dgst.Validate(); err != nil {
		return v1.Descriptor{}, err
	}

	return mbds.stat(ctx, dgst)
}

func (mbds *memcachedBlobDescriptorService) stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	item, err := mbds.client.Get(mbds.blobDescriptorKey(dgst))

	if err != nil {
		if err == memcache.ErrCacheMiss {
			return v1.Descriptor{}, distribution.ErrBlobUnknown
		}

		return v1.Descriptor{}, err
	}

	var desc v1.Descriptor

	if err := json.Unmarshal(item.Value, &desc); err != nil {
		return v1.Descriptor{}, err
	}

	return desc, nil
}

// Clear removes the descriptor from the memcached entry.
func (mbds *memcachedBlobDescriptorService) Clear(ctx context.Context, dgst digest.Digest) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	err := mbds.client.Delete(mbds.blobDescriptorKey(dgst))

	if err != nil {
		if err == memcache.ErrCacheMiss {
			return distribution.ErrBlobUnknown
		}
		return err
	}

	return nil
}

// SetDescriptor sets the descriptor data for the given digest as a JSON encoded value.
func (mbds *memcachedBlobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	if err := cache.ValidateDescriptor(desc); err != nil {
		return err
	}

	return mbds.setDescriptor(ctx, dgst, desc)
}

func (mbds *memcachedBlobDescriptorService) setDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	data, err := json.Marshal(desc)

	if err != nil {
		return err
	}

	// Add stores the entry only if the key is absent, so the media type of the first write is preserved.
	// digest and size are content-derived and therefore identical for any later write of the same digest.
	if err := mbds.client.Add(&memcache.Item{Key: mbds.blobDescriptorKey(dgst), Value: data}); err != nil && err != memcache.ErrNotStored {
		return err
	}

	return nil
}

// keyFor returns a valid memcached key. Memcached keys are limited to 250
// bytes and must not contain spaces or control characters, so keys derived
// from long repository names or containing control characters are replaced
// with a fixed length hash.
func keyFor(key string) string {
	if len(key) <= 250 && !hasControlChars(key) {
		return key
	}

	sum := sha256.Sum256([]byte(key))
	return "hashed::" + hex.EncodeToString(sum[:])
}

func hasControlChars(key string) bool {
	for i := 0; i < len(key); i++ {
		b := key[i]
		if b <= 0x20 || b == 0x7f {
			return true
		}
	}
	return false
}

func (mbds *memcachedBlobDescriptorService) blobDescriptorKey(dgst digest.Digest) string {
	return keyFor("blobs::" + dgst.String())
}

func (rsmbds *repositoryScopedMemcachedBlobDescriptorService) blobDescriptorKey(dgst digest.Digest) string {
	return keyFor("repository::" + rsmbds.repo + "::blobs::" + dgst.String())
}

type repositoryScopedMemcachedBlobDescriptorService struct {
	repo     string
	upstream *memcachedBlobDescriptorService
}

var _ distribution.BlobDescriptorService = &repositoryScopedMemcachedBlobDescriptorService{}

// Stat returns the descriptor from the repository scoped memcached entry. If
// the digest is not a member of the repository, ErrBlobUnknown is returned.
func (rsmbds *repositoryScopedMemcachedBlobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	if err := dgst.Validate(); err != nil {
		return v1.Descriptor{}, err
	}

	item, err := rsmbds.upstream.client.Get(rsmbds.blobDescriptorKey(dgst))

	if err != nil {
		if err == memcache.ErrCacheMiss {
			return v1.Descriptor{}, distribution.ErrBlobUnknown
		}
		return v1.Descriptor{}, err
	}

	var desc v1.Descriptor

	if err := json.Unmarshal(item.Value, &desc); err != nil {
		return v1.Descriptor{}, err
	}

	return desc, nil
}

// Clear removes the descriptor from the repository and forwards to the
// upstream descriptor store.
func (rsmbds *repositoryScopedMemcachedBlobDescriptorService) Clear(ctx context.Context, dgst digest.Digest) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	// Check membership to the repository first.
	if _, err := rsmbds.upstream.client.Get(rsmbds.blobDescriptorKey(dgst)); err != nil {
		if err == memcache.ErrCacheMiss {
			return distribution.ErrBlobUnknown
		}
		return err
	}

	if err := rsmbds.upstream.client.Delete(rsmbds.blobDescriptorKey(dgst)); err != nil {
		return err
	}

	if err := rsmbds.upstream.client.Delete(rsmbds.upstream.blobDescriptorKey(dgst)); err != nil && err != memcache.ErrCacheMiss {
		return err
	}
	return nil
}

func (rsmbds *repositoryScopedMemcachedBlobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	if err := cache.ValidateDescriptor(desc); err != nil {
		return err
	}

	if dgst != desc.Digest {
		if dgst.Algorithm() == desc.Digest.Algorithm() {
			return fmt.Errorf("memcached cache: digest for descriptors differ but algorithm does not: %q != %q", dgst, desc.Digest)
		}
	}

	return rsmbds.setDescriptor(ctx, dgst, desc)
}

func (rsmbds *repositoryScopedMemcachedBlobDescriptorService) setDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	// The repository scoped entry carries the repository's media type.
	data, err := json.Marshal(desc)
	if err != nil {
		return err
	}
	if err := rsmbds.upstream.client.Set(&memcache.Item{Key: rsmbds.blobDescriptorKey(dgst), Value: data}); err != nil {
		return err
	}

	if err := rsmbds.upstream.setDescriptor(ctx, dgst, desc); err != nil {
		return err
	}

	// Also set the values for the primary descriptor, if they differ by
	// algorithm (ie sha256 vs sha512).
	if desc.Digest != "" && dgst != desc.Digest && dgst.Algorithm() != desc.Digest.Algorithm() {
		if err := rsmbds.setDescriptor(ctx, desc.Digest, desc); err != nil {
			return err
		}
	}

	return nil
}

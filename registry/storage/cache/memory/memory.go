package memory

import (
	"context"
	"math"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/reference"
	"github.com/hashicorp/golang-lru/arc/v2"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// DefaultSize is the default cache size to use if no size is explicitly
	// configured.
	DefaultSize = 10000

	// UnlimitedSize indicates the cache size should not be limited.
	UnlimitedSize = math.MaxInt
)

type descriptorCacheKey struct {
	digest digest.Digest
	repo   string
}

// descriptorCacheEntry is a cached descriptor with its expiration time. A
// zero expiresAt means the entry never expires.
type descriptorCacheEntry struct {
	desc      v1.Descriptor
	expiresAt time.Time
}

type inMemoryBlobDescriptorCacheProvider struct {
	lru *arc.ARCCache[descriptorCacheKey, descriptorCacheEntry]
	ttl time.Duration
	now func() time.Time
}

// NewInMemoryBlobDescriptorCacheProvider returns a new mapped-based cache for
// storing blob descriptor data.
func NewInMemoryBlobDescriptorCacheProvider(size int) cache.BlobDescriptorCacheProvider {
	return NewInMemoryBlobDescriptorCacheProviderWithTTL(size, 0)
}

// NewInMemoryBlobDescriptorCacheProviderWithTTL returns a new mapped-based
// cache for storing blob descriptor data, where entries expire after ttl.
// Expired entries are looked up again in the backend, which bounds how long
// the cache can keep reporting blobs that were removed from storage behind
// its back (e.g. by garbage collection). A ttl <= 0 disables expiration.
func NewInMemoryBlobDescriptorCacheProviderWithTTL(size int, ttl time.Duration) cache.BlobDescriptorCacheProvider {
	return newInMemoryBlobDescriptorCacheProvider(size, ttl, time.Now)
}

func newInMemoryBlobDescriptorCacheProvider(size int, ttl time.Duration, now func() time.Time) *inMemoryBlobDescriptorCacheProvider {
	if size <= 0 {
		size = math.MaxInt
	}
	lruCache, err := arc.NewARC[descriptorCacheKey, descriptorCacheEntry](size)
	if err != nil {
		// NewARC can only fail if size is <= 0, so this unreachable
		panic(err)
	}
	return &inMemoryBlobDescriptorCacheProvider{
		lru: lruCache,
		ttl: ttl,
		now: now,
	}
}

// get returns the entry for key, dropping it if it has expired.
func (imbdcp *inMemoryBlobDescriptorCacheProvider) get(key descriptorCacheKey) (descriptorCacheEntry, bool) {
	entry, ok := imbdcp.lru.Get(key)
	if !ok {
		return descriptorCacheEntry{}, false
	}
	if !entry.expiresAt.IsZero() && !imbdcp.now().Before(entry.expiresAt) {
		imbdcp.lru.Remove(key)
		return descriptorCacheEntry{}, false
	}
	return entry, true
}

// expiresAt returns the expiration time of an entry added now.
func (imbdcp *inMemoryBlobDescriptorCacheProvider) expiresAt() time.Time {
	if imbdcp.ttl <= 0 {
		return time.Time{}
	}
	return imbdcp.now().Add(imbdcp.ttl)
}

func (imbdcp *inMemoryBlobDescriptorCacheProvider) RepositoryScoped(repo string) (distribution.BlobDescriptorService, error) {
	if _, err := reference.ParseNormalizedNamed(repo); err != nil {
		if err == reference.ErrNameTooLong {
			return nil, distribution.ErrRepositoryNameInvalid{
				Name:   repo,
				Reason: reference.ErrNameTooLong,
			}
		}
		return nil, err
	}

	return &repositoryScopedInMemoryBlobDescriptorCache{
		repo:   repo,
		parent: imbdcp,
	}, nil
}

func (imbdcp *inMemoryBlobDescriptorCacheProvider) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	if err := dgst.Validate(); err != nil {
		return v1.Descriptor{}, err
	}

	key := descriptorCacheKey{
		digest: dgst,
	}
	entry, ok := imbdcp.get(key)
	if ok {
		return entry.desc, nil
	}
	return v1.Descriptor{}, distribution.ErrBlobUnknown
}

func (imbdcp *inMemoryBlobDescriptorCacheProvider) Clear(ctx context.Context, dgst digest.Digest) error {
	key := descriptorCacheKey{
		digest: dgst,
	}
	imbdcp.lru.Remove(key)
	return nil
}

func (imbdcp *inMemoryBlobDescriptorCacheProvider) SetDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	_, err := imbdcp.Stat(ctx, dgst)
	if err == distribution.ErrBlobUnknown {
		if dgst.Algorithm() != desc.Digest.Algorithm() && dgst != desc.Digest {
			// if the digests differ, set the other canonical mapping
			if err := imbdcp.SetDescriptor(ctx, desc.Digest, desc); err != nil {
				return err
			}
		}

		if err := dgst.Validate(); err != nil {
			return err
		}

		if err := cache.ValidateDescriptor(desc); err != nil {
			return err
		}

		key := descriptorCacheKey{
			digest: dgst,
		}
		imbdcp.lru.Add(key, descriptorCacheEntry{desc: desc, expiresAt: imbdcp.expiresAt()})
		return nil
	}
	// we already know it, do nothing
	return err
}

// repositoryScopedInMemoryBlobDescriptorCache provides the request scoped
// repository cache. Instances are not thread-safe but the delegated
// operations are.
type repositoryScopedInMemoryBlobDescriptorCache struct {
	repo   string
	parent *inMemoryBlobDescriptorCacheProvider // allows lazy allocation of repo's map
}

func (rsimbdcp *repositoryScopedInMemoryBlobDescriptorCache) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	if err := dgst.Validate(); err != nil {
		return v1.Descriptor{}, err
	}

	key := descriptorCacheKey{
		digest: dgst,
		repo:   rsimbdcp.repo,
	}
	entry, ok := rsimbdcp.parent.get(key)
	if ok {
		return entry.desc, nil
	}
	return v1.Descriptor{}, distribution.ErrBlobUnknown
}

func (rsimbdcp *repositoryScopedInMemoryBlobDescriptorCache) Clear(ctx context.Context, dgst digest.Digest) error {
	key := descriptorCacheKey{
		digest: dgst,
		repo:   rsimbdcp.repo,
	}
	rsimbdcp.parent.lru.Remove(key)
	return nil
}

func (rsimbdcp *repositoryScopedInMemoryBlobDescriptorCache) SetDescriptor(ctx context.Context, dgst digest.Digest, desc v1.Descriptor) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

	if err := cache.ValidateDescriptor(desc); err != nil {
		return err
	}

	// A repository miss is usually answered from the global entry (through
	// the linked blob statter) without checking storage, so the repository
	// entry must not outlive the global one.
	expiresAt := rsimbdcp.parent.expiresAt()
	if global, ok := rsimbdcp.parent.get(descriptorCacheKey{digest: dgst}); ok && !global.expiresAt.IsZero() && global.expiresAt.Before(expiresAt) {
		expiresAt = global.expiresAt
	}

	key := descriptorCacheKey{
		digest: dgst,
		repo:   rsimbdcp.repo,
	}
	rsimbdcp.parent.lru.Add(key, descriptorCacheEntry{desc: desc, expiresAt: expiresAt})
	return rsimbdcp.parent.SetDescriptor(ctx, dgst, desc)
}

package memory

import (
	"context"
	"math"

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

type inMemoryBlobDescriptorCacheProvider struct {
	lru *arc.ARCCache[descriptorCacheKey, v1.Descriptor]
}

// NewInMemoryBlobDescriptorCacheProvider returns a new mapped-based cache for
// storing blob descriptor data.
func NewInMemoryBlobDescriptorCacheProvider(size int) cache.BlobDescriptorCacheProvider {
	if size <= 0 {
		size = math.MaxInt
	}
	lruCache, err := arc.NewARC[descriptorCacheKey, v1.Descriptor](size)
	if err != nil {
		// NewARC can only fail if size is <= 0, so this unreachable
		panic(err)
	}
	return &inMemoryBlobDescriptorCacheProvider{
		lru: lruCache,
	}
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
	descriptor, ok := imbdcp.lru.Get(key)
	if ok {
		return descriptor, nil
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
		imbdcp.lru.Add(key, desc)
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
	descriptor, ok := rsimbdcp.parent.lru.Get(key)
	if ok {
		return descriptor, nil
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

	key := descriptorCacheKey{
		digest: dgst,
		repo:   rsimbdcp.repo,
	}
	rsimbdcp.parent.lru.Add(key, desc)
	return rsimbdcp.parent.SetDescriptor(ctx, dgst, desc)
}

package cache

import (
	"github.com/docker/distribution/digest"
	"golang.org/x/net/context"
)

// inmemoryLayerInfoCache is a map-based implementation of LayerInfoCache.
type inmemoryLayerInfoCache struct {
	membership map[string]map[digest.Digest]struct{}
	meta       map[digest.Digest]LayerMeta
}

// NewInMemoryLayerInfoCache provides an implementation of LayerInfoCache that
// stores results in memory.
func NewInMemoryLayerInfoCache() LayerInfoCache {
	return &base{&inmemoryLayerInfoCache{
		membership: make(map[string]map[digest.Digest]struct{}),
		meta:       make(map[digest.Digest]LayerMeta),
	}}
}

func (ilic *inmemoryLayerInfoCache) Contains(ctx context.Context, repo string, dgst digest.Digest) (bool, error) {
	members, ok := ilic.membership[repo]
	if !ok {
		return false, nil
	}

	_, ok = members[dgst]
	return ok, nil
}

// Add adds the layer to the redis repository blob set.
func (ilic *inmemoryLayerInfoCache) Add(ctx context.Context, repo string, dgst digest.Digest) error {
	members, ok := ilic.membership[repo]
	if !ok {
		members = make(map[digest.Digest]struct{})
		ilic.membership[repo] = members
	}

	members[dgst] = struct{}{}

	return nil
}

// Meta retrieves the layer meta data from the redis hash, returning
// ErrUnknownLayer if not found.
func (ilic *inmemoryLayerInfoCache) Meta(ctx context.Context, dgst digest.Digest) (LayerMeta, error) {
	meta, ok := ilic.meta[dgst]
	if !ok {
		return LayerMeta{}, ErrNotFound
	}

	return meta, nil
}

// SetMeta sets the meta data for the given digest using a redis hash. A hash
// is used here since we may store unrelated fields about a layer in the
// future.
func (ilic *inmemoryLayerInfoCache) SetMeta(ctx context.Context, dgst digest.Digest, meta LayerMeta) error {
	ilic.meta[dgst] = meta
	return nil
}

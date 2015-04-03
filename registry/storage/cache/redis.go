package cache

import (
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/garyburd/redigo/redis"
	"golang.org/x/net/context"
)

// redisLayerInfoCache provides an implementation of storage.LayerInfoCache
// based on redis. Layer info is stored in two parts. The first provide fast
// access to repository membership through a redis set for each repo. The
// second is a redis hash keyed by the digest of the layer, providing path and
// length information. Note that there is no implied relationship between
// these two caches. The layer may exist in one, both or none and the code
// must be written this way.
type redisLayerInfoCache struct {
	pool *redis.Pool

	// TODO(stevvooe): We use a pool because we don't have great control over
	// the cache lifecycle to manage connections. A new connection if fetched
	// for each operation. Once we have better lifecycle management of the
	// request objects, we can change this to a connection.
}

// NewRedisLayerInfoCache returns a new redis-based LayerInfoCache using the
// provided redis connection pool.
func NewRedisLayerInfoCache(pool *redis.Pool) LayerInfoCache {
	return &base{&redisLayerInfoCache{
		pool: pool,
	}}
}

// Contains does a membership check on the repository blob set in redis. This
// is used as an access check before looking up global path information. If
// false is returned, the caller should still check the backend to if it
// exists elsewhere.
func (rlic *redisLayerInfoCache) Contains(ctx context.Context, repo string, dgst digest.Digest) (bool, error) {
	conn := rlic.pool.Get()
	defer conn.Close()

	ctxu.GetLogger(ctx).Debugf("(*redisLayerInfoCache).Contains(%q, %q)", repo, dgst)
	return redis.Bool(conn.Do("SISMEMBER", rlic.repositoryBlobSetKey(repo), dgst))
}

// Add adds the layer to the redis repository blob set.
func (rlic *redisLayerInfoCache) Add(ctx context.Context, repo string, dgst digest.Digest) error {
	conn := rlic.pool.Get()
	defer conn.Close()

	ctxu.GetLogger(ctx).Debugf("(*redisLayerInfoCache).Add(%q, %q)", repo, dgst)
	_, err := conn.Do("SADD", rlic.repositoryBlobSetKey(repo), dgst)
	return err
}

// Meta retrieves the layer meta data from the redis hash, returning
// ErrUnknownLayer if not found.
func (rlic *redisLayerInfoCache) Meta(ctx context.Context, dgst digest.Digest) (LayerMeta, error) {
	conn := rlic.pool.Get()
	defer conn.Close()

	reply, err := redis.Values(conn.Do("HMGET", rlic.blobMetaHashKey(dgst), "path", "length"))
	if err != nil {
		return LayerMeta{}, err
	}

	if len(reply) < 2 || reply[0] == nil || reply[1] == nil {
		return LayerMeta{}, ErrNotFound
	}

	var meta LayerMeta
	if _, err := redis.Scan(reply, &meta.Path, &meta.Length); err != nil {
		return LayerMeta{}, err
	}

	return meta, nil
}

// SetMeta sets the meta data for the given digest using a redis hash. A hash
// is used here since we may store unrelated fields about a layer in the
// future.
func (rlic *redisLayerInfoCache) SetMeta(ctx context.Context, dgst digest.Digest, meta LayerMeta) error {
	conn := rlic.pool.Get()
	defer conn.Close()

	_, err := conn.Do("HMSET", rlic.blobMetaHashKey(dgst), "path", meta.Path, "length", meta.Length)
	return err
}

// repositoryBlobSetKey returns the key for the blob set in the cache.
func (rlic *redisLayerInfoCache) repositoryBlobSetKey(repo string) string {
	return "repository::" + repo + "::blobs"
}

// blobMetaHashKey returns the cache key for immutable blob meta data.
func (rlic *redisLayerInfoCache) blobMetaHashKey(dgst digest.Digest) string {
	return "blobs::" + dgst.String()
}

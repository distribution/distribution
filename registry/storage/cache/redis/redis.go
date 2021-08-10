package redis

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/distribution/v3/registry/storage/cache/metrics"
	"github.com/gomodule/redigo/redis"
	"github.com/opencontainers/go-digest"
)

const (
	CacheType = "redis"
)

// redisBlobStatService provides an implementation of
// BlobDescriptorCacheProvider based on redis. Blob descriptors are stored in
// two parts. The first provide fast access to repository membership through a
// redis set for each repo. The second is a redis hash keyed by the digest of
// the layer, providing path, length and mediatype information. There is also
// a per-repository redis hash of the blob descriptor, allowing override of
// data. This is currently used to override the mediatype on a per-repository
// basis.
//
// Note that there is no implied relationship between these two caches. The
// layer may exist in one, both or none and the code must be written this way.
type redisBlobDescriptorService struct {
	pool *redis.Pool

	// TODO(stevvooe): We use a pool because we don't have great control over
	// the cache lifecycle to manage connections. A new connection if fetched
	// for each operation. Once we have better lifecycle management of the
	// request objects, we can change this to a connection.
}

// NewRedisBlobDescriptorCacheProvider returns a new redis-based
// BlobDescriptorCacheProvider using the provided redis connection pool.
func NewRedisBlobDescriptorCacheProvider(pool *redis.Pool) cache.BlobDescriptorCacheProvider {
	return metrics.NewPrometheusCacheProvider(
		&redisBlobDescriptorService{
			pool: pool,
		},
		"cache_redis",
		"Number of seconds taken by redis",
	)
}

// RepositoryScoped returns the scoped cache.
func (rbds *redisBlobDescriptorService) RepositoryScoped(repo string) (distribution.BlobDescriptorService, error) {
	if _, err := reference.ParseNormalizedNamed(repo); err != nil {
		return nil, err
	}

	return &repositoryScopedRedisBlobDescriptorService{
		repo:     repo,
		upstream: rbds,
	}, nil
}

// Stat retrieves the descriptor data from the redis hash entry.
func (rbds *redisBlobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", CacheType, "Stat"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		return distribution.Descriptor{}, err
	}

	conn := rbds.pool.Get()
	defer conn.Close()

	return rbds.stat(spanCtx, conn, dgst)
}

func (rbds *redisBlobDescriptorService) Clear(ctx context.Context, dgst digest.Digest) error {
	span, _ := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", CacheType, "Clear"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		return err
	}

	conn := rbds.pool.Get()
	defer conn.Close()

	span.SetAttributes(attribute.String("Command", fmt.Sprintf("%s %s %s %s %s",
		"HDEL", rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype")))

	// Not atomic in redis <= 2.3
	reply, err := conn.Do("HDEL", rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype")
	if err != nil {
		return err
	}

	if reply == 0 {
		return distribution.ErrBlobUnknown
	}

	return nil
}

// stat provides an internal stat call that takes a connection parameter. This
// allows some internal management of the connection scope.
func (rbds *redisBlobDescriptorService) stat(ctx context.Context, conn redis.Conn, dgst digest.Digest) (distribution.Descriptor, error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("Command1", fmt.Sprintf("%s %s %s %s %s",
		"HMGET", rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype")))

	reply, err := redis.Values(conn.Do("HMGET", rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype"))
	if err != nil {
		return distribution.Descriptor{}, err
	}

	// NOTE(stevvooe): The "size" field used to be "length". We treat a
	// missing "size" field here as an unknown blob, which causes a cache
	// miss, effectively migrating the field.
	if len(reply) < 3 || reply[0] == nil || reply[1] == nil { // don't care if mediatype is nil
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	span.SetAttributes(attribute.String("Command2", fmt.Sprintf("%s %+v",
		"Scan", reply)))

	var desc distribution.Descriptor
	if _, err := redis.Scan(reply, &desc.Digest, &desc.Size, &desc.MediaType); err != nil {
		return distribution.Descriptor{}, err
	}

	return desc, nil
}

// SetDescriptor sets the descriptor data for the given digest using a redis
// hash. A hash is used here since we may store unrelated fields about a layer
// in the future.
func (rbds *redisBlobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", CacheType, "SetDescriptor"),
		trace.WithAttributes(attribute.String("dgst", dgst.String()),
			attribute.String("MediaType", desc.MediaType)))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		return err
	}

	if err := cache.ValidateDescriptor(desc); err != nil {
		return err
	}

	conn := rbds.pool.Get()
	defer conn.Close()

	return rbds.setDescriptor(spanCtx, conn, dgst, desc)
}

func (rbds *redisBlobDescriptorService) setDescriptor(ctx context.Context, conn redis.Conn, dgst digest.Digest, desc distribution.Descriptor) error {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("Command1", fmt.Sprintf("%s %s %s %s %s %d",
		"HMSET", rbds.blobDescriptorHashKey(dgst), "digest", desc.Digest, "size", desc.Size)))

	if _, err := conn.Do("HMSET", rbds.blobDescriptorHashKey(dgst),
		"digest", desc.Digest,
		"size", desc.Size); err != nil {
		return err
	}

	span.SetAttributes(attribute.String("Command2", fmt.Sprintf("%s %s %s %s",
		"HSETNX", rbds.blobDescriptorHashKey(dgst), "mediatype", desc.MediaType)))

	// Only set mediatype if not already set.
	if _, err := conn.Do("HSETNX", rbds.blobDescriptorHashKey(dgst),
		"mediatype", desc.MediaType); err != nil {
		return err
	}

	return nil
}

func (rbds *redisBlobDescriptorService) blobDescriptorHashKey(dgst digest.Digest) string {
	return "blobs::" + dgst.String()
}

type repositoryScopedRedisBlobDescriptorService struct {
	repo     string
	upstream *redisBlobDescriptorService
}

var _ distribution.BlobDescriptorService = &repositoryScopedRedisBlobDescriptorService{}

// Stat ensures that the digest is a member of the specified repository and
// forwards the descriptor request to the global blob store. If the media type
// differs for the repository, we override it.
func (rsrbds *repositoryScopedRedisBlobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	span, _ := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", CacheType, "Stat"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		return distribution.Descriptor{}, err
	}

	conn := rsrbds.upstream.pool.Get()
	defer conn.Close()

	// Check membership to repository first
	member, err := redis.Bool(conn.Do("SISMEMBER", rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst))
	if err != nil {
		return distribution.Descriptor{}, err
	}

	if !member {
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	upstream, err := rsrbds.upstream.stat(ctx, conn, dgst)
	if err != nil {
		return distribution.Descriptor{}, err
	}

	span.SetAttributes(attribute.String("Command3", fmt.Sprintf("%s %s %s", "HGET", rsrbds.blobDescriptorHashKey(dgst), "mediatype")))

	// We allow a per repository mediatype, let's look it up here.
	mediatype, err := redis.String(conn.Do("HGET", rsrbds.blobDescriptorHashKey(dgst), "mediatype"))
	if err != nil {
		if err == redis.ErrNil {
			return distribution.Descriptor{}, distribution.ErrBlobUnknown
		}

		return distribution.Descriptor{}, err
	}

	if mediatype != "" {
		upstream.MediaType = mediatype
	}

	return upstream, nil
}

// Clear removes the descriptor from the cache and forwards to the upstream descriptor store
func (rsrbds *repositoryScopedRedisBlobDescriptorService) Clear(ctx context.Context, dgst digest.Digest) error {
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", CacheType, "Clear"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		return err
	}

	conn := rsrbds.upstream.pool.Get()
	defer conn.Close()

	span.SetAttributes(attribute.String("Command", fmt.Sprintf("%s %s %s", "SISMEMBER", rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst)))

	// Check membership to repository first
	member, err := redis.Bool(conn.Do("SISMEMBER", rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst))
	if err != nil {
		return err
	}

	if !member {
		return distribution.ErrBlobUnknown
	}

	return rsrbds.upstream.Clear(spanCtx, dgst)
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", CacheType, "SetDescriptor"),
		trace.WithAttributes(attribute.String("dgst", dgst.String()),
			attribute.String("MediaType", desc.MediaType)))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		return err
	}

	if err := cache.ValidateDescriptor(desc); err != nil {
		return err
	}

	if dgst != desc.Digest {
		if dgst.Algorithm() == desc.Digest.Algorithm() {
			return fmt.Errorf("redis cache: digest for descriptors differ but algorithm does not: %q != %q", dgst, desc.Digest)
		}
	}

	conn := rsrbds.upstream.pool.Get()
	defer conn.Close()

	return rsrbds.setDescriptor(spanCtx, conn, dgst, desc)
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) setDescriptor(ctx context.Context, conn redis.Conn, dgst digest.Digest, desc distribution.Descriptor) error {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("Command1", fmt.Sprintf("%s %s %s", "SADD", rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst)))

	if _, err := conn.Do("SADD", rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst); err != nil {
		return err
	}

	if err := rsrbds.upstream.setDescriptor(ctx, conn, dgst, desc); err != nil {
		return err
	}

	span.SetAttributes(attribute.String("Command2", fmt.Sprintf("%s %s %s %s", "HSET", rsrbds.blobDescriptorHashKey(dgst), "mediatype", desc.MediaType)))

	// Override repository mediatype.
	if _, err := conn.Do("HSET", rsrbds.blobDescriptorHashKey(dgst), "mediatype", desc.MediaType); err != nil {
		return err
	}

	// Also set the values for the primary descriptor, if they differ by
	// algorithm (ie sha256 vs sha512).
	if desc.Digest != "" && dgst != desc.Digest && dgst.Algorithm() != desc.Digest.Algorithm() {
		if err := rsrbds.setDescriptor(ctx, conn, desc.Digest, desc); err != nil {
			return err
		}
	}

	return nil
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) blobDescriptorHashKey(dgst digest.Digest) string {
	return "repository::" + rsrbds.repo + "::blobs::" + dgst.String()
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) repositoryBlobSetKey(repo string) string {
	return "repository::" + rsrbds.repo + "::blobs"
}

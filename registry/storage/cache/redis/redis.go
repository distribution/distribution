package redis

import (
	"context"
	"fmt"
	"strconv"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/distribution/v3/registry/storage/cache/metrics"
	"github.com/distribution/distribution/v3/tracing"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	TraceSpanCacheType = "RedisCache"
)

// redisBlobDescriptorService provides an implementation of
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
	pool *redis.Client

	// TODO(stevvooe): We use a pool because we don't have great control over
	// the cache lifecycle to manage connections. A new connection if fetched
	// for each operation. Once we have better lifecycle management of the
	// request objects, we can change this to a connection.
}

var _ distribution.BlobDescriptorService = &redisBlobDescriptorService{}

// NewRedisBlobDescriptorCacheProvider returns a new redis-based
// BlobDescriptorCacheProvider using the provided redis connection pool.
func NewRedisBlobDescriptorCacheProvider(pool *redis.Client) cache.BlobDescriptorCacheProvider {
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
		if err == reference.ErrNameTooLong {
			return nil, distribution.ErrRepositoryNameInvalid{
				Name:   repo,
				Reason: reference.ErrNameTooLong,
			}
		}
		return nil, err
	}

	return &repositoryScopedRedisBlobDescriptorService{
		repo:     repo,
		upstream: rbds,
	}, nil
}

// Stat retrieves the descriptor data from the redis hash entry.
func (rbds *redisBlobDescriptorService) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", TraceSpanCacheType, "Stat"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		span.RecordError(err)
		return distribution.Descriptor{}, err
	}

	return rbds.stat(spanCtx, dgst)
}

func (rbds *redisBlobDescriptorService) Clear(ctx context.Context, dgst digest.Digest) error {
	span, _ := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", TraceSpanCacheType, "Clear"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		span.RecordError(err)
		return err
	}

	span.SetAttributes(attribute.String("RedisCommand", fmt.Sprintf("%s %s %s %s %s",
		"HDEL", rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype")))

	// Not atomic in redis <= 2.3
	cmd := rbds.pool.HDel(ctx, rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype")
	res, err := cmd.Result()
	if err != nil {
		return err
	}
	if res == 0 {
		return distribution.ErrBlobUnknown
	}
	return nil
}

func (rbds *redisBlobDescriptorService) stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("StatCommand1", fmt.Sprintf("%s %s %s %s %s",
		"HMGET", rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype")))

	cmd := rbds.pool.HMGet(ctx, rbds.blobDescriptorHashKey(dgst), "digest", "size", "mediatype")
	reply, err := cmd.Result()
	if err != nil {
		return distribution.Descriptor{}, err
	}

	// NOTE(stevvooe): The "size" field used to be "length". We treat a
	// missing "size" field here as an unknown blob, which causes a cache
	// miss, effectively migrating the field.
	if len(reply) < 3 || reply[0] == nil || reply[1] == nil { // don't care if mediatype is nil
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	span.SetAttributes(attribute.String("StatCommand2", fmt.Sprintf("%s %+v",
		"Scan", reply)))

	var desc distribution.Descriptor
	digestString, ok := reply[0].(string)
	if !ok {
		return distribution.Descriptor{}, fmt.Errorf("digest is not a string")
	}
	desc.Digest = digest.Digest(digestString)
	sizeString, ok := reply[1].(string)
	if !ok {
		return distribution.Descriptor{}, fmt.Errorf("size is not a string")
	}
	size, err := strconv.ParseInt(sizeString, 10, 64)
	if err != nil {
		return distribution.Descriptor{}, err
	}
	desc.Size = size
	if reply[2] != nil {
		mediaType, ok := reply[2].(string)
		if ok {
			desc.MediaType = mediaType
		}
	}
	return desc, nil
}

// SetDescriptor sets the descriptor data for the given digest using a redis
// hash. A hash is used here since we may store unrelated fields about a layer
// in the future.
func (rbds *redisBlobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", TraceSpanCacheType, "SetDescriptor"),
		trace.WithAttributes(attribute.String("dgst", dgst.String()),
			attribute.String("MediaType", desc.MediaType)))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		span.RecordError(err)
		return err
	}

	if err := cache.ValidateDescriptor(desc); err != nil {
		return err
	}

	return rbds.setDescriptor(spanCtx, dgst, desc)
}

func (rbds *redisBlobDescriptorService) setDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("SetDescriptorCommand1", fmt.Sprintf("%s %s %s %s %s %d",
		"HMSET", rbds.blobDescriptorHashKey(dgst), "digest", desc.Digest, "size", desc.Size)))

	cmd := rbds.pool.HMSet(ctx, rbds.blobDescriptorHashKey(dgst), "digest", desc.Digest.String(), "size", desc.Size)
	if cmd.Err() != nil {
		return cmd.Err()
	}

	span.SetAttributes(attribute.String("SetDescriptorCommand1", fmt.Sprintf("%s %s %s %s",
		"HSETNX", rbds.blobDescriptorHashKey(dgst), "mediatype", desc.MediaType)))

	cmd = rbds.pool.HSetNX(ctx, rbds.blobDescriptorHashKey(dgst), "mediatype", desc.MediaType)
	if cmd.Err() != nil {
		return cmd.Err()
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
	span, _ := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", TraceSpanCacheType, "Stat"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		span.RecordError(err)
		return distribution.Descriptor{}, err
	}

	pool := rsrbds.upstream.pool

	span.SetAttributes(attribute.String("Command3", fmt.Sprintf("%s %s %s", "HGET", rsrbds.blobDescriptorHashKey(dgst), "mediatype")))

	// Check membership to repository first
	member, err := pool.SIsMember(ctx, rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst.String()).Result()
	if err != nil {
		return distribution.Descriptor{}, err
	}
	if !member {
		return distribution.Descriptor{}, distribution.ErrBlobUnknown
	}

	upstream, err := rsrbds.upstream.stat(ctx, dgst)
	if err != nil {
		return distribution.Descriptor{}, err
	}

	// We allow a per repository mediatype, let's look it up here.
	mediatype, err := pool.HGet(ctx, rsrbds.blobDescriptorHashKey(dgst), "mediatype").Result()
	if err != nil {
		if err == redis.Nil {
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
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", TraceSpanCacheType, "Clear"),
		trace.WithAttributes(attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	if err := dgst.Validate(); err != nil {
		span.RecordError(err)
		return err
	}

	span.SetAttributes(attribute.String("ClearCommand", fmt.Sprintf("%s %s %s", "SISMEMBER", rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst)))

	// Check membership to repository first
	member, err := rsrbds.upstream.pool.SIsMember(ctx, rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst.String()).Result()
	if err != nil {
		return err
	}
	if !member {
		return distribution.ErrBlobUnknown
	}

	return rsrbds.upstream.Clear(spanCtx, dgst)
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	span, spanCtx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", TraceSpanCacheType, "SetDescriptor"),
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

	return rsrbds.setDescriptor(spanCtx, dgst, desc)
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) setDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("Command1", fmt.Sprintf("%s %s %s", "SADD", rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst)))

	conn := rsrbds.upstream.pool
	_, err := conn.SAdd(ctx, rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst.String()).Result()
	if err != nil {
		return err
	}

	if err := rsrbds.upstream.setDescriptor(ctx, dgst, desc); err != nil {
		return err
	}

	span.SetAttributes(attribute.String("Command2", fmt.Sprintf("%s %s %s %s", "HSET", rsrbds.blobDescriptorHashKey(dgst), "mediatype", desc.MediaType)))

	// Override repository mediatype.
	_, err = conn.HSet(ctx, rsrbds.blobDescriptorHashKey(dgst), "mediatype", desc.MediaType).Result()
	if err != nil {
		return err
	}

	// Also set the values for the primary descriptor, if they differ by
	// algorithm (ie sha256 vs sha512).
	if desc.Digest != "" && dgst != desc.Digest && dgst.Algorithm() != desc.Digest.Algorithm() {
		if err := rsrbds.setDescriptor(ctx, desc.Digest, desc); err != nil {
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

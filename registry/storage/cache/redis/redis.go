package redis

import (
	"context"
	"fmt"
	"strconv"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/distribution/v3/registry/storage/cache/metrics"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	"github.com/redis/go-redis/v9"
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
	pool redis.UniversalClient

	// TODO(stevvooe): We use a pool because we don't have great control over
	// the cache lifecycle to manage connections. A new connection if fetched
	// for each operation. Once we have better lifecycle management of the
	// request objects, we can change this to a connection.
}

var _ distribution.BlobDescriptorService = &redisBlobDescriptorService{}

// NewRedisBlobDescriptorCacheProvider returns a new redis-based
// BlobDescriptorCacheProvider using the provided redis connection pool.
func NewRedisBlobDescriptorCacheProvider(pool redis.UniversalClient) cache.BlobDescriptorCacheProvider {
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
	if err := dgst.Validate(); err != nil {
		return distribution.Descriptor{}, err
	}

	return rbds.stat(ctx, dgst)
}

func (rbds *redisBlobDescriptorService) Clear(ctx context.Context, dgst digest.Digest) error {
	if err := dgst.Validate(); err != nil {
		return err
	}

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
	if err := dgst.Validate(); err != nil {
		return err
	}

	if err := cache.ValidateDescriptor(desc); err != nil {
		return err
	}

	return rbds.setDescriptor(ctx, dgst, desc)
}

func (rbds *redisBlobDescriptorService) setDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	cmd := rbds.pool.HMSet(ctx, rbds.blobDescriptorHashKey(dgst), "digest", desc.Digest.String(), "size", desc.Size)
	if cmd.Err() != nil {
		return cmd.Err()
	}

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
	if err := dgst.Validate(); err != nil {
		return distribution.Descriptor{}, err
	}

	pool := rsrbds.upstream.pool
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
	if err := dgst.Validate(); err != nil {
		return err
	}

	// Check membership to repository first
	member, err := rsrbds.upstream.pool.SIsMember(ctx, rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst.String()).Result()
	if err != nil {
		return err
	}
	if !member {
		return distribution.ErrBlobUnknown
	}

	return rsrbds.upstream.Clear(ctx, dgst)
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) SetDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
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

	return rsrbds.setDescriptor(ctx, dgst, desc)
}

func (rsrbds *repositoryScopedRedisBlobDescriptorService) setDescriptor(ctx context.Context, dgst digest.Digest, desc distribution.Descriptor) error {
	conn := rsrbds.upstream.pool
	_, err := conn.SAdd(ctx, rsrbds.repositoryBlobSetKey(rsrbds.repo), dgst.String()).Result()
	if err != nil {
		return err
	}

	if err := rsrbds.upstream.setDescriptor(ctx, dgst, desc); err != nil {
		return err
	}

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

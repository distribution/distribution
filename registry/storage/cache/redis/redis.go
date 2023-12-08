package redis

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"strconv"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	"github.com/distribution/distribution/v3/registry/storage/cache/metrics"
	"github.com/distribution/reference"
	"github.com/mitchellh/mapstructure"
	"github.com/opencontainers/go-digest"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

// init registers the redis cacheprovider.
func init() {
	cache.Register("redis", NewBlobDescriptorCacheProvider)
}

var (
	// ErrMissingConfig is returned when redis config is missing.
	ErrMissingConfig = errors.New("missing configuration")
	// ErrMissingAddr is returned when redis congig misses address.
	ErrMissingAddr = errors.New("missing address")
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

// NewBlobDescriptorCacheProvider returns a new redis-based
// BlobDescriptorCacheProvider using the provided redis connection pool.
func NewBlobDescriptorCacheProvider(ctx context.Context, options map[string]interface{}) (cache.BlobDescriptorCacheProvider, error) {
	params, ok := options["params"]
	if !ok {
		return nil, ErrMissingConfig
	}

	var c Redis

	// NOTE(milosgajdos): mapstructure does not decode time types such as duration
	// which we need in the timeout configuration values out of the box
	// but it provides a way to do this via DecodeHook functions.
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
		WeaklyTypedInput: true,
		Result:           &c,
	})
	if err != nil {
		return nil, err
	}
	if err := dec.Decode(params); err != nil {
		return nil, err
	}

	if c.Addr == "" {
		return nil, ErrMissingAddr
	}

	pool := createPool(c)

	// Enable metrics instrumentation.
	if err := redisotel.InstrumentMetrics(pool); err != nil {
		dcontext.GetLogger(ctx).Errorf("failed to instrument metrics on redis: %v", err)
	}

	// setup expvar
	registry := expvar.Get("registry")
	if registry == nil {
		registry = expvar.NewMap("registry")
	}

	registry.(*expvar.Map).Set("redis", expvar.Func(func() interface{} {
		stats := pool.PoolStats()
		return map[string]interface{}{
			"Config": c,
			"Active": stats.TotalConns - stats.IdleConns,
		}
	}))

	return metrics.NewPrometheusCacheProvider(
		&redisBlobDescriptorService{
			pool: pool,
		},
		"cache_redis",
		"Number of seconds taken by redis",
	), nil
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

// Redis configures the redis pool available to the registry.
type Redis struct {
	// Addr specifies the the redis instance available to the application.
	Addr string `yaml:"addr,omitempty"`

	// Usernames can be used as a finer-grained permission control since the introduction of the redis 6.0.
	Username string `yaml:"username,omitempty"`

	// Password string to use when making a connection.
	Password string `yaml:"password,omitempty"`

	// DB specifies the database to connect to on the redis instance.
	DB int `yaml:"db,omitempty"`

	// TLS configures settings for redis in-transit encryption
	TLS struct {
		Enabled bool `yaml:"enabled,omitempty"`
	} `yaml:"tls,omitempty"`

	DialTimeout  time.Duration `yaml:"dialtimeout,omitempty"`  // timeout for connect
	ReadTimeout  time.Duration `yaml:"readtimeout,omitempty"`  // timeout for reads of data
	WriteTimeout time.Duration `yaml:"writetimeout,omitempty"` // timeout for writes of data

	// Pool configures the behavior of the redis connection pool.
	Pool struct {
		// MaxIdle sets the maximum number of idle connections.
		MaxIdle int `yaml:"maxidle,omitempty"`

		// MaxActive sets the maximum number of connections that should be
		// opened before blocking a connection request.
		MaxActive int `yaml:"maxactive,omitempty"`

		// IdleTimeout sets the amount time to wait before closing
		// inactive connections.
		IdleTimeout time.Duration `yaml:"idletimeout,omitempty"`
	} `yaml:"pool,omitempty"`
}

func createPool(cfg Redis) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr: cfg.Addr,
		OnConnect: func(ctx context.Context, cn *redis.Conn) error {
			res := cn.Ping(ctx)
			return res.Err()
		},
		Username:        cfg.Username,
		Password:        cfg.Password,
		DB:              cfg.DB,
		MaxRetries:      3,
		DialTimeout:     cfg.DialTimeout,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		PoolFIFO:        false,
		MaxIdleConns:    cfg.Pool.MaxIdle,
		PoolSize:        cfg.Pool.MaxActive,
		ConnMaxIdleTime: cfg.Pool.IdleTimeout,
	})
}

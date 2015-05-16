package storage

import (
	"expvar"
	"sync/atomic"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"

	"github.com/docker/distribution"
)

type cachedBlobStatter struct {
	cache   distribution.BlobDescriptorService
	backend distribution.BlobStatter
}

func (cbds *cachedBlobStatter) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	atomic.AddUint64(&blobStatterCacheMetrics.Stat.Requests, 1)
	desc, err := cbds.cache.Stat(ctx, dgst)
	if err != nil {
		if err != distribution.ErrBlobUnknown {
			context.GetLogger(ctx).Errorf("error retrieving descriptor from cache: %v", err)
		}

		goto fallback
	}

	atomic.AddUint64(&blobStatterCacheMetrics.Stat.Hits, 1)
	return desc, nil
fallback:
	atomic.AddUint64(&blobStatterCacheMetrics.Stat.Misses, 1)
	desc, err = cbds.backend.Stat(ctx, dgst)
	if err != nil {
		return desc, err
	}

	if err := cbds.cache.SetDescriptor(ctx, dgst, desc); err != nil {
		context.GetLogger(ctx).Errorf("error adding descriptor %v to cache: %v", desc.Digest, err)
	}

	return desc, err
}

// blobStatterCacheMetrics keeps track of cache metrics for blob descriptor
// cache requests. Note this is kept globally and made available via expvar.
// For more detailed metrics, its recommend to instrument a particular cache
// implementation.
var blobStatterCacheMetrics struct {
	// Stat tracks calls to the caches.
	Stat struct {
		Requests uint64
		Hits     uint64
		Misses   uint64
	}
}

func init() {
	registry := expvar.Get("registry")
	if registry == nil {
		registry = expvar.NewMap("registry")
	}

	cache := registry.(*expvar.Map).Get("cache")
	if cache == nil {
		cache = &expvar.Map{}
		cache.(*expvar.Map).Init()
		registry.(*expvar.Map).Set("cache", cache)
	}

	storage := cache.(*expvar.Map).Get("storage")
	if storage == nil {
		storage = &expvar.Map{}
		storage.(*expvar.Map).Init()
		cache.(*expvar.Map).Set("storage", storage)
	}

	storage.(*expvar.Map).Set("blobdescriptor", expvar.Func(func() interface{} {
		// no need for synchronous access: the increments are atomic and
		// during reading, we don't care if the data is up to date. The
		// numbers will always *eventually* be reported correctly.
		return blobStatterCacheMetrics
	}))
}

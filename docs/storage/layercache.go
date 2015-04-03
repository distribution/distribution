package storage

import (
	"expvar"
	"sync/atomic"
	"time"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage/cache"
	"github.com/docker/distribution/registry/storage/driver"
	"golang.org/x/net/context"
)

// cachedLayerService implements the layer service with path-aware caching,
// using a LayerInfoCache interface.
type cachedLayerService struct {
	distribution.LayerService // upstream layer service
	repository                distribution.Repository
	ctx                       context.Context
	driver                    driver.StorageDriver
	*blobStore                // global blob store
	cache                     cache.LayerInfoCache
}

// Exists checks for existence of the digest in the cache, immediately
// returning if it exists for the repository. If not, the upstream is checked.
// When a positive result is found, it is written into the cache.
func (lc *cachedLayerService) Exists(dgst digest.Digest) (bool, error) {
	ctxu.GetLogger(lc.ctx).Debugf("(*cachedLayerService).Exists(%q)", dgst)
	now := time.Now()
	defer func() {
		// TODO(stevvooe): Replace this with a decent context-based metrics solution
		ctxu.GetLoggerWithField(lc.ctx, "blob.exists.duration", time.Since(now)).
			Infof("(*cachedLayerService).Exists(%q)", dgst)
	}()

	atomic.AddUint64(&layerInfoCacheMetrics.Exists.Requests, 1)
	available, err := lc.cache.Contains(lc.ctx, lc.repository.Name(), dgst)
	if err != nil {
		ctxu.GetLogger(lc.ctx).Errorf("error checking availability of %v@%v: %v", lc.repository.Name(), dgst, err)
		goto fallback
	}

	if available {
		atomic.AddUint64(&layerInfoCacheMetrics.Exists.Hits, 1)
		return true, nil
	}

fallback:
	atomic.AddUint64(&layerInfoCacheMetrics.Exists.Misses, 1)
	exists, err := lc.LayerService.Exists(dgst)
	if err != nil {
		return exists, err
	}

	if exists {
		// we can only cache this if the existence is positive.
		if err := lc.cache.Add(lc.ctx, lc.repository.Name(), dgst); err != nil {
			ctxu.GetLogger(lc.ctx).Errorf("error adding %v@%v to cache: %v", lc.repository.Name(), dgst, err)
		}
	}

	return exists, err
}

// Fetch checks for the availability of the layer in the repository via the
// cache. If present, the metadata is resolved and the layer is returned. If
// any operation fails, the layer is read directly from the upstream. The
// results are cached, if possible.
func (lc *cachedLayerService) Fetch(dgst digest.Digest) (distribution.Layer, error) {
	ctxu.GetLogger(lc.ctx).Debugf("(*layerInfoCache).Fetch(%q)", dgst)
	now := time.Now()
	defer func() {
		ctxu.GetLoggerWithField(lc.ctx, "blob.fetch.duration", time.Since(now)).
			Infof("(*layerInfoCache).Fetch(%q)", dgst)
	}()

	atomic.AddUint64(&layerInfoCacheMetrics.Fetch.Requests, 1)
	available, err := lc.cache.Contains(lc.ctx, lc.repository.Name(), dgst)
	if err != nil {
		ctxu.GetLogger(lc.ctx).Errorf("error checking availability of %v@%v: %v", lc.repository.Name(), dgst, err)
		goto fallback
	}

	if available {
		// fast path: get the layer info and return
		meta, err := lc.cache.Meta(lc.ctx, dgst)
		if err != nil {
			ctxu.GetLogger(lc.ctx).Errorf("error fetching %v@%v from cache: %v", lc.repository.Name(), dgst, err)
			goto fallback
		}

		atomic.AddUint64(&layerInfoCacheMetrics.Fetch.Hits, 1)
		return newLayerReader(lc.driver, dgst, meta.Path, meta.Length)
	}

	// NOTE(stevvooe): Unfortunately, the cache here only makes checks for
	// existing layers faster. We'd have to provide more careful
	// synchronization with the backend to make the missing case as fast.

fallback:
	atomic.AddUint64(&layerInfoCacheMetrics.Fetch.Misses, 1)
	layer, err := lc.LayerService.Fetch(dgst)
	if err != nil {
		return nil, err
	}

	// add the layer to the repository
	if err := lc.cache.Add(lc.ctx, lc.repository.Name(), dgst); err != nil {
		ctxu.GetLogger(lc.ctx).
			Errorf("error caching repository relationship for %v@%v: %v", lc.repository.Name(), dgst, err)
	}

	// lookup layer path and add it to the cache, if it succeds. Note that we
	// still return the layer even if we have trouble caching it.
	if path, err := lc.resolveLayerPath(layer); err != nil {
		ctxu.GetLogger(lc.ctx).
			Errorf("error resolving path while caching %v@%v: %v", lc.repository.Name(), dgst, err)
	} else {
		// add the layer to the cache once we've resolved the path.
		if err := lc.cache.SetMeta(lc.ctx, dgst, cache.LayerMeta{Path: path, Length: layer.Length()}); err != nil {
			ctxu.GetLogger(lc.ctx).Errorf("error adding meta for %v@%v to cache: %v", lc.repository.Name(), dgst, err)
		}
	}

	return layer, err
}

// extractLayerInfo pulls the layerInfo from the layer, attempting to get the
// path information from either the concrete object or by resolving the
// primary blob store path.
func (lc *cachedLayerService) resolveLayerPath(layer distribution.Layer) (path string, err error) {
	// try and resolve the type and driver, so we don't have to traverse links
	switch v := layer.(type) {
	case *layerReader:
		// only set path if we have same driver instance.
		if v.driver == lc.driver {
			return v.path, nil
		}
	}

	ctxu.GetLogger(lc.ctx).Warnf("resolving layer path during cache lookup (%v@%v)", lc.repository.Name(), layer.Digest())
	// we have to do an expensive stat to resolve the layer location but no
	// need to check the link, since we already have layer instance for this
	// repository.
	bp, err := lc.blobStore.path(layer.Digest())
	if err != nil {
		return "", err
	}

	return bp, nil
}

// layerInfoCacheMetrics keeps track of cache metrics for layer info cache
// requests. Note this is kept globally and made available via expvar. For
// more detailed metrics, its recommend to instrument a particular cache
// implementation.
var layerInfoCacheMetrics struct {
	// Exists tracks calls to the Exists caches.
	Exists struct {
		Requests uint64
		Hits     uint64
		Misses   uint64
	}

	// Fetch tracks calls to the fetch caches.
	Fetch struct {
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

	storage.(*expvar.Map).Set("layerinfo", expvar.Func(func() interface{} {
		// no need for synchronous access: the increments are atomic and
		// during reading, we don't care if the data is up to date. The
		// numbers will always *eventually* be reported correctly.
		return layerInfoCacheMetrics
	}))
}

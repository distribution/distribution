package storage

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/garyburd/redigo/redis"
	"golang.org/x/net/context"
)

// layerInfoCache is a driver-aware cache of layer metadata. Basically, it
// provides a fast cache for checks against repository metadata, avoiding
// round trips to check for linked blobs. Note that this is different from a
// pure layer cache, which would also provide access to backing data. Such a
// cache should be implemented as a middleware, rather than integrated with
// the storage backend.
//
// Layer info is stored in two parts. The first provide fast access to
// repository membership through a redis set for each repo. The second is a
// redis hash keyed by the digest of the layer, providing path and length
// information. Note that there is no implied relationship between these two
// caches. The layer may exist in one, both or none and the code must be
// written this way.
type layerInfoCache struct {
	distribution.Repository
	distribution.LayerService
	ctx        context.Context
	driver     driver.StorageDriver
	*blobStore // global blob store
	redis      *redis.Pool
}

func (lc *layerInfoCache) Exists(dgst digest.Digest) (bool, error) {
	logrus.Infof("(*layerInfoCache).Exists(%q)", dgst)
	var (
		available bool
		err       error
		conn      redis.Conn
	)

	if lc.redis == nil {
		goto fallback
	}

	conn = lc.redis.Get()
	defer conn.Close()

	available, err = lc.available(conn, dgst)
	if err != nil {
		logrus.Errorf("error checking availability of %v@%v: %v", lc.Repository.Name(), dgst, err)
		goto fallback
	}

	if available {
		return true, nil
	}

fallback:
	exists, err := lc.LayerService.Exists(dgst)
	if err != nil {
		return exists, err
	}

	if conn != nil && exists {
		// we can only cache this if the existence is positive.
		if err := lc.addDigest(conn, dgst); err != nil {
			logrus.Errorf("error adding %v@%v to cache: %v", lc.Repository.Name(), dgst, err)
		}
	}

	return exists, err
}

func (lc *layerInfoCache) Fetch(dgst digest.Digest) (distribution.Layer, error) {
	logrus.Infof("(*layerInfoCache).Fetch(%q)", dgst)
	now := time.Now()
	defer func() {
		logrus.WithField("blob.fetch.duration", time.Since(now)).Infof("(*layerInfoCache).Fetch(%q)", dgst)
	}()

	var (
		available bool
		err       error
		conn      redis.Conn
	)

	if lc.redis == nil {
		goto fallback
	}

	conn = lc.redis.Get()
	defer conn.Close()

	available, err = lc.available(conn, dgst)
	if err != nil {
		logrus.Errorf("error checking availability of %v@%v: %v", lc.Repository.Name(), dgst, err)
		goto fallback
	}

	logrus.Infof("(*layerInfoCache).Fetch(%q) -> avialable: %v", dgst, available)
	if available {
		// fast path: get the layer info and return
		layer, err := lc.fetch(conn, dgst)
		if err != nil {
			logrus.Errorf("error fetching %v@%v from cache: %v", lc.Repository.Name(), dgst, err)
			goto fallback
		}

		return layer, nil
	}

	// NOTE(stevvooe): Unfortunately, the cache here only makes checks for
	// existing layers faster. We'd have to provide more careful
	// synchronization with the backend to make the missing case as fast.

fallback:
	layer, err := lc.LayerService.Fetch(dgst)
	if err != nil {
		return nil, err
	}

	if conn != nil {
		if err := lc.add(conn, layer); err != nil {
			logrus.Errorf("error adding %v@%v to cache: %v", lc.Repository.Name(), dgst, err)
		}
	}

	return layer, err
}

// available tells the caller that the layer exists in the repository used by
// this layerInfoCache. This is used as an access check before looking up
// global path information. If false is returned, the caller should still
// check the backend to if it exists elsewhere.
func (lc *layerInfoCache) available(conn redis.Conn, dgst digest.Digest) (bool, error) {
	logrus.Infof("(*layerInfoCache).available(%q)", dgst)
	return redis.Bool(conn.Do("SISMEMBER", lc.repositoryBlobsKey(), dgst))
}

// add the layer to the cache.
func (lc *layerInfoCache) add(conn redis.Conn, layer distribution.Layer) error {
	// first, add the raw blob data
	if err := lc.addLayer(conn, layer); err != nil {
		return err
	}

	// second, add the layer to the repo
	return lc.addDigest(conn, layer.Digest())
}

// addLayer stores the layer info in the cache.
func (lc *layerInfoCache) addLayer(conn redis.Conn, layer distribution.Layer) error {
	path, err := lc.resolveLayerPath(layer)
	if err != nil {
		return err
	}
	// Store the data in a hash. Using a hash here since we might store
	// other layer related fields in the future.
	_, err = conn.Do("HMSET", blobKey(layer.Digest()), "path", path, "length", layer.Length())
	return err
}

// addDigest associates the digest with the repository set.
func (lc *layerInfoCache) addDigest(conn redis.Conn, dgst digest.Digest) error {
	// now, associate it with the repository.
	_, err := conn.Do("SADD", lc.repositoryBlobsKey(), dgst)
	return err
}

// fetch grabs the layer from the cache, if available.
func (lc *layerInfoCache) fetch(conn redis.Conn, dgst digest.Digest) (distribution.Layer, error) {
	logrus.Infof("(*layerInfoCache).fetch(%q)", dgst)
	reply, err := redis.Values(conn.Do("HMGET", blobKey(dgst), "path", "length"))
	if err != nil {
		return nil, err
	}

	var (
		path   string
		length int64
	)

	if _, err := redis.Scan(reply, &path, &length); err != nil {
		return nil, err
	}

	if path == "" {
		return nil, distribution.ErrUnknownLayer{
			FSLayer: manifest.FSLayer{BlobSum: dgst},
		}
	}

	return newLayerReader(lc.driver, dgst, path, length)
}

// extractLayerInfo pulls the layerInfo from the layer, attempting to get the
// path information from either the concrete object or by resolving the
// primary blob store path.
func (lc *layerInfoCache) resolveLayerPath(layer distribution.Layer) (path string, err error) {
	// try and resolve the type and driver, so we don't have to traverse links
	switch v := layer.(type) {
	case *layerReader:
		// only set path if we have same driver instance.
		if v.driver == lc.driver {
			return v.path, nil
		}
	}

	logrus.Warnf("resolving layer path during cache lookup (%v@%v)", lc.Repository.Name(), layer.Digest())
	// we have to do an expensive stat to resolve the layer location but no
	// need to check the link, since we already have layer instance for this
	// repository.
	bp, err := lc.blobStore.path(layer.Digest())
	if err != nil {
		return "", err
	}

	return bp, nil
}

// repositoryBlobsKey returns the key for the blob set in the cache.
func (lc *layerInfoCache) repositoryBlobsKey() string {
	return "repository::" + lc.Repository.Name() + "::blobs"
}

func blobKey(dgst digest.Digest) string {
	return "blobs::" + dgst.String()
}

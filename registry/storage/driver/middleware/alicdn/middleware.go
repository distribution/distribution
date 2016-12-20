package middleware

import (
	"fmt"
	"hash/fnv"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	storagemiddleware "github.com/docker/distribution/registry/storage/driver/middleware"
	"net/url"
	"time"
)

type aliCDNStorageMiddleware struct {
	storagedriver.StorageDriver
	scheme string
	host   string
}

func newAliCDNStorageMiddleware(storageDriver storagedriver.StorageDriver, options map[string]interface{}) (storagedriver.StorageDriver, error) {
	o, ok := options["baseurl"]
	if !ok {
		return nil, fmt.Errorf("no baseurl provided")
	}
	b, ok := o.(string)
	if !ok {
		return nil, fmt.Errorf("baseurl must be a string")
	}
	u, err := url.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("unable to parse alicdn baseurl: %s", b)
	}
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	if u.Host == "" {
		return nil, fmt.Errorf("no host specified for alicdn")
	}
	return &aliCDNStorageMiddleware{
		StorageDriver: storageDriver,
		scheme:        u.Scheme,
		host:          u.Host,
	}, nil
}

func h(data string) int {
	h := fnv.New32a()
	h.Write([]byte(data))
	return int(h.Sum32())
}

func expireOf(path string) time.Time {
	// map hash(path) to fix point by second on wall clock
	secondsInWallClock := h(path) % 3600
	now := time.Now().Truncate(time.Second)
	secondsOfHour := now.Minute()*60 + now.Second()
	insurance := 5 * 60

	var expire time.Time
	expire = now.Add(time.Duration(secondsInWallClock-secondsOfHour) * time.Second)
	if secondsInWallClock-secondsOfHour < insurance {
		expire = expire.Add(time.Hour)
	}
	return expire
}

func (cdn *aliCDNStorageMiddleware) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	if cdn.StorageDriver.Name() != "oss" {
		context.GetLogger(ctx).Warn("Alicdn middleware does not support this backend storage driver")
		return cdn.StorageDriver.URLFor(ctx, path, options)
	}

	options["expiry"] = expireOf(path)

	ossURL, err := cdn.StorageDriver.URLFor(ctx, path, options)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(ossURL)
	if err != nil {
		return "", err
	}
	u.Scheme = cdn.scheme
	u.Host = cdn.host
	return u.String(), nil
}
func init() {
	storagemiddleware.Register("alicdn", storagemiddleware.InitFunc(newAliCDNStorageMiddleware))
}

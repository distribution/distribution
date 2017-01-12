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
		return nil, fmt.Errorf("no scheme specified for alicdn baseurl")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("no host specified for alicdn baseurl")
	}
	return &aliCDNStorageMiddleware{
		StorageDriver: storageDriver,
		scheme:        u.Scheme,
		host:          u.Host,
	}, nil
}

func (cdn *aliCDNStorageMiddleware) h(data string) int {
	h := fnv.New32a()
	h.Write([]byte(data))
	return int(h.Sum32())
}

func (cdn *aliCDNStorageMiddleware) expireOf(now time.Time, path string) time.Time {

	// Every file has it's own expire point based on hash of the path,
	// such as 19 minutes past the hour, 25 minutes and 12 seconds past the hour, to avoid heavy traffic
	// back to oss if all file on cdn expire at the same time.

	// If the calculated expire time is too close(less than 5 minutes), return expire point at next hour.

	// To balance the security and cost, one hour as max expire time is considerable.

	secondsInWallClock := cdn.h(path) % 3600
	now = now.Truncate(time.Second)
	secondsOfHour := now.Minute()*60 + now.Second()
	insurance := 5 * 60

	var expire time.Time
	expire = now.Add(time.Duration(secondsInWallClock-secondsOfHour) * time.Second)
	if secondsInWallClock-secondsOfHour < insurance {
		expire = expire.Add(time.Hour)
	}
	return expire
}

func (cdn *aliCDNStorageMiddleware) replaceDomain(ossURL string) (string, error) {
	u, err := url.Parse(ossURL)
	if err != nil {
		return "", err
	}
	u.Scheme = cdn.scheme
	u.Host = cdn.host
	return u.String(), nil
}

func (cdn *aliCDNStorageMiddleware) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	if cdn.StorageDriver.Name() != "oss" {
		context.GetLogger(ctx).Warn("Alicdn middleware does not support this backend storage driver")
		return cdn.StorageDriver.URLFor(ctx, path, options)
	}

	// With private oss bucket, alicdn can not retrive content directly from oss when cache missed.
	// Unlike cloudfront+S3, we can't grant access privileges of oss to alicdn.
	// we have to construct an oss sigend url and replace the domain part with alicdn,
	// the url looks like: http://my-cdn-domain.com/path/to/file?signature=xxxxxx.
	// if cached missed, alicdn just replace the domain part with oss and get content from oss.
	// The problem is signature changes every request and cdn's will never be hit.
	// So I generate oss url with fixed expire (less than one hour) rather fixed time from now.
	// Before the expire time, all request to the same oss file return the same cdn url, to hit cdn's cache.

	now := time.Now()
	options["expiry"] = cdn.expireOf(now, path)
	ossURL, err := cdn.StorageDriver.URLFor(ctx, path, options)
	if err != nil {
		return "", err
	}
	return cdn.replaceDomain(ossURL)
}
func init() {
	storagemiddleware.Register("alicdn", storagemiddleware.InitFunc(newAliCDNStorageMiddleware))
}

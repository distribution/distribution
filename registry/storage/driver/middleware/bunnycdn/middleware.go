package bunnycdn

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	dcontext "github.com/distribution/distribution/v3/context"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	storagemiddleware "github.com/distribution/distribution/v3/registry/storage/driver/middleware"
	s3 "github.com/distribution/distribution/v3/registry/storage/driver/s3-aws"
)

// DriverName defines the slug of the driver
const DriverName = "bunnycdn"

type bunnyCDNStorageMiddleware struct {
	storagedriver.StorageDriver
	baseURL  string
	token    string
	duration time.Duration
}

var _ storagedriver.StorageDriver = &bunnyCDNStorageMiddleware{}

// newBunnyCDNStorageMiddleware constructs and returns a new BunnyCDN
// StorageDriver implementation.
// Required options: baseurl, token
// Optional options: duration
func newBunnyCDNStorageMiddleware(storageDriver storagedriver.StorageDriver, options map[string]interface{}) (storagedriver.StorageDriver, error) {
	// parse baseurl
	base, ok := options["baseurl"]
	if !ok {
		return nil, fmt.Errorf("no baseurl provided")
	}
	baseURL, ok := base.(string)
	if !ok {
		return nil, fmt.Errorf("baseurl must be a string")
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}
	if _, err := url.Parse(baseURL); err != nil {
		return nil, fmt.Errorf("invalid baseurl: %v", err)
	}

	baseURL = strings.TrimSuffix(baseURL, "/")

	// parse token
	at, ok := options["token"]
	if !ok {
		return nil, fmt.Errorf("no authtype provided")
	}
	token, ok := at.(string)
	if !ok {
		return nil, fmt.Errorf("token must be a string")
	}

	// parse duration
	duration := 20 * time.Minute
	d, ok := options["duration"]
	if ok {
		switch d := d.(type) {
		case time.Duration:
			duration = d
		case string:
			dur, err := time.ParseDuration(d)
			if err != nil {
				return nil, fmt.Errorf("invalid duration: %s", err)
			}
			duration = dur
		}
	}

	return &bunnyCDNStorageMiddleware{
		StorageDriver: storageDriver,
		baseURL:       baseURL,
		token:         token,
		duration:      duration,
	}, nil
}

// URLFor attempts to find a url which may be used to retrieve the file at the given path.
func (bcm *bunnyCDNStorageMiddleware) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {

	if bcm.StorageDriver.Name() != s3.DriverName {
		dcontext.GetLogger(ctx).Warn("The BunnyCDN middleware does not support this backend storage driver")
		return bcm.StorageDriver.URLFor(ctx, path, options)
	}
	expirationTime := time.Now().UTC().Add(bcm.duration).Unix()
	bcSignedToken := getBunnySignedToken(path, bcm.token, expirationTime, "")

	bcURL := fmt.Sprintf("%s%s?token=%s&expires=%d", bcm.baseURL, path, bcSignedToken, expirationTime)
	return bcURL, nil
}

func getBunnySignedToken(path, securityKey string, expirationTime int64, userIP string) string {
	hashableBase := fmt.Sprintf("%s%s%d", securityKey, path, expirationTime)
	if userIP != "" {
		hashableBase = fmt.Sprintf("%s%s", hashableBase, userIP)
	}

	byteValue := []byte(hashableBase)
	hasher := md5.New()
	hasher.Write(byteValue)
	md5sum := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	out := md5sum
	out = strings.ReplaceAll(out, "+", "-")
	out = strings.ReplaceAll(out, "/", "_")
	out = strings.ReplaceAll(out, "=", "")
	return out
}

// init registers the alicdn layerHandler backend.
func init() {
	storagemiddleware.Register(DriverName, storagemiddleware.InitFunc(newBunnyCDNStorageMiddleware))
}

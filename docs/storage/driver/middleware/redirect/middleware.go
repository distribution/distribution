package middleware

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	storagemiddleware "github.com/docker/distribution/registry/storage/driver/middleware"
)

type redirectStorageMiddleware struct {
	storagedriver.StorageDriver
	scheme string
	host   string
}

var _ storagedriver.StorageDriver = &redirectStorageMiddleware{}

func newRedirectStorageMiddleware(sd storagedriver.StorageDriver, options map[string]interface{}) (storagedriver.StorageDriver, error) {
	o, ok := options["baseurl"]
	if !ok {
		return nil, fmt.Errorf("no baseurl provided")
	}
	b, ok := o.(string)
	if !ok {
		return nil, fmt.Errorf("baseurl must be a string")
	}
	if !strings.Contains(b, "://") {
		b = "https://" + b
	}
	u, err := url.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("invalid baseurl: %v", err)
	}

	return &redirectStorageMiddleware{StorageDriver: sd, scheme: u.Scheme, host: u.Host}, nil
}

func (r *redirectStorageMiddleware) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	u := &url.URL{Scheme: r.scheme, Host: r.host, Path: path}
	return u.String(), nil
}

func init() {
	storagemiddleware.Register("redirect", storagemiddleware.InitFunc(newRedirectStorageMiddleware))
}

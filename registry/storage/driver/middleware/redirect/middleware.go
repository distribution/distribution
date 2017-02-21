package middleware

import (
	"errors"
	"fmt"
	"net/url"

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
		return nil, errors.New("no baseurl provided")
	}
	b, ok := o.(string)
	if !ok {
		return nil, errors.New("baseurl must be a string")
	}
	u, err := url.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("unable to parse redirect baseurl: %s", b)
	}
	if u.Scheme == "" {
		return nil, errors.New("no scheme specified for redirect baseurl")
	}
	if u.Host == "" {
		return nil, errors.New("no host specified for redirect baseurl")
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

package middleware

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	storagemiddleware "github.com/distribution/distribution/v3/registry/storage/driver/middleware"
)

func init() {
	storagemiddleware.Register("rewrite", newRewriteStorageMiddleware)
}

type rewriteStorageMiddleware struct {
	storagedriver.StorageDriver
	overrideScheme string
	overrideHost   string
	trimPathPrefix string
}

var _ storagedriver.StorageDriver = &rewriteStorageMiddleware{}

func getStringOption(key string, options map[string]interface{}) (string, error) {
	o, ok := options[key]
	if !ok {
		return "", nil
	}
	s, ok := o.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return s, nil
}

func newRewriteStorageMiddleware(ctx context.Context, sd storagedriver.StorageDriver, options map[string]interface{}) (storagedriver.StorageDriver, error) {
	var err error

	r := &rewriteStorageMiddleware{StorageDriver: sd}

	if r.overrideScheme, err = getStringOption("scheme", options); err != nil {
		return nil, err
	}

	if r.overrideHost, err = getStringOption("host", options); err != nil {
		return nil, err
	}

	if r.trimPathPrefix, err = getStringOption("trimpathprefix", options); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *rewriteStorageMiddleware) URLFor(ctx context.Context, urlPath string, options map[string]interface{}) (string, error) {
	storagePath, err := r.StorageDriver.URLFor(ctx, urlPath, options)
	if err != nil {
		return "", err
	}

	u, err := url.Parse(storagePath)
	if err != nil {
		return "", err
	}

	if r.overrideScheme != "" {
		u.Scheme = r.overrideScheme
	}

	if r.overrideHost != "" {
		u.Host = r.overrideHost
	}

	if r.trimPathPrefix != "" {
		u.Path = strings.TrimPrefix(u.Path, r.trimPathPrefix)
	}

	return u.String(), nil
}

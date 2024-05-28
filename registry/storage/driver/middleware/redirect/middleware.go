package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	storagemiddleware "github.com/distribution/distribution/v3/registry/storage/driver/middleware"
	"github.com/sirupsen/logrus"
)

func init() {
	if err := storagemiddleware.Register("redirect", newRedirectStorageMiddleware); err != nil {
		logrus.Errorf("tailed to register redirect storage middleware: %v", err)
	}
}

type redirectStorageMiddleware struct {
	storagedriver.StorageDriver
	scheme   string
	host     string
	basePath string
}

var _ storagedriver.StorageDriver = &redirectStorageMiddleware{}

func newRedirectStorageMiddleware(ctx context.Context, sd storagedriver.StorageDriver, options map[string]interface{}) (storagedriver.StorageDriver, error) {
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
		return nil, fmt.Errorf("unable to parse redirect baseurl: %s", b)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("no scheme specified for redirect baseurl")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("no host specified for redirect baseurl")
	}

	return &redirectStorageMiddleware{StorageDriver: sd, scheme: u.Scheme, host: u.Host, basePath: u.Path}, nil
}

func (r *redirectStorageMiddleware) RedirectURL(_ *http.Request, urlPath string) (string, error) {
	if r.basePath != "" {
		urlPath = path.Join(r.basePath, urlPath)
	}
	u := &url.URL{Scheme: r.scheme, Host: r.host, Path: urlPath}
	return u.String(), nil
}

package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/stretchr/testify/require"
)

type mockSD struct {
	base.Base
}

func (*mockSD) RedirectURL(_ *http.Request, urlPath string) (string, error) {
	return "http://some.host/some/path/file", nil
}

func TestNoConfig(t *testing.T) {
	options := make(map[string]interface{})
	middleware, err := newRewriteStorageMiddleware(context.Background(), &mockSD{}, options)
	require.NoError(t, err)

	_, ok := middleware.(*rewriteStorageMiddleware)
	require.True(t, ok)

	url, err := middleware.RedirectURL(nil, "")
	require.NoError(t, err)
	require.Equal(t, "http://some.host/some/path/file", url)
}

func TestWrongType(t *testing.T) {
	options := map[string]interface{}{
		"scheme": 1,
	}
	_, err := newRewriteStorageMiddleware(context.TODO(), nil, options)
	require.ErrorContains(t, err, "scheme must be a string")
}

func TestRewriteHostsScheme(t *testing.T) {
	options := map[string]interface{}{
		"scheme": "https",
		"host":   "example.com",
	}

	middleware, err := newRewriteStorageMiddleware(context.TODO(), &mockSD{}, options)
	require.NoError(t, err)

	m, ok := middleware.(*rewriteStorageMiddleware)
	require.True(t, ok)
	require.Equal(t, "https", m.overrideScheme)
	require.Equal(t, "example.com", m.overrideHost)

	url, err := middleware.RedirectURL(nil, "")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/some/path/file", url)
}

func TestTrimPrefix(t *testing.T) {
	options := map[string]interface{}{
		"trimpathprefix": "/some/path",
	}

	middleware, err := newRewriteStorageMiddleware(context.TODO(), &mockSD{}, options)
	require.NoError(t, err)

	m, ok := middleware.(*rewriteStorageMiddleware)
	require.True(t, ok)
	require.Equal(t, "/some/path", m.trimPathPrefix)

	url, err := middleware.RedirectURL(nil, "")
	require.NoError(t, err)
	require.Equal(t, "http://some.host/file", url)
}

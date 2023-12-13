package middleware

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoConfig(t *testing.T) {
	options := make(map[string]interface{})
	_, err := newRedirectStorageMiddleware(context.Background(), nil, options)
	require.ErrorContains(t, err, "no baseurl provided")
}

func TestMissingScheme(t *testing.T) {
	options := make(map[string]interface{})
	options["baseurl"] = "example.com"
	_, err := newRedirectStorageMiddleware(context.Background(), nil, options)
	require.ErrorContains(t, err, "no scheme specified for redirect baseurl")
}

func TestHttpsPort(t *testing.T) {
	options := make(map[string]interface{})
	options["baseurl"] = "https://example.com:5443"
	middleware, err := newRedirectStorageMiddleware(context.Background(), nil, options)
	require.NoError(t, err)

	m, ok := middleware.(*redirectStorageMiddleware)
	require.True(t, ok)
	require.Equal(t, "https", m.scheme)
	require.Equal(t, "example.com:5443", m.host)

	url, err := middleware.RedirectURL(nil, "/rick/data")
	require.NoError(t, err)
	require.Equal(t, "https://example.com:5443/rick/data", url)
}

func TestHTTP(t *testing.T) {
	options := make(map[string]interface{})
	options["baseurl"] = "http://example.com"
	middleware, err := newRedirectStorageMiddleware(context.Background(), nil, options)
	require.NoError(t, err)

	m, ok := middleware.(*redirectStorageMiddleware)
	require.True(t, ok)
	require.Equal(t, "http", m.scheme)
	require.Equal(t, "example.com", m.host)

	url, err := middleware.RedirectURL(nil, "morty/data")
	require.NoError(t, err)
	require.Equal(t, "http://example.com/morty/data", url)
}

func TestPath(t *testing.T) {
	// basePath: end with no slash
	options := make(map[string]interface{})
	options["baseurl"] = "https://example.com/path"
	middleware, err := newRedirectStorageMiddleware(context.Background(), nil, options)
	require.NoError(t, err)

	m, ok := middleware.(*redirectStorageMiddleware)
	require.True(t, ok)
	require.Equal(t, "https", m.scheme)
	require.Equal(t, "example.com", m.host)
	require.Equal(t, "/path", m.basePath)

	// call RedirectURL() with no leading slash
	url, err := middleware.RedirectURL(nil, "morty/data")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/path/morty/data", url)
	// call RedirectURL() with leading slash
	url, err = middleware.RedirectURL(nil, "/morty/data")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/path/morty/data", url)

	// basePath: end with slash
	options["baseurl"] = "https://example.com/path/"
	middleware, err = newRedirectStorageMiddleware(context.Background(), nil, options)
	require.NoError(t, err)

	m, ok = middleware.(*redirectStorageMiddleware)
	require.True(t, ok)
	require.Equal(t, "https", m.scheme)
	require.Equal(t, "example.com", m.host)
	require.Equal(t, "/path/", m.basePath)

	// call RedirectURL() with no leading slash
	url, err = middleware.RedirectURL(nil, "morty/data")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/path/morty/data", url)
	// call RedirectURL() with leading slash
	url, err = middleware.RedirectURL(nil, "/morty/data")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/path/morty/data", url)
}

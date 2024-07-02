package middleware

import (
	"context"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type MiddlewareSuite struct{}

var _ = check.Suite(&MiddlewareSuite{})

type mockSD struct {
	base.Base
}

func (*mockSD) URLFor(ctx context.Context, urlPath string, options map[string]interface{}) (string, error) {
	return "http://some.host/some/path/file", nil
}

func (s *MiddlewareSuite) TestNoConfig(c *check.C) {
	options := make(map[string]interface{})
	middleware, err := newRewriteStorageMiddleware(context.Background(), &mockSD{}, options)
	c.Assert(err, check.Equals, nil)

	_, ok := middleware.(*rewriteStorageMiddleware)
	c.Assert(ok, check.Equals, true)

	url, err := middleware.URLFor(context.Background(), "", nil)
	c.Assert(err, check.Equals, nil)

	c.Assert(url, check.Equals, "http://some.host/some/path/file")
}

func (s *MiddlewareSuite) TestWrongType(c *check.C) {
	options := map[string]interface{}{
		"scheme": 1,
	}
	_, err := newRewriteStorageMiddleware(context.TODO(), nil, options)
	c.Assert(err, check.ErrorMatches, "scheme must be a string")
}

func (s *MiddlewareSuite) TestRewriteHostsScheme(c *check.C) {
	options := map[string]interface{}{
		"scheme": "https",
		"host":   "example.com",
	}

	middleware, err := newRewriteStorageMiddleware(context.TODO(), &mockSD{}, options)
	c.Assert(err, check.Equals, nil)

	m, ok := middleware.(*rewriteStorageMiddleware)
	c.Assert(ok, check.Equals, true)
	c.Assert(m.overrideScheme, check.Equals, "https")
	c.Assert(m.overrideHost, check.Equals, "example.com")

	url, err := middleware.URLFor(context.TODO(), "", nil)
	c.Assert(err, check.Equals, nil)
	c.Assert(url, check.Equals, "https://example.com/some/path/file")
}

func (s *MiddlewareSuite) TestTrimPrefix(c *check.C) {
	options := map[string]interface{}{
		"trimpathprefix": "/some/path",
	}

	middleware, err := newRewriteStorageMiddleware(context.TODO(), &mockSD{}, options)
	c.Assert(err, check.Equals, nil)

	m, ok := middleware.(*rewriteStorageMiddleware)
	c.Assert(ok, check.Equals, true)
	c.Assert(m.trimPathPrefix, check.Equals, "/some/path")

	url, err := middleware.URLFor(context.TODO(), "", nil)
	c.Assert(err, check.Equals, nil)
	c.Assert(url, check.Equals, "http://some.host/file")
}

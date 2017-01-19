package middleware

import (
	"testing"

	check "gopkg.in/check.v1"
	"time"
)

func Test(t *testing.T) {
	check.TestingT(t)
}

type AliCDNMiddlewareSuite struct{}

var _ = check.Suite(&AliCDNMiddlewareSuite{})

func (s *AliCDNMiddlewareSuite) TestNoConfig(c *check.C) {
	options := make(map[string]interface{})
	_, err := newAliCDNStorageMiddleware(nil, options)
	c.Assert(err, check.ErrorMatches, "no baseurl provided")
}

func (s *AliCDNMiddlewareSuite) TestMissScheme(c *check.C) {
	options := make(map[string]interface{})
	options["baseurl"] = "example.com"
	_, err := newAliCDNStorageMiddleware(nil, options)
	c.Assert(err, check.ErrorMatches, "no scheme specified for alicdn baseurl")
}

func (s *AliCDNMiddlewareSuite) TestFullConfig(c *check.C) {
	options := make(map[string]interface{})
	options["baseurl"] = "http://example.com"
	middleware, err := newAliCDNStorageMiddleware(nil, options)
	c.Assert(err, check.IsNil)
	m, ok := middleware.(*aliCDNStorageMiddleware)
	c.Assert(ok, check.Equals, true)
	c.Assert(m.scheme, check.Equals, "http")
	c.Assert(m.host, check.Equals, "example.com")
}

func (s *AliCDNMiddlewareSuite) TestExpireOf(c *check.C) {
	options := make(map[string]interface{})
	options["baseurl"] = "http://example.com"
	middleware, err := newAliCDNStorageMiddleware(nil, options)

	c.Assert(err, check.IsNil)
	m, ok := middleware.(*aliCDNStorageMiddleware)
	c.Assert(ok, check.Equals, true)

	layout := "2006-01-02T15:04:05.000"
	path := "/path/to/file" // 12 minutes and 2 seconds past the hour

	t, err := time.Parse(layout, "2017-01-12T16:01:20.123")
	c.Assert(err, check.IsNil)
	expire := m.expireOf(t, path)
	c.Assert(expire.Format(layout), check.Equals, "2017-01-12T16:12:02.000")

	t, err = time.Parse(layout, "2017-01-12T16:05:20.123")
	c.Assert(err, check.IsNil)
	expire = m.expireOf(t, path)
	c.Assert(expire.Format(layout), check.Equals, "2017-01-12T16:12:02.000") // expire not change

	t, err = time.Parse(layout, "2017-01-12T16:10:20.123") // less than 5min to expire
	c.Assert(err, check.IsNil)
	expire = m.expireOf(t, path)
	c.Assert(expire.Format(layout), check.Equals, "2017-01-12T17:12:02.000")

	t, err = time.Parse(layout, "2017-01-12T16:20:20.123") // past expire point in current hour
	c.Assert(err, check.IsNil)
	expire = m.expireOf(t, path)
	c.Assert(expire.Format(layout), check.Equals, "2017-01-12T17:12:02.000")
}

func (s *aliCDNStorageMiddleware) TestReplaceDomain(c *check.C) {
	options := make(map[string]interface{})
	for _, baseurl := range []string{"http://example.com", "https://example.com"} {
		options["baseurl"] = baseurl

		middleware, err := newAliCDNStorageMiddleware(nil, options)

		c.Assert(err, check.IsNil)
		m, ok := middleware.(*aliCDNStorageMiddleware)
		c.Assert(ok, check.Equals, true)

		for _, ossDomain := range []string{"http://bucket.oss.aliyuncs.com", "https://another.oss.aliyuncs.com"} {
			ossURL := ossDomain + "/path/to/file?signature=xxxxxx"
			cdnURL, err := m.replaceDomain(ossURL)
			c.Assert(err, check.IsNil)
			c.Assert(cdnURL, check.Equals, baseurl+"/path/to/file?signature=xxxxxx")
		}
	}
}

package middleware

import (
	"testing"

	check "gopkg.in/check.v1"
	"io/ioutil"
	"os"
)

func Test(t *testing.T) { check.TestingT(t) }

type MiddlewareSuite struct{}

var _ = check.Suite(&MiddlewareSuite{})

func (s *MiddlewareSuite) TestNoConfig(c *check.C) {
	options := make(map[string]interface{})
	_, err := newCloudFrontStorageMiddleware(nil, options)
	c.Assert(err, check.ErrorMatches, "no baseurl provided")
}

func TestCloudFrontStorageMiddlewareGenerateKey(t *testing.T) {

	options := make(map[string]interface{})
	options["baseurl"] = "example.com"

	var privk = `-----BEGIN RSA PRIVATE KEY-----
MIICXQIBAAKBgQCy0ZZsItDuYoX3y6hWqyU9YdH/0B+tlOhvjlaJqvkmAIBBatVV
VAShnEAEircBwV3i08439WYgjXnrZ0FjXBTjTKWwCsbpuWJY1w8hqHW3VDivUo1n
F9WTeclVJuEMhmiAhek3dhUdATaEDqBNskXMofSgKmQHqhPdXCgDmnzKoQIDAQAB
AoGBAJM0xI8qrjLAeqa+SktmwtZgM99StvFPt3U2iPj1/fsRyIOR7iM7ckCUf4L9
qqBQTfjQAmDArR05OlfW/dZM1IfUagiAh+Ss7KTt+re1U0sNwoAk8yJlbYAD+0Qy
vuMowSDoMnGe/5RJbdqK9n5lUZ7aZk8ybumJeuHb/ykVkU7tAkEA6LoqdQAZ9wwX
7l0gewwCiAFCYMTuGQcvd5OcjToeCQOgn94YZHQybm1DtGg3+c1raVE5M0xw7Hbs
P6KCC+Le4wJBAMSzXB7DpBFOpd8AvGNkfo/ESGCDHg3JbNxQh531zeD6Gmm4uEF+
42J1CVMyPLw5NoBh83GK08FftwN9xXIZw6sCQBnfiJTVXA2hJI/1foTvguCH8086
1ZWmvNo4aPEyguBRrOvZDzEr0eeA8kP+SirVcZmV1Bwl5XAEkKNKd9bGdC0CQFLi
wY61Ig2o9nxh8wBu+GXccCM7HQ7yMc0kogEN8xM6UKb8D6iJr4dtieBk6vLlqPGw
VMUjmteBXb064liSQsECQQDAdw9jH1Y7SJf/aujlrIuzeei3hJ6HdP1OrfM24CK1
pZeMRablbPQdp8/1NyIwimq1VlG0ohQ4P6qhW7E09ZMC
-----END RSA PRIVATE KEY-----
`

	file, err := ioutil.TempFile("", "pkey")
	if err != nil {
		t.Fatal("File cannot be created")
	}
	file.WriteString(privk)
	defer os.Remove(file.Name())
	options["privatekey"] = file.Name()
	options["keypairid"] = "test"
	storageDriver, err := newCloudFrontStorageMiddleware(nil, options)
	if err != nil {
		t.Fatal(err)
	}
	if storageDriver == nil {
		t.Fatal("Driver couldnt be initialized.")
	}
}

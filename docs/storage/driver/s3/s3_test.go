package s3

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/AdRoll/goamz/aws"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

type S3DriverConstructor func(rootDirectory string) (*Driver, error)

func init() {
	accessKey := os.Getenv("AWS_ACCESS_KEY")
	secretKey := os.Getenv("AWS_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	encrypt := os.Getenv("S3_ENCRYPT")
	secure := os.Getenv("S3_SECURE")
	v4auth := os.Getenv("S3_USE_V4_AUTH")
	region := os.Getenv("AWS_REGION")
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	s3DriverConstructor := func(rootDirectory string) (*Driver, error) {
		encryptBool := false
		if encrypt != "" {
			encryptBool, err = strconv.ParseBool(encrypt)
			if err != nil {
				return nil, err
			}
		}

		secureBool := true
		if secure != "" {
			secureBool, err = strconv.ParseBool(secure)
			if err != nil {
				return nil, err
			}
		}

		v4AuthBool := false
		if v4auth != "" {
			v4AuthBool, err = strconv.ParseBool(v4auth)
			if err != nil {
				return nil, err
			}
		}

		parameters := DriverParameters{
			accessKey,
			secretKey,
			bucket,
			aws.GetRegion(region),
			encryptBool,
			secureBool,
			v4AuthBool,
			minChunkSize,
			rootDirectory,
		}

		return New(parameters)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipCheck := func() string {
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			return "Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, AWS_REGION, S3_BUCKET, and S3_ENCRYPT to run S3 tests"
		}
		return ""
	}

	driverConstructor := func() (storagedriver.StorageDriver, error) {
		return s3DriverConstructor(root)
	}

	testsuites.RegisterInProcessSuite(driverConstructor, skipCheck)

	// s3Constructor := func() (*Driver, error) {
	// 	return s3DriverConstructor(aws.GetRegion(region))
	// }

	RegisterS3DriverSuite(s3DriverConstructor, skipCheck)

	// testsuites.RegisterIPCSuite(driverName, map[string]string{
	// 	"accesskey": accessKey,
	// 	"secretkey": secretKey,
	// 	"region":    region.Name,
	// 	"bucket":    bucket,
	// 	"encrypt":   encrypt,
	// }, skipCheck)
	// }
}

func RegisterS3DriverSuite(s3DriverConstructor S3DriverConstructor, skipCheck testsuites.SkipCheck) {
	check.Suite(&S3DriverSuite{
		Constructor: s3DriverConstructor,
		SkipCheck:   skipCheck,
	})
}

type S3DriverSuite struct {
	Constructor S3DriverConstructor
	testsuites.SkipCheck
}

func (suite *S3DriverSuite) SetUpSuite(c *check.C) {
	if reason := suite.SkipCheck(); reason != "" {
		c.Skip(reason)
	}
}

func (suite *S3DriverSuite) TestEmptyRootList(c *check.C) {
	validRoot, err := ioutil.TempDir("", "driver-")
	c.Assert(err, check.IsNil)
	defer os.Remove(validRoot)

	rootedDriver, err := suite.Constructor(validRoot)
	c.Assert(err, check.IsNil)
	emptyRootDriver, err := suite.Constructor("")
	c.Assert(err, check.IsNil)
	slashRootDriver, err := suite.Constructor("/")
	c.Assert(err, check.IsNil)

	filename := "/test"
	contents := []byte("contents")
	err = rootedDriver.PutContent(filename, contents)
	c.Assert(err, check.IsNil)
	defer rootedDriver.Delete(filename)

	keys, err := emptyRootDriver.List("/")
	for _, path := range keys {
		c.Assert(storagedriver.PathRegexp.MatchString(path), check.Equals, true)
	}

	keys, err = slashRootDriver.List("/")
	for _, path := range keys {
		c.Assert(storagedriver.PathRegexp.MatchString(path), check.Equals, true)
	}
}

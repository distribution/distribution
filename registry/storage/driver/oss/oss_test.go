package oss

import (
	alioss "github.com/denverdino/aliyungo/oss"
	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"testing"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

type OSSDriverConstructor func(rootDirectory string) (*Driver, error)

func init() {
	accessKey := os.Getenv("ALIYUN_ACCESS_KEY_ID")
	secretKey := os.Getenv("ALIYUN_ACCESS_KEY_SECRET")
	bucket := os.Getenv("OSS_BUCKET")
	region := os.Getenv("OSS_REGION")
	encrypt := os.Getenv("OSS_ENCRYPT")
	secure := os.Getenv("OSS_SECURE")
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

		parameters := DriverParameters{
			accessKey,
			secretKey,
			bucket,
			alioss.Region(region),
			encryptBool,
			secureBool,
			minChunkSize,
			rootDirectory,
		}

		return New(parameters)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipCheck := func() string {
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			log.Println("Missing something")
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

func RegisterS3DriverSuite(s3DriverConstructor OSSDriverConstructor, skipCheck testsuites.SkipCheck) {
	check.Suite(&S3DriverSuite{
		Constructor: s3DriverConstructor,
		SkipCheck:   skipCheck,
	})
}

type S3DriverSuite struct {
	Constructor OSSDriverConstructor
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
	ctx := context.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	c.Assert(err, check.IsNil)
	defer rootedDriver.Delete(ctx, filename)

	keys, err := emptyRootDriver.List(ctx, "/")
	for _, path := range keys {
		c.Assert(storagedriver.PathRegexp.MatchString(path), check.Equals, true)
	}

	keys, err = slashRootDriver.List(ctx, "/")
	for _, path := range keys {
		c.Assert(storagedriver.PathRegexp.MatchString(path), check.Equals, true)
	}
}

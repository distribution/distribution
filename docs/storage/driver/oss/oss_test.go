package oss

import (
	alioss "github.com/denverdino/aliyungo/oss"
	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"io/ioutil"
	//"log"
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
	internal := os.Getenv("OSS_INTERNAL")
	encrypt := os.Getenv("OSS_ENCRYPT")
	secure := os.Getenv("OSS_SECURE")
	endpoint := os.Getenv("OSS_ENDPOINT")
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	ossDriverConstructor := func(rootDirectory string) (*Driver, error) {
		encryptBool := false
		if encrypt != "" {
			encryptBool, err = strconv.ParseBool(encrypt)
			if err != nil {
				return nil, err
			}
		}

		secureBool := false
		if secure != "" {
			secureBool, err = strconv.ParseBool(secure)
			if err != nil {
				return nil, err
			}
		}

		internalBool := false
		if internal != "" {
			internalBool, err = strconv.ParseBool(internal)
			if err != nil {
				return nil, err
			}
		}

		parameters := DriverParameters{
			AccessKeyID:     accessKey,
			AccessKeySecret: secretKey,
			Bucket:          bucket,
			Region:          alioss.Region(region),
			Internal:        internalBool,
			ChunkSize:       minChunkSize,
			RootDirectory:   rootDirectory,
			Encrypt:         encryptBool,
			Secure:          secureBool,
			Endpoint:        endpoint,
		}

		return New(parameters)
	}

	// Skip OSS storage driver tests if environment variable parameters are not provided
	skipCheck := func() string {
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			return "Must set ALIYUN_ACCESS_KEY_ID, ALIYUN_ACCESS_KEY_SECRET, OSS_REGION, OSS_BUCKET, and OSS_ENCRYPT to run OSS tests"
		}
		return ""
	}

	driverConstructor := func() (storagedriver.StorageDriver, error) {
		return ossDriverConstructor(root)
	}

	testsuites.RegisterInProcessSuite(driverConstructor, skipCheck)

	// ossConstructor := func() (*Driver, error) {
	// 	return ossDriverConstructor(aws.GetRegion(region))
	// }

	RegisterOSSDriverSuite(ossDriverConstructor, skipCheck)

	// testsuites.RegisterIPCSuite(driverName, map[string]string{
	// 	"accesskey": accessKey,
	// 	"secretkey": secretKey,
	// 	"region":    region.Name,
	// 	"bucket":    bucket,
	// 	"encrypt":   encrypt,
	// }, skipCheck)
	// }
}

func RegisterOSSDriverSuite(ossDriverConstructor OSSDriverConstructor, skipCheck testsuites.SkipCheck) {
	check.Suite(&OSSDriverSuite{
		Constructor: ossDriverConstructor,
		SkipCheck:   skipCheck,
	})
}

type OSSDriverSuite struct {
	Constructor OSSDriverConstructor
	testsuites.SkipCheck
}

func (suite *OSSDriverSuite) SetUpSuite(c *check.C) {
	if reason := suite.SkipCheck(); reason != "" {
		c.Skip(reason)
	}
}

func (suite *OSSDriverSuite) TestEmptyRootList(c *check.C) {
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

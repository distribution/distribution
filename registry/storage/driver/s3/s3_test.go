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

	s3DriverConstructor := func(region aws.Region) (storagedriver.StorageDriver, error) {
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

		v4AuthBool := true
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
			region,
			encryptBool,
			secureBool,
			v4AuthBool,
			minChunkSize,
			root,
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

	// for _, region := range aws.Regions {
	// 	if region == aws.USGovWest {
	// 		continue
	// 	}

	testsuites.RegisterInProcessSuite(func() (storagedriver.StorageDriver, error) {
		return s3DriverConstructor(aws.GetRegion(region))
	}, skipCheck)
	// testsuites.RegisterIPCSuite(driverName, map[string]string{
	// 	"accesskey": accessKey,
	// 	"secretkey": secretKey,
	// 	"region":    region.Name,
	// 	"bucket":    bucket,
	// 	"encrypt":   encrypt,
	// }, skipCheck)
	// }
}

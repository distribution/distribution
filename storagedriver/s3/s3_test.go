// +build ignore

package s3

import (
	"os"
	"strconv"
	"testing"

	"github.com/crowdmob/goamz/aws"
	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

func init() {
	accessKey := os.Getenv("AWS_ACCESS_KEY")
	secretKey := os.Getenv("AWS_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	encrypt := os.Getenv("S3_ENCRYPT")

	s3DriverConstructor := func(region aws.Region) (storagedriver.StorageDriver, error) {
		shouldEncrypt, err := strconv.ParseBool(encrypt)
		if err != nil {
			return nil, err
		}
		return New(accessKey, secretKey, region, shouldEncrypt, bucket)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipCheck := func() string {
		if accessKey == "" || secretKey == "" || bucket == "" || encrypt == "" {
			return "Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, S3_BUCKET, and S3_ENCRYPT to run S3 tests"
		}
		return ""
	}

	for _, region := range aws.Regions {
		if region == aws.USGovWest {
			continue
		}

		testsuites.RegisterInProcessSuite(s3DriverConstructor(region), skipCheck)
		testsuites.RegisterIPCSuite(driverName, map[string]string{
			"accesskey": accessKey,
			"secretkey": secretKey,
			"region":    region.Name,
			"bucket":    bucket,
			"encrypt":   encrypt,
		}, skipCheck)
	}
}

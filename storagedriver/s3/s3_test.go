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
	region := os.Getenv("AWS_REGION")
	bucket := os.Getenv("S3_BUCKET")
	encrypt := os.Getenv("S3_ENCRYPT")

	s3DriverConstructor := func() (storagedriver.StorageDriver, error) {
		shouldEncrypt, err := strconv.ParseBool(encrypt)
		if err != nil {
			return nil, err
		}
		return New(accessKey, secretKey, aws.GetRegion(region), shouldEncrypt, bucket)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipCheck := func() string {
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			return "Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, AWS_REGION, S3_BUCKET, and S3_ENCRYPT to run S3 tests"
		}
		return ""
	}

	testsuites.RegisterInProcessSuite(s3DriverConstructor, skipCheck)
	testsuites.RegisterIPCSuite(driverName, map[string]string{
		"accesskey": accessKey,
		"secretkey": secretKey,
		"region":    region,
		"bucket":    bucket,
		"encrypt":   encrypt,
	}, skipCheck)
}

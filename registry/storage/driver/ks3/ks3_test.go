// +build include_ks3

package ks3

import (
	"io/ioutil"
	"os"
	"strconv"
	"testing"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var ks3DriverConstructor func(rootDirectory string) (*Driver, error)

var skipKS3Check func() string

func init() {
	accessKey := os.Getenv("KS3_ACCESS_KEY")
	secretKey := os.Getenv("KS3_SECRET_KEY")
	bucket := os.Getenv("KS3_BUCKET")
	encrypt := os.Getenv("KS3_ENCRYPT")
	secure := os.Getenv("KS3_SECURE")
	region := os.Getenv("KS3_REGION")
	internal := os.Getenv("KS3_INTERNAL")
	regionEndpoint := os.Getenv("KS3_REGIONENDPOINT")
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	ks3DriverConstructor = func(rootDirectory string) (*Driver, error) {
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

		internalBool := false
		if internal != "" {
			internalBool, err = strconv.ParseBool(internal)
			if err != nil {
				return nil, err
			}
		}

		parameters := DriverParameters{
			accessKey,
			secretKey,
			bucket,
			region,
			internalBool,
			encryptBool,
			secureBool,
			minChunkSize,
			rootDirectory,
			regionEndpoint,
		}

		return New(parameters)
	}

	// Skip KS3 storage driver tests if environment variable parameters are not provided
	skipKS3Check = func() string {
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			return "Must set KS3_ACCESS_KEY, KS3_SECRET_KEY, KS3_REGION, KS3_BUCKET, and KS3_ENCRYPT to run KS3 tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return ks3DriverConstructor(root)
	}, skipKS3Check)
}

func TestEmptyRootList(t *testing.T) {
	if skipKS3Check() != "" {
		t.Skip(skipKS3Check())
	}

	validRoot, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(validRoot)

	rootedDriver, err := ks3DriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := ks3DriverConstructor("")
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := ks3DriverConstructor("/")
	if err != nil {
		t.Fatalf("unexpected error creating slash root driver: %v", err)
	}

	filename := "/test"
	contents := []byte("contents")
	ctx := context.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer rootedDriver.Delete(ctx, filename)

	keys, err := emptyRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}

	keys, err = slashRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}
}

// +build include_bos

package bos

import (
	"io/ioutil"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	//"log"
	"os"
	"strconv"
	"testing"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var bosDriverConstructor func(rootDirectory string) (*Driver, error)

var skipCheck func() string

func init() {
	accessKey := os.Getenv("BAIDU_BCE_AK")
	secretKey := os.Getenv("BAIDU_BCE_SK")
	bucket := os.Getenv("BOS_BUCKET")
	region := os.Getenv("BOS_REGION")
	secure := os.Getenv("BOS_SECURE")
	endpoint := os.Getenv("BOS_ENDPOINT")
	debug := os.Getenv("BOS_DEBUG")
	root, err := ioutil.TempDir("", "driver-")

	if err != nil {
		panic(err)
	}

	defer os.Remove(root)

	bosDriverConstructor = func(rootDirectory string) (*Driver, error) {
		secureBool := false
		if secure != "" {
			secureBool, err = strconv.ParseBool(secure)
			if err != nil {
				return nil, err
			}
		}

		debugBool := false
		if debug != "" {
			debugBool, err = strconv.ParseBool(debug)
			if err != nil {
				return nil, err
			}
		}

		parameters := DriverParameters{
			AccessKeyID:     accessKey,
			AccessKeySecret: secretKey,
			Bucket:          bucket,
			Region:          region,
			ChunkSize:       minChunkSize,
			RootDirectory:   rootDirectory,
			Secure:          secureBool,
			Endpoint:        endpoint,
			Debug:           debugBool,
		}

		return New(parameters)
	}

	// Skip BOS storage driver tests if environment variable parameters are not provided
	skipCheck = func() string {
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" {
			return "Must set BAIDU_BCE_AK, BAIDU_BCE_SK, BOS_REGION, and BOS_BUCKET to run BOS tests"
		}

		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return bosDriverConstructor(root)
	}, skipCheck)
}

func TestEmptyRootList(t *testing.T) {
	if skipCheck() != "" {
		t.Skip(skipCheck())
	}

	validRoot, err := ioutil.TempDir("", "driver-")

	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}

	defer os.Remove(validRoot)

	rootedDriver, err := bosDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := bosDriverConstructor("")
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := bosDriverConstructor("/")
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

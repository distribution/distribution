package qingstor

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

var qsDriverConstructor func(rootDirectory string) (*Driver, error)
var skipQS func() string

func init() {
	accessKey := os.Getenv("QS_ACCESS_KEY")
	secretKey := os.Getenv("QS_SECRET_KEY")
	bucket := os.Getenv("QS_BUCKET")
	zone := os.Getenv("QS_ZONE")
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	qsDriverConstructor = func(rootDirectory string) (*Driver, error) {

		parameters := DriverParameters{
			AccessKey:     fmt.Sprint(accessKey),
			SecretKey:     fmt.Sprint(secretKey),
			Bucket:        fmt.Sprint(bucket),
			Zone:          fmt.Sprint(zone),
			ChunkSize:     2 * 4 << 20,
			UserAgent:     "",
			RootDirectory: rootDirectory,
		}

		return New(parameters)
	}

	skipQS = func() string {
		if accessKey == "" || secretKey == "" || bucket == "" || zone == "" {
			return "Must set QS_ACCESS_KEY, QS_SECRET_KEY, QS_BUCKET, QS_ZONE, and QS_ROOTDIRECTORY to run QS tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return qsDriverConstructor(root)
	}, skipQS)
}

func TestEmptyRootList(t *testing.T) {
	if skipQS() != "" {
		t.Skip(skipQS())
	}

	validRoot, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(validRoot)

	rootedDriver, err := qsDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	filename := "/test"
	contents := []byte("contents")
	ctx := context.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer rootedDriver.Delete(ctx, filename)

	keys, err := rootedDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}
}

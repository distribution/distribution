// +build include_gcs

package gcs

import (
	"io/ioutil"
	"os"
	"testing"

	ctx "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var gcsDriverConstructor func(rootDirectory string) (storagedriver.StorageDriver, error)
var skipGCS func() string

func init() {
	bucket := os.Getenv("REGISTRY_STORAGE_GCS_BUCKET")
	keyfile := os.Getenv("REGISTRY_STORAGE_GCS_KEYFILE")
	credentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	gcsDriverConstructor = func(rootDirectory string) (storagedriver.StorageDriver, error) {

		parameters := driverParameters{
			bucket,
			keyfile,
			rootDirectory,
		}

		return New(parameters)
	}

	// Skip GCS storage driver tests if environment variable parameters are not provided
	skipGCS = func() string {
		if bucket == "" || (credentials == "" && keyfile == "") {
			return "Must set REGISTRY_STORAGE_GCS_BUCKET and (GOOGLE_APPLICATION_CREDENTIALS or REGISTRY_STORAGE_GCS_KEYFILE) to run GCS tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return gcsDriverConstructor(root)
	}, skipGCS)
}

func TestEmptyRootList(t *testing.T) {
	if skipGCS() != "" {
		t.Skip(skipGCS())
	}

	validRoot, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(validRoot)

	rootedDriver, err := gcsDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := gcsDriverConstructor("")
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := gcsDriverConstructor("/")
	if err != nil {
		t.Fatalf("unexpected error creating slash root driver: %v", err)
	}

	filename := "/test"
	contents := []byte("contents")
	ctx := ctx.Background()
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

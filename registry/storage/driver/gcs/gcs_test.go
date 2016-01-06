// +build include_gcs

package gcs

import (
	"io/ioutil"
	"os"
	"testing"

	"fmt"
	ctx "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/cloud/storage"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var gcsDriverConstructor func(rootDirectory string) (storagedriver.StorageDriver, error)
var skipGCS func() string

func init() {
	bucket := os.Getenv("REGISTRY_STORAGE_GCS_BUCKET")
	credentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	// Skip GCS storage driver tests if environment variable parameters are not provided
	skipGCS = func() string {
		if bucket == "" || credentials == "" {
			return "The following environment variables must be set to enable these tests: REGISTRY_STORAGE_GCS_BUCKET, REGISTRY_STORAGE_GCS_CREDS"
		}
		return ""
	}

	if skipGCS() != "" {
		return
	}

	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	_, err = os.Stat(credentials)
	if err == nil {
		jsonKey, err := ioutil.ReadFile(credentials)
		if err != nil {
			panic(fmt.Sprintf("Unable to read credentials from file : %s", err))
		}
		credentials = string(jsonKey)
	}

	// Assume that the file contents are within the environment variable since it exists
	// but does not contain a valid file path
	jwtConfig, err := google.JWTConfigFromJSON([]byte(credentials), storage.ScopeFullControl)
	if err != nil {
		panic(fmt.Sprintf("Error reading JWT config : %s", err))
	}

	gcsDriverConstructor = func(rootDirectory string) (storagedriver.StorageDriver, error) {
		parameters := driverParameters{
			bucket:        bucket,
			rootDirectory: root,
			email:         jwtConfig.Email,
			privateKey:    []byte(jwtConfig.PrivateKey),
			client:        oauth2.NewClient(ctx.Background(), jwtConfig.TokenSource(ctx.Background())),
		}

		return New(parameters)
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return gcsDriverConstructor(root)
	}, skipGCS)
}

func TestRetry(t *testing.T) {
	if skipGCS() != "" {
		t.Skip(skipGCS())
	}

	assertError := func(expected string, observed error) {
		observedMsg := "<nil>"
		if observed != nil {
			observedMsg = observed.Error()
		}
		if observedMsg != expected {
			t.Fatalf("expected %v, observed %v\n", expected, observedMsg)
		}
	}

	err := retry(2, func() error {
		return &googleapi.Error{
			Code:    503,
			Message: "google api error",
		}
	})
	assertError("googleapi: Error 503: google api error", err)

	err = retry(2, func() error {
		return &googleapi.Error{
			Code:    404,
			Message: "google api error",
		}
	})
	assertError("googleapi: Error 404: google api error", err)

	err = retry(2, func() error {
		return fmt.Errorf("error")
	})
	assertError("error", err)
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

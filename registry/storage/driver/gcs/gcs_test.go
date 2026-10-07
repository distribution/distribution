package gcs

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/distribution/distribution/v3/internal/dcontext"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"google.golang.org/api/googleapi"
)

var (
	gcsDriverConstructor func(rootDirectory string) (storagedriver.StorageDriver, error)
	skipCheck            func(tb testing.TB)
)

func init() {
	bucket := os.Getenv("REGISTRY_STORAGE_GCS_BUCKET")
	credentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	emulatorHost := os.Getenv("STORAGE_EMULATOR_HOST")

	// Skip unless configured for the emulator (bucket + STORAGE_EMULATOR_HOST) or
	// real GCS (bucket + GOOGLE_APPLICATION_CREDENTIALS).
	skipCheck = func(tb testing.TB) {
		tb.Helper()

		if bucket == "" {
			tb.Skip("REGISTRY_STORAGE_GCS_BUCKET must be set to enable these tests")
		}
		if emulatorHost == "" && credentials == "" {
			tb.Skip("Set STORAGE_EMULATOR_HOST for the local emulator, or GOOGLE_APPLICATION_CREDENTIALS for real GCS")
		}
	}

	gcsDriverConstructor = func(rootDirectory string) (storagedriver.StorageDriver, error) {
		params := map[string]any{
			"bucket":         bucket,
			"rootdirectory":  rootDirectory,
			"chunksize":      defaultChunkSize,
			"maxconcurrency": uint64(8),
		}
		if credentials != "" {
			params["keyfile"] = credentials
		}

		return FromParameters(context.Background(), params)
	}
}

func newDriverConstructor(tb testing.TB) testsuites.DriverConstructor {
	root := tb.TempDir()

	return func() (storagedriver.StorageDriver, error) {
		return gcsDriverConstructor(root)
	}
}

func TestGCSDriverSuite(t *testing.T) {
	skipCheck(t)
	testsuites.Driver(t, newDriverConstructor(t), false)
}

func BenchmarkGCSDriverSuite(b *testing.B) {
	skipCheck(b)
	testsuites.BenchDriver(b, newDriverConstructor(b))
}

// Test Committing a FileWriter without having called Write
func TestCommitEmpty(t *testing.T) {
	skipCheck(t)

	validRoot := t.TempDir()

	driver, err := gcsDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	filename := "/test"
	ctx := dcontext.Background()

	writer, err := driver.Writer(ctx, filename, false)
	// nolint:errcheck
	defer driver.Delete(ctx, filename)
	if err != nil {
		t.Fatalf("driver.Writer: unexpected error: %v", err)
	}
	err = writer.Commit(context.Background())
	if err != nil {
		t.Fatalf("writer.Commit: unexpected error: %v", err)
	}
	err = writer.Close()
	if err != nil {
		t.Fatalf("writer.Close: unexpected error: %v", err)
	}
	if writer.Size() != 0 {
		t.Fatalf("writer.Size: %d != 0", writer.Size())
	}
	readContents, err := driver.GetContent(ctx, filename)
	if err != nil {
		t.Fatalf("driver.GetContent: unexpected error: %v", err)
	}
	if len(readContents) != 0 {
		t.Fatalf("len(driver.GetContent(..)): %d != 0", len(readContents))
	}
}

// Test Committing a FileWriter after having written exactly
// defaultChunksize bytes.
func TestCommit(t *testing.T) {
	skipCheck(t)

	validRoot := t.TempDir()

	driver, err := gcsDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	filename := "/test"
	ctx := dcontext.Background()

	contents := make([]byte, defaultChunkSize)
	writer, err := driver.Writer(ctx, filename, false)
	// nolint:errcheck
	defer driver.Delete(ctx, filename)
	if err != nil {
		t.Fatalf("driver.Writer: unexpected error: %v", err)
	}
	_, err = writer.Write(contents)
	if err != nil {
		t.Fatalf("writer.Write: unexpected error: %v", err)
	}
	err = writer.Commit(context.Background())
	if err != nil {
		t.Fatalf("writer.Commit: unexpected error: %v", err)
	}
	err = writer.Close()
	if err != nil {
		t.Fatalf("writer.Close: unexpected error: %v", err)
	}
	if writer.Size() != int64(len(contents)) {
		t.Fatalf("writer.Size: %d != %d", writer.Size(), len(contents))
	}
	readContents, err := driver.GetContent(ctx, filename)
	if err != nil {
		t.Fatalf("driver.GetContent: unexpected error: %v", err)
	}
	if len(readContents) != len(contents) {
		t.Fatalf("len(driver.GetContent(..)): %d != %d", len(readContents), len(contents))
	}
}

func TestRetry(t *testing.T) {
	skipCheck(t)

	assertError := func(expected string, observed error) {
		observedMsg := "<nil>"
		if observed != nil {
			observedMsg = observed.Error()
		}
		if observedMsg != expected {
			t.Fatalf("expected %v, observed %v\n", expected, observedMsg)
		}
	}

	err := retry(func() error {
		return &googleapi.Error{
			Code:    503,
			Message: "google api error",
		}
	})
	assertError("googleapi: Error 503: google api error", err)

	err = retry(func() error {
		return &googleapi.Error{
			Code:    404,
			Message: "google api error",
		}
	})
	assertError("googleapi: Error 404: google api error", err)

	err = retry(func() error {
		return fmt.Errorf("error")
	})
	assertError("error", err)
}

func TestEmptyRootList(t *testing.T) {
	skipCheck(t)

	validRoot := t.TempDir()

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
	ctx := dcontext.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer func() {
		err := rootedDriver.Delete(ctx, filename)
		if err != nil {
			t.Fatalf("failed to remove %v due to %v\n", filename, err)
		}
	}()
	keys, err := emptyRootDriver.List(ctx, "/")
	if err != nil {
		t.Fatalf("unexpected error listing empty root content: %v", err)
	}
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}

	keys, err = slashRootDriver.List(ctx, "/")
	if err != nil {
		t.Fatalf("unexpected error listing slash root content: %v", err)
	}
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}
}

// TestMoveDirectory checks that moving a directory returns an error.
func TestMoveDirectory(t *testing.T) {
	skipCheck(t)

	validRoot := t.TempDir()

	driver, err := gcsDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	ctx := dcontext.Background()
	contents := []byte("contents")
	// Create a regular file.
	err = driver.PutContent(ctx, "/parent/dir/foo", contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer func() {
		err := driver.Delete(ctx, "/parent")
		if err != nil {
			t.Fatalf("failed to remove /parent due to %v\n", err)
		}
	}()

	err = driver.Move(ctx, "/parent/dir", "/parent/other")
	if err == nil {
		t.Fatal("Moving directory /parent/dir /parent/other should have return a non-nil error")
	}
}

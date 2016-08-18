// +build include_gdrive

package gdrive

import (
	"os"
	"testing"

	ctx "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	check "gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

var gdriveDriverConstructor func(rootDirectory string) (storagedriver.StorageDriver, error)
var skipGdrive func() string

func init() {

	keyFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS_FILE")

	// Skip GCS storage driver tests if environment variable parameters are not provided
	skipGdrive = func() string {
		if keyFile == "" {
			return "The following environment variables must be set to enable these tests: GOOGLE_APPLICATION_CREDENTIALS_FILE"
		}
		return ""
	}

	if skipGdrive() != "" {
		return
	}

	root := "testdir"
	gdriveDriverConstructor = func(rootDirectory string) (storagedriver.StorageDriver, error) {
		parameters := DriverParameters{
			KeyFile:       keyFile,
			RootDirectory: rootDirectory,
		}

		return New(parameters), nil
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return gdriveDriverConstructor(root)
	}, skipGdrive)
}

// Test Committing a FileWriter without having called Write
func TestCommitEmpty(t *testing.T) {

	if skipGdrive() != "" {
		t.Skip(skipGdrive())
	}

	validRoot := "testdir"

	driver, err := gdriveDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}
	filename := "/testfile"
	ctx := ctx.Background()

	writer, err := driver.Writer(ctx, filename, false)
	defer driver.Delete(ctx, filename)
	if err != nil {
		t.Fatalf("driver.Writer: unexpected error: %v", err)
	}
	err = writer.Commit()
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

	if skipGdrive() != "" {
		t.Skip(skipGdrive())
	}

	validRoot := "testdir"
	driver, err := gdriveDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	filename := "/testfile"
	ctx := ctx.Background()
	defaultChunkSize := 1000
	contents := make([]byte, defaultChunkSize)
	writer, err := driver.Writer(ctx, filename, false)
	defer driver.Delete(ctx, filename)
	if err != nil {
		t.Fatalf("driver.Writer: unexpected error: %v", err)
	}
	_, err = writer.Write(contents)
	if err != nil {
		t.Fatalf("writer.Write: unexpected error: %v", err)
	}
	err = writer.Commit()
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

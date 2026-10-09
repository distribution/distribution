package gcs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/distribution/distribution/v3/internal/dcontext"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

var (
	gcsDriverConstructor func(rootDirectory string) (storagedriver.StorageDriver, error)
	skipCheck            func(tb testing.TB)
)

func init() {
	bucket := os.Getenv("REGISTRY_STORAGE_GCS_BUCKET")
	credentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	// Skip GCS storage driver tests if environment variable parameters are not provided
	skipCheck = func(tb testing.TB) {
		tb.Helper()

		if bucket == "" || credentials == "" {
			tb.Skip("The following environment variables must be set to enable these tests: REGISTRY_STORAGE_GCS_BUCKET, GOOGLE_APPLICATION_CREDENTIALS")
		}
	}

	gcsDriverConstructor = func(rootDirectory string) (storagedriver.StorageDriver, error) {
		jsonKey, err := os.ReadFile(credentials)
		if err != nil {
			panic(fmt.Sprintf("Error reading JSON key : %v", err))
		}

		var ts oauth2.TokenSource
		var email string
		var privateKey []byte

		ts, err = google.DefaultTokenSource(dcontext.Background(), storage.ScopeFullControl)
		if err != nil {
			// Assume that the file contents are within the environment variable since it exists
			// but does not contain a valid file path
			jwtConfig, err := google.JWTConfigFromJSON(jsonKey, storage.ScopeFullControl)
			if err != nil {
				panic(fmt.Sprintf("Error reading JWT config : %s", err))
			}
			email = jwtConfig.Email
			privateKey = jwtConfig.PrivateKey
			if len(privateKey) == 0 {
				panic("Error reading JWT config : missing private_key property")
			}
			if email == "" {
				panic("Error reading JWT config : missing client_email property")
			}
			ts = jwtConfig.TokenSource(dcontext.Background())
		}

		gcs, err := storage.NewClient(dcontext.Background(), option.WithTokenSource(ts))
		if err != nil {
			panic(fmt.Sprintf("Error initializing gcs client : %v", err))
		}

		parameters := driverParameters{
			bucket:         bucket,
			rootDirectory:  rootDirectory,
			email:          email,
			privateKey:     privateKey,
			client:         oauth2.NewClient(dcontext.Background(), ts),
			chunkSize:      defaultChunkSize,
			gcs:            gcs,
			maxConcurrency: 8,
		}

		return New(context.Background(), parameters)
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

// TestWalk exercises the GCS driver's Walk implementation, ported from the
// equivalent S3 driver conformance test (registry/storage/driver/s3-aws/s3_test.go)
// since it wasn't previously covered here. This is the test that should have
// existed before Walk silently fell back to a Stat-per-entry implementation
// that's fine for a handful of files but takes minutes against a tag
// directory with thousands of entries.
func TestWalk(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()

	drvr, err := gcsDriverConstructor(rootDir)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := dcontext.Background()

	fileset := []string{
		"/file1",
		"/folder1-suffix/file1",
		"/folder1/file1",
		"/folder2/file1",
		"/folder3/subfolder1/subfolder1/file1",
		"/folder3/subfolder2/subfolder1/file1",
		"/folder4/file1",
	}

	created := make([]string, 0, len(fileset))
	for _, p := range fileset {
		if err := drvr.PutContent(ctx, p, []byte("content "+p)); err != nil {
			t.Fatalf("unable to create file %s: %s", p, err)
		}
		created = append(created, p)
	}

	defer func() {
		var lastErr error
		for _, p := range created {
			if err := drvr.Delete(ctx, p); err != nil {
				lastErr = err
			}
		}
		if lastErr != nil {
			t.Fatalf("cleanup failed: %s", lastErr)
		}
	}()

	noopFn := func(fileInfo storagedriver.FileInfo) error { return nil }

	tcs := []struct {
		name     string
		fn       storagedriver.WalkFn
		from     string
		options  []func(*storagedriver.WalkOptions)
		expected []string
		err      bool
	}{
		{
			name: "walk all",
			fn:   noopFn,
			expected: []string{
				"/file1",
				"/folder1-suffix",
				"/folder1-suffix/file1",
				"/folder1",
				"/folder1/file1",
				"/folder2",
				"/folder2/file1",
				"/folder3",
				"/folder3/subfolder1",
				"/folder3/subfolder1/subfolder1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
				"/folder4",
				"/folder4/file1",
			},
		},
		{
			name: "skip directory",
			fn: func(fileInfo storagedriver.FileInfo) error {
				if fileInfo.Path() == "/folder3" {
					return storagedriver.ErrSkipDir
				}
				if strings.Contains(fileInfo.Path(), "/folder3") {
					t.Fatalf("skipped dir %s and should not walk %s", "/folder3", fileInfo.Path())
				}
				return nil
			},
			expected: []string{
				"/file1",
				"/folder1-suffix",
				"/folder1-suffix/file1",
				"/folder1",
				"/folder1/file1",
				"/folder2",
				"/folder2/file1",
				"/folder3",
				// folder3 contents skipped
				"/folder4",
				"/folder4/file1",
			},
		},
		{
			name: "start late without from",
			fn:   noopFn,
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3/subfolder1/subfolder1/file1"),
			},
			expected: []string{
				"/folder3",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
				"/folder4",
				"/folder4/file1",
			},
		},
		{
			name: "start late with from",
			fn:   noopFn,
			from: "/folder3",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3/subfolder1/subfolder1/file1"),
			},
			expected: []string{
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
			},
		},
		{
			name: "start after from",
			fn:   noopFn,
			from: "/folder1",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder2"),
			},
			expected: []string{},
		},
		{
			name: "start matches from",
			fn:   noopFn,
			from: "/folder3",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3"),
			},
			expected: []string{
				"/folder3/subfolder1",
				"/folder3/subfolder1/subfolder1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
			},
		},
		{
			name: "start doesn't exist",
			fn:   noopFn,
			from: "/folder3",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3/notafolder/notafile"),
			},
			expected: []string{
				"/folder3/subfolder1",
				"/folder3/subfolder1/subfolder1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
			},
		},
		{
			name: "stop early",
			fn: func(fileInfo storagedriver.FileInfo) error {
				if fileInfo.Path() == "/folder1/file1" {
					return storagedriver.ErrFilledBuffer
				}
				return nil
			},
			expected: []string{
				"/file1",
				"/folder1-suffix",
				"/folder1-suffix/file1",
				"/folder1",
				"/folder1/file1",
				// stop early
			},
		},
		{
			name: "error",
			fn: func(fileInfo storagedriver.FileInfo) error {
				return errors.New("foo")
			},
			expected: []string{
				"/file1",
			},
			err: true,
		},
		{
			name: "from folder",
			fn:   noopFn,
			expected: []string{
				"/folder1/file1",
			},
			from: "/folder1",
		},
	}

	for _, tc := range tcs {
		var walked []string
		if tc.from == "" {
			tc.from = "/"
		}
		t.Run(tc.name, func(t *testing.T) {
			err := drvr.Walk(ctx, tc.from, func(fileInfo storagedriver.FileInfo) error {
				walked = append(walked, fileInfo.Path())
				return tc.fn(fileInfo)
			}, tc.options...)
			if tc.err && err == nil {
				t.Fatal("expected err")
			}
			if !tc.err && err != nil {
				t.Fatal(err)
			}
			compareWalked(t, tc.expected, walked)
		})
	}
}

func TestIsSubpath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		parent   string
		expected bool
	}{
		{
			name:     "empty parent",
			path:     "/folder1/file1",
			parent:   "",
			expected: false,
		},
		{
			name:     "same path",
			path:     "/folder1",
			parent:   "/folder1",
			expected: true,
		},
		{
			name:     "descendant path",
			path:     "/folder1/file1",
			parent:   "/folder1",
			expected: true,
		},
		{
			name:     "sibling with lexical prefix",
			path:     "/folder1-suffix/file1",
			parent:   "/folder1",
			expected: false,
		},
		{
			name:     "root parent",
			path:     "/folder1/file1",
			parent:   "/",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := isSubpath(tt.path, tt.parent)
			if actual != tt.expected {
				t.Fatalf("isSubpath(%q, %q) = %t, want %t", tt.path, tt.parent, actual, tt.expected)
			}
		})
	}
}

func compareWalked(t *testing.T, expected, walked []string) {
	t.Helper()
	if len(walked) != len(expected) {
		t.Fatalf("mismatched number of fileInfo walked %d expected %d; walked %s; expected %s", len(walked), len(expected), walked, expected)
	}
	for i := range walked {
		if walked[i] != expected[i] {
			t.Fatalf("walked in unexpected order: expected %s; walked %s", expected, walked)
		}
	}
}

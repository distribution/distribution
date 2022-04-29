package storj

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var storjDriverConstructor func() (*Driver, error)

var skipStorj func() string

func init() {
	accessGrant := os.Getenv("STORJ_ACCESS_GRANT")
	bucket := os.Getenv("STORJ_BUCKET")

	storjDriverConstructor = func() (*Driver, error) {
		parameters := DriverParameters{
			accessGrant,
			bucket,
		}

		return New(parameters)
	}

	// Skip Storj storage driver tests if environment variable parameters are not provided
	skipStorj = func() string {
		if accessGrant == "" || bucket == "" {
			return "Must set STORJ_ACCESS_GRANT and STORJ_BUCKET to run Storj tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return storjDriverConstructor()
	}, skipStorj)
}

func TestWalk(t *testing.T) {
	if skipStorj() != "" {
		t.Skip(skipStorj())
	}

	driver, err := storjDriverConstructor()
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	var fileset = []string{
		"/file1",
		"/folder1/file1",
		"/folder2/file1",
		"/folder3/subfolder1/subfolder1/file1",
		"/folder3/subfolder2/subfolder1/file1",
		"/folder4/file1",
	}

	// create file structure matching fileset above
	var created []string
	for _, path := range fileset {
		err := driver.PutContent(context.Background(), path, []byte("content "+path))
		if err != nil {
			fmt.Printf("unable to create file %s: %s\n", path, err)
			continue
		}
		created = append(created, path)
	}

	// cleanup
	defer func() {
		var lastErr error
		for _, path := range created {
			err := driver.Delete(context.Background(), path)
			if err != nil {
				_ = fmt.Errorf("cleanup failed for path %s: %s", path, err)
				lastErr = err
			}
		}
		if lastErr != nil {
			t.Fatalf("cleanup failed: %s", err)
		}
	}()

	tcs := []struct {
		name     string
		fn       storagedriver.WalkFn
		from     string
		expected []string
		err      bool
	}{
		{
			name: "walk all",
			fn:   func(fileInfo storagedriver.FileInfo) error { return nil },
			expected: []string{
				"/file1",
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
				"/folder1",
				"/folder1/file1",
				"/folder2",
				"/folder2/file1",
				"/folder3",
				// folder 3 contents skipped
				"/folder4",
				"/folder4/file1",
			},
		},
		{
			name: "stop early",
			fn: func(fileInfo storagedriver.FileInfo) error {
				if fileInfo.Path() == "/folder1/file1" {
					return storagedriver.ErrSkipDir
				}
				return nil
			},
			expected: []string{
				"/folder1",
				"/folder1/file1",
				// stop early
			},
			err: false,
		},
		{
			name: "error",
			fn: func(fileInfo storagedriver.FileInfo) error {
				return errors.New("foo")
			},
			expected: []string{
				"/folder1",
			},
			err: true,
		},
		{
			name: "from folder",
			fn:   func(fileInfo storagedriver.FileInfo) error { return nil },
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
			err := driver.Walk(context.Background(), tc.from, func(fileInfo storagedriver.FileInfo) error {
				walked = append(walked, fileInfo.Path())
				return tc.fn(fileInfo)
			})
			if tc.err && err == nil {
				t.Fatalf("expected err")
			}
			if !tc.err && err != nil {
				t.Fatalf(err.Error())
			}
			compareWalked(t, tc.expected, walked)
		})
	}
}

func compareWalked(t *testing.T, expected, walked []string) {
	sort.Strings(expected)
	sort.Strings(walked)

	if len(walked) != len(expected) {
		t.Fatalf("Mismatch number of fileInfo walked %d expected %d; walked %s; expected %s;", len(walked), len(expected), walked, expected)
	}
	for i := range walked {
		if walked[i] != expected[i] {
			t.Fatalf("walked in unexpected order: expected %s; walked %s", expected, walked)
		}
	}
}

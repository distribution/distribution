package conformance

import (
	"context"
	"errors"
	"fmt"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"strings"
	"testing"
)

var fileset = map[string][]string{
	"/":        {"/file1", "/folder1", "/folder2"},
	"/folder1": {"/folder1/file1"},
	"/folder2": {"/folder2/file1"},
}

type walkConformance struct {
	driver  storagedriver.StorageDriver
	created []string
}

func (wc *walkConformance) init() error {
	for _, paths := range fileset {
		for _, path := range paths {
			if _, isDir := fileset[path]; isDir {
				continue // skip directories
			}
			err := wc.driver.PutContent(context.Background(), path, []byte("content "+path))
			if err != nil {
				return errors.New(fmt.Sprintf("unable to create file %s: %s", path, err))
			}
			wc.created = append(wc.created, path)
		}
	}
	return nil
}

func (wc *walkConformance) cleanup() error {
	var lastError error
	for _, path := range wc.created {
		err := wc.driver.Delete(context.Background(), path)
		if err != nil {
			_ = fmt.Errorf("cleanup failed for path %s: %s", path, err)
			lastError = err
		}
	}
	return lastError
}

func (wc *walkConformance) isDir(path string) bool {
	_, isDir := fileset[path]
	return isDir
}

func Run(driver storagedriver.StorageDriver, t *testing.T) error {
	wc := walkConformance{driver: driver}
	defer func() {
		err := wc.cleanup()
		if err != nil {
			t.Fatalf("cleanup failed: %s", err)
		}
	}()
	err := wc.init()
	if err != nil {
		return err
	}

	noopFn := func(fileInfo storagedriver.FileInfo) error { return nil }

	tcs := []struct {
		name     string
		fn       storagedriver.WalkFn
		from     string
		expected []string
		err      bool
	}{
		{
			name: "walk all",
			fn:   noopFn,
			expected: []string{
				"/",
				"/file1",
				"/folder1",
				"/folder1/file1",
				"/folder2",
				"/folder2/file1",
			},
		},
		{
			name: "skip directory",
			fn: func(fileInfo storagedriver.FileInfo) error {
				if fileInfo.Path() == "/folder1" {
					return storagedriver.ErrSkipDir
				}
				if strings.Contains(fileInfo.Path(), "/folder1") {
					t.Fatalf("skipped dir %s and should not walk %s", "/folder1", fileInfo.Path())
				}
				return nil
			},
			expected: []string{
				"/",
				"/file1",
				"/folder1", // return ErrSkipDir, skip anything under /folder1
				// skip /folder1/file1
				"/folder2",
				"/folder2/file1",
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
				"/",
				"/file1",
				"/folder1",
				"/folder1/file1",
				// stop early
			},
		},
		{
			name: "from folder",
			fn:   noopFn,
			expected: []string{
				"/folder1",
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
			err := wc.driver.Walk(context.Background(), tc.from, func(fileInfo storagedriver.FileInfo) error {
				walked = append(walked, fileInfo.Path())
				if fileInfo.IsDir() != wc.isDir(fileInfo.Path()) {
					t.Fatalf("fileInfo isDir not matching file system: expected %t actual %t", wc.isDir(fileInfo.Path()), fileInfo.IsDir())
				}
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
	return nil
}

func compareWalked(t *testing.T, expected, walked []string) {
	if len(walked) != len(expected) {
		t.Fatalf("Mismatch number of fileInfo walked %d expected %d; walked %s; expected %s;", len(walked), len(expected), walked, expected)
	}
	for i := range walked {
		if walked[i] != expected[i] {
			t.Fatalf("expected walked to come in order expected: walked %s", walked)
		}
	}
}

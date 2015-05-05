package storage

import (
	"fmt"
	"testing"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/inmemory"
)

func testFS(t *testing.T) (driver.StorageDriver, map[string]string, context.Context) {
	d := inmemory.New()
	c := []byte("")
	ctx := context.Background()
	if err := d.PutContent(ctx, "/a/b/c/d", c); err != nil {
		t.Fatalf("Unable to put to inmemory fs")
	}
	if err := d.PutContent(ctx, "/a/b/c/e", c); err != nil {
		t.Fatalf("Unable to put to inmemory fs")
	}

	expected := map[string]string{
		"/a":       "dir",
		"/a/b":     "dir",
		"/a/b/c":   "dir",
		"/a/b/c/d": "file",
		"/a/b/c/e": "file",
	}

	return d, expected, ctx
}

func TestWalkErrors(t *testing.T) {
	d, expected, ctx := testFS(t)
	fileCount := len(expected)
	err := Walk(ctx, d, "", func(fileInfo driver.FileInfo) error {
		return nil
	})
	if err == nil {
		t.Error("Expected invalid root err")
	}

	err = Walk(ctx, d, "/", func(fileInfo driver.FileInfo) error {
		// error on the 2nd file
		if fileInfo.Path() == "/a/b" {
			return fmt.Errorf("Early termination")
		}
		delete(expected, fileInfo.Path())
		return nil
	})
	if len(expected) != fileCount-1 {
		t.Error("Walk failed to terminate with error")
	}
	if err != nil {
		t.Error(err.Error())
	}

	err = Walk(ctx, d, "/nonexistant", func(fileInfo driver.FileInfo) error {
		return nil
	})
	if err == nil {
		t.Errorf("Expected missing file err")
	}

}

func TestWalk(t *testing.T) {
	d, expected, ctx := testFS(t)
	err := Walk(ctx, d, "/", func(fileInfo driver.FileInfo) error {
		filePath := fileInfo.Path()
		filetype, ok := expected[filePath]
		if !ok {
			t.Fatalf("Unexpected file in walk: %q", filePath)
		}

		if fileInfo.IsDir() {
			if filetype != "dir" {
				t.Errorf("Unexpected file type: %q", filePath)
			}
		} else {
			if filetype != "file" {
				t.Errorf("Unexpected file type: %q", filePath)
			}
		}
		delete(expected, filePath)
		return nil
	})
	if len(expected) > 0 {
		t.Errorf("Missed files in walk: %q", expected)
	}
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func TestWalkSkipDir(t *testing.T) {
	d, expected, ctx := testFS(t)
	err := Walk(ctx, d, "/", func(fileInfo driver.FileInfo) error {
		filePath := fileInfo.Path()
		if filePath == "/a/b" {
			// skip processing /a/b/c and /a/b/c/d
			return ErrSkipDir
		}
		delete(expected, filePath)
		return nil
	})
	if err != nil {
		t.Fatalf(err.Error())
	}
	if _, ok := expected["/a/b/c"]; !ok {
		t.Errorf("/a/b/c not skipped")
	}
	if _, ok := expected["/a/b/c/d"]; !ok {
		t.Errorf("/a/b/c/d not skipped")
	}
	if _, ok := expected["/a/b/c/e"]; !ok {
		t.Errorf("/a/b/c/e not skipped")
	}

}

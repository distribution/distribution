package driver

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type changingFileSystem struct {
	StorageDriver
	fileset   []string
	keptFiles map[string]bool
}

func (cfs *changingFileSystem) List(_ context.Context, _ string) ([]string, error) {
	return cfs.fileset, nil
}
func (cfs *changingFileSystem) Stat(_ context.Context, path string) (FileInfo, error) {
	kept, ok := cfs.keptFiles[path]
	if ok && kept {
		return &FileInfoInternal{
			FileInfoFields: FileInfoFields{
				Path: path,
			},
		}, nil
	}
	return nil, PathNotFoundError{}
}

type fileSystem struct {
	StorageDriver
	// maps folder to list results
	fileset map[string][]string
}

func (cfs *fileSystem) List(_ context.Context, path string) ([]string, error) {
	return cfs.fileset[path], nil
}

func (cfs *fileSystem) Stat(_ context.Context, path string) (FileInfo, error) {
	_, isDir := cfs.fileset[path]
	return &FileInfoInternal{
		FileInfoFields: FileInfoFields{
			Path:  path,
			IsDir: isDir,
			Size:  int64(len(path)),
		},
	}, nil
}
func (cfs *fileSystem) isDir(path string) bool {
	_, isDir := cfs.fileset[path]
	return isDir
}

func TestWalkFileRemoved(t *testing.T) {
	d := &changingFileSystem{
		fileset: []string{"zoidberg", "bender"},
		keptFiles: map[string]bool{
			"zoidberg": true,
		},
	}
	infos := []FileInfo{}
	err := WalkFallback(context.Background(), d, "", func(fileInfo FileInfo) error {
		infos = append(infos, fileInfo)
		return nil
	})
	if len(infos) != 1 || infos[0].Path() != "zoidberg" {
		t.Errorf(fmt.Sprintf("unexpected path set during walk: %s", infos))
	}
	if err != nil {
		t.Fatalf(err.Error())
	}
}

func TestWalkFallback(t *testing.T) {
	d := &fileSystem{
		fileset: map[string][]string{
			"/":        {"/file1", "/folder1", "/folder2"},
			"/folder1": {"/folder1/file1"},
			"/folder2": {"/folder2/file1"},
		},
	}
	noopFn := func(fileInfo FileInfo) error { return nil }

	tcs := []struct {
		name     string
		fn       WalkFn
		from     string
		expected []string
		err      bool
	}{
		{
			name: "walk all",
			fn:   noopFn,
			expected: []string{
				"/file1",
				"/folder1",
				"/folder1/file1",
				"/folder2",
				"/folder2/file1",
			},
		},
		{
			name: "skip directory",
			fn: func(fileInfo FileInfo) error {
				if fileInfo.Path() == "/folder1" {
					return ErrSkipDir
				}
				if strings.Contains(fileInfo.Path(), "/folder1") {
					t.Fatalf("skipped dir %s and should not walk %s", "/folder1", fileInfo.Path())
				}
				return nil
			},
			expected: []string{
				"/file1",
				"/folder1", // return ErrSkipDir, skip anything under /folder1
				// skip /folder1/file1
				"/folder2",
				"/folder2/file1",
			},
		},
		{
			name: "stop early",
			fn: func(fileInfo FileInfo) error {
				if fileInfo.Path() == "/folder1/file1" {
					return ErrSkipDir
				}
				return nil
			},
			expected: []string{
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
			err := WalkFallback(context.Background(), d, tc.from, func(fileInfo FileInfo) error {
				walked = append(walked, fileInfo.Path())
				if fileInfo.IsDir() != d.isDir(fileInfo.Path()) {
					t.Fatalf("fileInfo isDir not matching file system: expected %t actual %t", d.isDir(fileInfo.Path()), fileInfo.IsDir())
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

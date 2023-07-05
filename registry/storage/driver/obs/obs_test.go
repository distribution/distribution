package obs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"

	dcontext "github.com/distribution/distribution/v3/context"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var obsDriverConstructor func(rootDirectory string, storageClass obs.StorageClassType) (*Driver, error)

var skipOBS func() string

func init() {
	var (
		accessKey = os.Getenv("HUAWEICLOUD_ACCESS_KEY")
		secretKey = os.Getenv("HUAWEICLOUD_SECRET_KEY")
		bucket    = os.Getenv("OBS_BUCKET")
		endpoint  = os.Getenv("OBS_ENDPOINT")
		encrypt   = os.Getenv("OBS_ENCRYPT")
		keyID     = os.Getenv("OBS_KEY_ID")
		objectACL = os.Getenv("OBS_OBJECT_ACL")
	)

	root, err := os.MkdirTemp("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	obsDriverConstructor = func(rootDirectory string, storageClass obs.StorageClassType) (*Driver, error) {
		encryptBool := false
		if encrypt != "" {
			encryptBool, err = strconv.ParseBool(encrypt)
			if err != nil {
				return nil, err
			}
		}

		parameters := DriverParameters{
			AccessKey:                   accessKey,
			SecretKey:                   secretKey,
			Bucket:                      bucket,
			Endpoint:                    endpoint,
			Encrypt:                     encryptBool,
			EncryptionKeyID:             keyID,
			ChunkSize:                   minChunkSize,
			MultipartCopyThresholdSize:  defaultMultipartCopyThresholdSize,
			MultipartCopyMaxConcurrency: defaultMultipartCopyMaxConcurrency,
			MultipartCopyChunkSize:      defaultMultipartCopyChunkSize,
			RootDirectory:               rootDirectory,
			StorageClass:                storageClass,
			ObjectACL:                   obs.AclType(objectACL),
		}

		return New(parameters)
	}

	// Skip OBS storage driver tests if environment variable parameters are not provided
	skipOBS = func() string {
		if accessKey == "" || secretKey == "" || endpoint == "" || bucket == "" {
			return "Must set HUAWEICLOUD_ACCESS_KEY, HUAWEICLOUD_SECRET_KEY, OBS_ENDPOINT, OBS_BUCKET, and OBS_ENCRYPT to run OBS tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return obsDriverConstructor(root, obs.StorageClassStandard)
	}, skipOBS)
}

func TestEmptyRootList(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	validRoot := t.TempDir()
	rootedDriver, err := obsDriverConstructor(validRoot, obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := obsDriverConstructor("", obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := obsDriverConstructor("/", obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating slash root driver: %v", err)
	}

	filename := "/test"
	contents := []byte("contents")
	ctx := context.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer rootedDriver.Delete(ctx, filename)

	keys, _ := emptyRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}

	keys, _ = slashRootDriver.List(ctx, "/")
	for _, p := range keys {
		if !storagedriver.PathRegexp.MatchString(p) {
			t.Fatalf("unexpected string in path: %q != %q", p, storagedriver.PathRegexp)
		}
	}
}

// TestWalkEmptySubDirectory assures we list an empty sub directory only once when walking
// through its parent directory.
func TestWalkEmptySubDirectory(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	drv, err := obsDriverConstructor("", obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	// create an empty sub directory.
	obsdriver := drv.StorageDriver.(*driver)
	if err := obsdriver.PutContent(context.Background(), "/testdir/emptydir/", []byte("")); err != nil {
		t.Fatalf("error creating empty directory: %s", err)
	}

	bucketFiles := []string{}
	obsdriver.Walk(context.Background(), "/testdir", func(fileInfo storagedriver.FileInfo) error {
		bucketFiles = append(bucketFiles, fileInfo.Path())
		return nil
	})

	expected := []string{"/testdir/emptydir"}
	if !reflect.DeepEqual(bucketFiles, expected) {
		t.Errorf("expecting files %+v, found %+v instead", expected, bucketFiles)
	}
}

func TestDelete(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	rootDir := t.TempDir()

	drv, err := obsDriverConstructor(rootDir, obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	type errFn func(error) bool
	type testCase struct {
		name     string
		delete   string
		expected []string
		// error validation function
		err errFn
	}

	errPathNotFound := func(err error) bool {
		if err == nil {
			return false
		}
		switch err.(type) {
		case storagedriver.PathNotFoundError:
			return true
		}
		return false
	}
	errInvalidPath := func(err error) bool {
		if err == nil {
			return false
		}
		switch err.(type) {
		case storagedriver.InvalidPathError:
			return true
		}
		return false
	}

	objs := []string{
		"/file1",
		"/file1-2",
		"/file1/2",
		"/folder1/file1",
		"/folder2/file1",
		"/folder3/file1",
		"/folder3/subfolder1/subfolder1/file1",
		"/folder3/subfolder2/subfolder1/file1",
		"/folder4/file1",
		"/folder1-v2/file1",
		"/folder1-v2/subfolder1/file1",
	}

	tcs := []testCase{
		{
			// special case where a given path is a file and has subpaths
			name:   "delete file1",
			delete: "/file1",
			expected: []string{
				"/file1",
				"/file1/2",
			},
		},
		{
			name:   "delete folder1",
			delete: "/folder1",
			expected: []string{
				"/folder1/file1",
			},
		},
		{
			name:   "delete folder2",
			delete: "/folder2",
			expected: []string{
				"/folder2/file1",
			},
		},
		{
			name:   "delete folder3",
			delete: "/folder3",
			expected: []string{
				"/folder3/file1",
				"/folder3/subfolder1/subfolder1/file1",
				"/folder3/subfolder2/subfolder1/file1",
			},
		},
		{
			name:     "delete path that doesn't exist",
			delete:   "/path/does/not/exist",
			expected: []string{},
			err:      errPathNotFound,
		},
		{
			name:     "delete path invalid: trailing slash",
			delete:   "/path/is/invalid/",
			expected: []string{},
			err:      errInvalidPath,
		},
		{
			name:     "delete path invalid: trailing special character",
			delete:   "/path/is/invalid*",
			expected: []string{},
			err:      errInvalidPath,
		},
	}

	// objects to skip auto-created test case
	skipCase := map[string]bool{
		// special case where deleting "/file1" also deletes "/file1/2" is tested explicitly
		"/file1": true,
	}
	// create a test case for each file
	for _, p := range objs {
		if skipCase[p] {
			continue
		}
		tcs = append(tcs, testCase{
			name:     fmt.Sprintf("delete path:'%s'", p),
			delete:   p,
			expected: []string{p},
		})
	}

	init := func() []string {
		// init file structure matching objs
		var created []string
		for _, p := range objs {
			err := drv.PutContent(context.Background(), p, []byte("content "+p))
			if err != nil {
				fmt.Printf("unable to init file %s: %s\n", p, err)
				continue
			}
			created = append(created, p)
		}
		return created
	}

	cleanup := func(objs []string) {
		var lastErr error
		for _, p := range objs {
			err := drv.Delete(context.Background(), p)
			if err != nil {
				switch err.(type) {
				case storagedriver.PathNotFoundError:
					continue
				}
				lastErr = err
			}
		}
		if lastErr != nil {
			t.Fatalf("cleanup failed: %s", lastErr)
		}
	}
	defer cleanup(objs)

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			objs := init()

			err := drv.Delete(context.Background(), tc.delete)

			if tc.err != nil {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !tc.err(err) {
					t.Fatalf("error does not match expected: %s", err)
				}
			}
			if tc.err == nil && err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			var issues []string

			// validate all files expected to be deleted are deleted
			// and all files not marked for deletion still remain
			expected := tc.expected
			isExpected := func(path string) bool {
				for _, epath := range expected {
					if epath == path {
						return true
					}
				}
				return false
			}
			for _, path := range objs {
				stat, err := drv.Stat(context.Background(), path)
				if err != nil {
					switch err.(type) {
					case storagedriver.PathNotFoundError:
						if !isExpected(path) {
							issues = append(issues, fmt.Sprintf("unexpected path was deleted: %s", path))
						}
						// path was deleted & was supposed to be
						continue
					}
					t.Fatalf("stat: %s", err)
				}
				if stat.IsDir() {
					// for special cases where an object path has subpaths (eg /file1)
					// once /file1 is deleted it's now a directory according to stat
					continue
				}
				if isExpected(path) {
					issues = append(issues, fmt.Sprintf("expected path was not deleted: %s", path))
				}
			}

			if len(issues) > 0 {
				t.Fatalf(strings.Join(issues, "; \n\t"))
			}
		})
	}
}

func TestWalk(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	rootDir := t.TempDir()

	drv, err := obsDriverConstructor(rootDir, obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	fileset := []string{
		"/file1",
		"/folder1/file1",
		"/folder2/file1",
		"/folder3/subfolder1/subfolder1/file1",
		"/folder3/subfolder2/subfolder1/file1",
		"/folder4/file1",
	}

	// create file structure matching fileset above
	var created []string
	for _, p := range fileset {
		err := drv.PutContent(context.Background(), p, []byte("content "+p))
		if err != nil {
			fmt.Printf("unable to create file %s: %s\n", p, err)
			continue
		}
		created = append(created, p)
	}

	// cleanup
	defer func() {
		var lastErr error
		for _, p := range created {
			err := drv.Delete(context.Background(), p)
			if err != nil {
				_ = fmt.Errorf("cleanup failed for path %s: %s", p, err)
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
				"/file1",
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
				"/file1",
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
			err := drv.Walk(context.Background(), tc.from, func(fileInfo storagedriver.FileInfo) error {
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
	if len(walked) != len(expected) {
		t.Fatalf("Mismatch number of fileInfo walked %d expected %d; walked %s; expected %s;", len(walked), len(expected), walked, expected)
	}
	for i := range walked {
		if walked[i] != expected[i] {
			t.Fatalf("walked in unexpected order: expected %s; walked %s", expected, walked)
		}
	}
}

func TestOverThousandBlobs(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	rootDir := t.TempDir()

	drv, err := obsDriverConstructor(rootDir, obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 1005; i++ {
		filename := "/thousandfiletest/file" + strconv.Itoa(i)
		contents := []byte("contents")
		err = drv.PutContent(ctx, filename, contents)
		if err != nil {
			t.Fatalf("unexpected error creating content: %v", err)
		}
	}

	// cant actually verify deletion because read-after-delete is inconsistent, but can ensure no errors
	err = drv.Delete(ctx, "/thousandfiletest")
	if err != nil {
		t.Fatalf("unexpected error deleting thousand files: %v", err)
	}
}

func TestMoveWithMultipartCopy(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	rootDir := t.TempDir()

	drv, err := obsDriverConstructor(rootDir, obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := context.Background()
	sourcePath := "/source"
	destPath := "/dest"

	defer drv.Delete(ctx, sourcePath)
	defer drv.Delete(ctx, destPath)

	// An object larger than d's MultipartCopyThresholdSize will cause d.Move() to perform a multipart copy.
	multipartCopyThresholdSize := drv.baseEmbed.Base.StorageDriver.(*driver).MultipartCopyThresholdSize
	contents := make([]byte, 2*multipartCopyThresholdSize)
	rand.Read(contents)

	err = drv.PutContent(ctx, sourcePath, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}

	err = drv.Move(ctx, sourcePath, destPath)
	if err != nil {
		t.Fatalf("unexpected error moving file: %v", err)
	}

	received, err := drv.GetContent(ctx, destPath)
	if err != nil {
		t.Fatalf("unexpected error getting content: %v", err)
	}
	if !bytes.Equal(contents, received) {
		t.Fatal("content differs")
	}

	_, err = drv.GetContent(ctx, sourcePath)
	switch err.(type) {
	case storagedriver.PathNotFoundError:
	default:
		t.Fatalf("unexpected error getting content: %v", err)
	}
}

func TestListObjects(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	rootDir := t.TempDir()

	drv, err := obsDriverConstructor(rootDir, obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := context.Background()
	n := 6
	prefix := "/test-list-objects-v2"
	var filePaths []string
	for i := 0; i < n; i++ {
		filePaths = append(filePaths, fmt.Sprintf("%s/%d", prefix, i))
	}
	for _, p := range filePaths {
		if err := drv.PutContent(ctx, p, []byte(p)); err != nil {
			t.Fatalf("unexpected error putting content: %v", err)
		}
	}

	info, err := drv.Stat(ctx, filePaths[0])
	if err != nil {
		t.Fatalf("unexpected error stating: %v", err)
	}

	if info.IsDir() || info.Size() != int64(len(filePaths[0])) || info.Path() != filePaths[0] {
		t.Fatal("unexcepted state info")
	}

	subDirPath := prefix + "/sub/0"
	if err := drv.PutContent(ctx, subDirPath, []byte(subDirPath)); err != nil {
		t.Fatalf("unexpected error putting content: %v", err)
	}

	subPaths := append(filePaths, path.Dir(subDirPath))

	result, err := drv.List(ctx, prefix)
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}

	sort.Strings(subPaths)
	sort.Strings(result)
	if !reflect.DeepEqual(subPaths, result) {
		t.Fatalf("unexpected list result")
	}

	var walkPaths []string
	if err := drv.Walk(ctx, prefix, func(fileInfo storagedriver.FileInfo) error {
		walkPaths = append(walkPaths, fileInfo.Path())
		if fileInfo.Path() == path.Dir(subDirPath) {
			if !fileInfo.IsDir() {
				t.Fatalf("unexpected walking file info")
			}
		} else {
			if fileInfo.IsDir() || fileInfo.Size() != int64(len(fileInfo.Path())) {
				t.Fatalf("unexpected walking file info")
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error walking: %v", err)
	}

	subPaths = append(subPaths, subDirPath)
	sort.Strings(walkPaths)
	sort.Strings(subPaths)
	if !reflect.DeepEqual(subPaths, walkPaths) {
		t.Fatalf("unexpected walking paths")
	}

	if err := drv.Delete(ctx, prefix); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
}

// Test Committing a FileWriter after having written exactly
// defaultChunksize bytes.
func TestCommit(t *testing.T) {
	if skipOBS() != "" {
		t.Skip(skipOBS())
	}

	rootDir := t.TempDir()

	drv, err := obsDriverConstructor(rootDir, obs.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	filename := "/test"
	ctx := dcontext.Background()

	contents := make([]byte, defaultChunkSize)
	writer, err := drv.Writer(ctx, filename, false)
	defer drv.Delete(ctx, filename)
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
	readContents, err := drv.GetContent(ctx, filename)
	if err != nil {
		t.Fatalf("driver.GetContent: unexpected error: %v", err)
	}
	if len(readContents) != len(contents) {
		t.Fatalf("len(driver.GetContent(..)): %d != %d", len(readContents), len(contents))
	}
}

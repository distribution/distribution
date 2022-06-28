package s3

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/check.v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/distribution/distribution/v3/context"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var s3DriverConstructor func(rootDirectory, storageClass string) (*Driver, error)
var skipS3 func() string

func init() {
	accessKey := os.Getenv("AWS_ACCESS_KEY")
	secretKey := os.Getenv("AWS_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	encrypt := os.Getenv("S3_ENCRYPT")
	keyID := os.Getenv("S3_KEY_ID")
	secure := os.Getenv("S3_SECURE")
	skipVerify := os.Getenv("S3_SKIP_VERIFY")
	v4Auth := os.Getenv("S3_V4_AUTH")
	region := os.Getenv("AWS_REGION")
	objectACL := os.Getenv("S3_OBJECT_ACL")
	root, err := ioutil.TempDir("", "driver-")
	regionEndpoint := os.Getenv("REGION_ENDPOINT")
	forcePathStyle := os.Getenv("AWS_S3_FORCE_PATH_STYLE")
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
	useDualStack := os.Getenv("S3_USE_DUALSTACK")
	combineSmallPart := os.Getenv("MULTIPART_COMBINE_SMALL_PART")
	accelerate := os.Getenv("S3_ACCELERATE")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	s3DriverConstructor = func(rootDirectory, storageClass string) (*Driver, error) {
		encryptBool := false
		if encrypt != "" {
			encryptBool, err = strconv.ParseBool(encrypt)
			if err != nil {
				return nil, err
			}
		}

		secureBool := true
		if secure != "" {
			secureBool, err = strconv.ParseBool(secure)
			if err != nil {
				return nil, err
			}
		}

		skipVerifyBool := false
		if skipVerify != "" {
			skipVerifyBool, err = strconv.ParseBool(skipVerify)
			if err != nil {
				return nil, err
			}
		}

		v4Bool := true
		if v4Auth != "" {
			v4Bool, err = strconv.ParseBool(v4Auth)
			if err != nil {
				return nil, err
			}
		}
		forcePathStyleBool := true
		if forcePathStyle != "" {
			forcePathStyleBool, err = strconv.ParseBool(forcePathStyle)
			if err != nil {
				return nil, err
			}
		}

		useDualStackBool := false
		if useDualStack != "" {
			useDualStackBool, err = strconv.ParseBool(useDualStack)
		}

		multipartCombineSmallPart := true
		if combineSmallPart != "" {
			multipartCombineSmallPart, err = strconv.ParseBool(combineSmallPart)
			if err != nil {
				return nil, err
			}
		}

		accelerateBool := true
		if accelerate != "" {
			accelerateBool, err = strconv.ParseBool(accelerate)
			if err != nil {
				return nil, err
			}
		}

		parameters := DriverParameters{
			accessKey,
			secretKey,
			bucket,
			region,
			regionEndpoint,
			forcePathStyleBool,
			encryptBool,
			keyID,
			secureBool,
			skipVerifyBool,
			v4Bool,
			minChunkSize,
			defaultMultipartCopyChunkSize,
			defaultMultipartCopyMaxConcurrency,
			defaultMultipartCopyThresholdSize,
			multipartCombineSmallPart,
			rootDirectory,
			storageClass,
			driverName + "-test",
			objectACL,
			sessionToken,
			useDualStackBool,
			accelerateBool,
		}

		return New(parameters)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipS3 = func() string {
		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			return "Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, AWS_REGION, S3_BUCKET, and S3_ENCRYPT to run S3 tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return s3DriverConstructor(root, s3.StorageClassStandard)
	}, skipS3)
}

func TestEmptyRootList(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	validRoot, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(validRoot)

	rootedDriver, err := s3DriverConstructor(validRoot, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := s3DriverConstructor("", s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := s3DriverConstructor("/", s3.StorageClassStandard)
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
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}
}

// TestWalkEmptySubDirectory assures we list an empty sub directory only once when walking
// through its parent directory.
func TestWalkEmptySubDirectory(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	drv, err := s3DriverConstructor("", s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	// create an empty sub directory.
	s3driver := drv.StorageDriver.(*driver)
	if _, err := s3driver.S3.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(os.Getenv("S3_BUCKET")),
		Key:    aws.String("/testdir/emptydir/"),
	}); err != nil {
		t.Fatalf("error creating empty directory: %s", err)
	}

	bucketFiles := []string{}
	s3driver.Walk(context.Background(), "/testdir", func(fileInfo storagedriver.FileInfo) error {
		bucketFiles = append(bucketFiles, fileInfo.Path())
		return nil
	})

	expected := []string{"/testdir/emptydir"}
	if !reflect.DeepEqual(bucketFiles, expected) {
		t.Errorf("expecting files %+v, found %+v instead", expected, bucketFiles)
	}
}

func TestStorageClass(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	contents := []byte("contents")
	ctx := context.Background()
	for _, storageClass := range s3StorageClasses {
		filename := "/test-" + storageClass
		s3Driver, err := s3DriverConstructor(rootDir, storageClass)
		if err != nil {
			t.Fatalf("unexpected error creating driver with storage class %v: %v", storageClass, err)
		}

		err = s3Driver.PutContent(ctx, filename, contents)
		if err != nil {
			t.Fatalf("unexpected error creating content with storage class %v: %v", storageClass, err)
		}
		defer s3Driver.Delete(ctx, filename)

		driverUnwrapped := s3Driver.Base.StorageDriver.(*driver)
		resp, err := driverUnwrapped.S3.GetObject(&s3.GetObjectInput{
			Bucket: aws.String(driverUnwrapped.Bucket),
			Key:    aws.String(driverUnwrapped.s3Path(filename)),
		})
		if err != nil {
			t.Fatalf("unexpected error retrieving file with storage class %v: %v", storageClass, err)
		}
		defer resp.Body.Close()
		// Amazon only populates this header value for non-standard storage classes
		if storageClass == s3.StorageClassStandard && resp.StorageClass != nil {
			t.Fatalf(
				"unexpected response storage class for file with storage class %v: %v",
				storageClass,
				*resp.StorageClass,
			)
		} else if storageClass != s3.StorageClassStandard && resp.StorageClass == nil {
			t.Fatalf(
				"unexpected response storage class for file with storage class %v: %v",
				storageClass,
				s3.StorageClassStandard,
			)
		} else if storageClass != s3.StorageClassStandard && storageClass != *resp.StorageClass {
			t.Fatalf(
				"unexpected response storage class for file with storage class %v: %v",
				storageClass,
				*resp.StorageClass,
			)
		}
	}
}

func TestDelete(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	driver, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
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

	var objs = []string{
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
	var skipCase = map[string]bool{
		// special case where deleting "/file1" also deletes "/file1/2" is tested explicitly
		"/file1": true,
	}
	// create a test case for each file
	for _, path := range objs {
		if skipCase[path] {
			continue
		}
		tcs = append(tcs, testCase{
			name:     fmt.Sprintf("delete path:'%s'", path),
			delete:   path,
			expected: []string{path},
		})
	}

	init := func() []string {
		// init file structure matching objs
		var created []string
		for _, path := range objs {
			err := driver.PutContent(context.Background(), path, []byte("content "+path))
			if err != nil {
				fmt.Printf("unable to init file %s: %s\n", path, err)
				continue
			}
			created = append(created, path)
		}
		return created
	}

	cleanup := func(objs []string) {
		var lastErr error
		for _, path := range objs {
			err := driver.Delete(context.Background(), path)
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

			err := driver.Delete(context.Background(), tc.delete)

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
				stat, err := driver.Stat(context.Background(), path)
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
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	driver, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
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

func TestOverThousandBlobs(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	standardDriver, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 1005; i++ {
		filename := "/thousandfiletest/file" + strconv.Itoa(i)
		contents := []byte("contents")
		err = standardDriver.PutContent(ctx, filename, contents)
		if err != nil {
			t.Fatalf("unexpected error creating content: %v", err)
		}
	}

	// cant actually verify deletion because read-after-delete is inconsistent, but can ensure no errors
	err = standardDriver.Delete(ctx, "/thousandfiletest")
	if err != nil {
		t.Fatalf("unexpected error deleting thousand files: %v", err)
	}
}

func TestMoveWithMultipartCopy(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	d, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := context.Background()
	sourcePath := "/source"
	destPath := "/dest"

	defer d.Delete(ctx, sourcePath)
	defer d.Delete(ctx, destPath)

	// An object larger than d's MultipartCopyThresholdSize will cause d.Move() to perform a multipart copy.
	multipartCopyThresholdSize := d.baseEmbed.Base.StorageDriver.(*driver).MultipartCopyThresholdSize
	contents := make([]byte, 2*multipartCopyThresholdSize)
	rand.Read(contents)

	err = d.PutContent(ctx, sourcePath, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}

	err = d.Move(ctx, sourcePath, destPath)
	if err != nil {
		t.Fatalf("unexpected error moving file: %v", err)
	}

	received, err := d.GetContent(ctx, destPath)
	if err != nil {
		t.Fatalf("unexpected error getting content: %v", err)
	}
	if !bytes.Equal(contents, received) {
		t.Fatal("content differs")
	}

	_, err = d.GetContent(ctx, sourcePath)
	switch err.(type) {
	case storagedriver.PathNotFoundError:
	default:
		t.Fatalf("unexpected error getting content: %v", err)
	}
}

func TestListObjectsV2(t *testing.T) {
	if skipS3() != "" {
		t.Skip(skipS3())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	d, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
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
	for _, path := range filePaths {
		if err := d.PutContent(ctx, path, []byte(path)); err != nil {
			t.Fatalf("unexpected error putting content: %v", err)
		}
	}

	info, err := d.Stat(ctx, filePaths[0])
	if err != nil {
		t.Fatalf("unexpected error stating: %v", err)
	}

	if info.IsDir() || info.Size() != int64(len(filePaths[0])) || info.Path() != filePaths[0] {
		t.Fatal("unexcepted state info")
	}

	subDirPath := prefix + "/sub/0"
	if err := d.PutContent(ctx, subDirPath, []byte(subDirPath)); err != nil {
		t.Fatalf("unexpected error putting content: %v", err)
	}

	subPaths := append(filePaths, path.Dir(subDirPath))

	result, err := d.List(ctx, prefix)
	if err != nil {
		t.Fatalf("unexpected error listing: %v", err)
	}

	sort.Strings(subPaths)
	sort.Strings(result)
	if !reflect.DeepEqual(subPaths, result) {
		t.Fatalf("unexpected list result")
	}

	var walkPaths []string
	if err := d.Walk(ctx, prefix, func(fileInfo storagedriver.FileInfo) error {
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

	if err := d.Delete(ctx, prefix); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
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

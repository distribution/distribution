package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
)

var (
	s3DriverConstructor func(rootDirectory, storageClass string) (*Driver, error)
	skipCheck           func(tb testing.TB)
)

func init() {
	var (
		accessKey      = os.Getenv("AWS_ACCESS_KEY")
		secretKey      = os.Getenv("AWS_SECRET_KEY")
		bucket         = os.Getenv("S3_BUCKET")
		encrypt        = os.Getenv("S3_ENCRYPT")
		keyID          = os.Getenv("S3_KEY_ID")
		secure         = os.Getenv("S3_SECURE")
		skipVerify     = os.Getenv("S3_SKIP_VERIFY")
		v4Auth         = os.Getenv("S3_V4_AUTH")
		region         = os.Getenv("AWS_REGION")
		objectACL      = os.Getenv("S3_OBJECT_ACL")
		regionEndpoint = os.Getenv("REGION_ENDPOINT")
		forcePathStyle = os.Getenv("AWS_S3_FORCE_PATH_STYLE")
		sessionToken   = os.Getenv("AWS_SESSION_TOKEN")
		useDualStack   = os.Getenv("S3_USE_DUALSTACK")
		accelerate     = os.Getenv("S3_ACCELERATE")
		logLevel       = os.Getenv("S3_LOGLEVEL")
	)

	var err error
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
			rootDirectory,
			storageClass,
			driverName + "-test",
			objectACL,
			sessionToken,
			useDualStackBool,
			accelerateBool,
			getS3LogLevelFromParam(logLevel),
		}

		return New(context.Background(), parameters)
	}

	// Skip S3 storage driver tests if environment variable parameters are not provided
	skipCheck = func(tb testing.TB) {
		tb.Helper()

		if accessKey == "" || secretKey == "" || region == "" || bucket == "" || encrypt == "" {
			tb.Skip("Must set AWS_ACCESS_KEY, AWS_SECRET_KEY, AWS_REGION, S3_BUCKET, and S3_ENCRYPT to run S3 tests")
		}
	}
}

func newDriverConstructor(tb testing.TB) testsuites.DriverConstructor {
	root := tb.TempDir()

	return func() (storagedriver.StorageDriver, error) {
		return s3DriverConstructor(root, s3.StorageClassStandard)
	}
}

func TestS3DriverSuite(t *testing.T) {
	skipCheck(t)
	testsuites.Driver(t, newDriverConstructor(t))
}

func BenchmarkS3DriverSuite(b *testing.B) {
	skipCheck(b)
	testsuites.BenchDriver(b, newDriverConstructor(b))
}

func TestEmptyRootList(t *testing.T) {
	skipCheck(t)

	validRoot := t.TempDir()
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
	ctx := dcontext.Background()
	err = rootedDriver.PutContent(ctx, filename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	// nolint:errcheck
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

func TestClientTransport(t *testing.T) {
	skipCheck(t)

	testCases := []struct {
		skipverify bool
	}{
		{true},
		{false},
	}

	for _, tc := range testCases {
		// NOTE(milosgajdos): we cannot simply reuse s3DriverConstructor
		// because s3DriverConstructor is initialized in init() using the process
		// env vars: we can not override S3_SKIP_VERIFY env var with t.Setenv
		params := map[string]interface{}{
			"region":     os.Getenv("AWS_REGION"),
			"bucket":     os.Getenv("S3_BUCKET"),
			"skipverify": tc.skipverify,
		}
		t.Run(fmt.Sprintf("SkipVerify %v", tc.skipverify), func(t *testing.T) {
			drv, err := FromParameters(context.TODO(), params)
			if err != nil {
				t.Fatalf("failed to create driver: %v", err)
			}

			s3drv := drv.baseEmbed.Base.StorageDriver.(*driver)
			if tc.skipverify {
				tr, ok := s3drv.S3.Client.Config.HTTPClient.Transport.(*http.Transport)
				if !ok {
					t.Fatal("unexpected driver transport")
				}
				if !tr.TLSClientConfig.InsecureSkipVerify {
					t.Errorf("unexpected TLS Config. Expected InsecureSkipVerify: %v, got %v",
						tc.skipverify,
						tr.TLSClientConfig.InsecureSkipVerify)
				}
				// make sure the proxy is always set
				if tr.Proxy == nil {
					t.Fatal("missing HTTP transport proxy config")
				}
				return
			}
			// if tc.skipverify is false we do not override the driver
			// HTTP client transport and leave it to the AWS SDK.
			if s3drv.S3.Client.Config.HTTPClient.Transport != nil {
				t.Errorf("unexpected S3 driver client transport")
			}
		})
	}
}

func TestStorageClass(t *testing.T) {
	skipCheck(t)

	rootDir := t.TempDir()
	contents := []byte("contents")
	ctx := dcontext.Background()

	// We don't need to test all the storage classes, just that its selectable.
	// The first 3 are common to AWS and MinIO, so use those.
	for _, storageClass := range s3StorageClasses[:3] {
		filename := "/test-" + storageClass
		s3Driver, err := s3DriverConstructor(rootDir, storageClass)
		if err != nil {
			t.Fatalf("unexpected error creating driver with storage class %v: %v", storageClass, err)
		}

		// Can only test outposts if using s3 outposts
		if storageClass == s3.StorageClassOutposts {
			continue
		}

		err = s3Driver.PutContent(ctx, filename, contents)
		if err != nil {
			t.Fatalf("unexpected error creating content with storage class %v: %v", storageClass, err)
		}
		// nolint:errcheck
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
		if storageClass == noStorageClass {
			// We haven't specified a storage class so we can't confirm what it is
		} else if storageClass == s3.StorageClassStandard && resp.StorageClass != nil {
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
	skipCheck(t)

	rootDir := t.TempDir()

	drvr, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
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

	// create a test case for each file
	for _, p := range objs {
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
			err := drvr.PutContent(dcontext.Background(), p, []byte("content "+p))
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
			err := drvr.Delete(dcontext.Background(), p)
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

			err := drvr.Delete(dcontext.Background(), tc.delete)

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
				stat, err := drvr.Stat(dcontext.Background(), path)
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
	skipCheck(t)

	rootDir := t.TempDir()

	drvr, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	fileset := []string{
		"/file1",
		"/folder1-suffix/file1",
		"/folder1/file1",
		"/folder2/file1",
		"/folder3/subfolder1/subfolder1/file1",
		"/folder3/subfolder2/subfolder1/file1",
		"/folder4/file1",
	}

	// create file structure matching fileset above
	created := make([]string, 0, len(fileset))
	for _, p := range fileset {
		err := drvr.PutContent(dcontext.Background(), p, []byte("content "+p))
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
			err := drvr.Delete(dcontext.Background(), p)
			if err != nil {
				_ = fmt.Errorf("cleanup failed for path %s: %s", p, err)
				lastErr = err
			}
		}
		if lastErr != nil {
			t.Fatalf("cleanup failed: %s", err)
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
				// folder 3 contents skipped
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
				// start late
				"/folder3",
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
				"/folder4",
				"/folder4/file1",
			},
			err: false,
		},
		{
			name: "start late with from",
			fn:   noopFn,
			from: "/folder3",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder3/subfolder1/subfolder1/file1"),
			},
			expected: []string{
				// start late
				"/folder3/subfolder2",
				"/folder3/subfolder2/subfolder1",
				"/folder3/subfolder2/subfolder1/file1",
			},
			err: false,
		},
		{
			name: "start after from",
			fn:   noopFn,
			from: "/folder1",
			options: []func(*storagedriver.WalkOptions){
				storagedriver.WithStartAfterHint("/folder2"),
			},
			expected: []string{},
			err:      false,
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
			err: false,
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
			err: false,
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
			err := drvr.Walk(dcontext.Background(), tc.from, func(fileInfo storagedriver.FileInfo) error {
				walked = append(walked, fileInfo.Path())
				return tc.fn(fileInfo)
			}, tc.options...)
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
	skipCheck(t)

	rootDir := t.TempDir()
	standardDriver, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver with standard storage: %v", err)
	}

	ctx := dcontext.Background()
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
	skipCheck(t)

	rootDir := t.TempDir()
	d, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := dcontext.Background()
	sourcePath := "/source"
	destPath := "/dest"

	// nolint:errcheck
	defer d.Delete(ctx, sourcePath)
	// nolint:errcheck
	defer d.Delete(ctx, destPath)

	// An object larger than d's MultipartCopyThresholdSize will cause d.Move() to perform a multipart copy.
	multipartCopyThresholdSize := d.baseEmbed.Base.StorageDriver.(*driver).MultipartCopyThresholdSize
	contents := make([]byte, 2*multipartCopyThresholdSize)
	if _, err := rand.Read(contents); err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}

	err = d.PutContent(ctx, sourcePath, contents)
	if err != nil {
		t.Fatalf("unexpected error writing content: %v", err)
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
	skipCheck(t)

	rootDir := t.TempDir()
	d, err := s3DriverConstructor(rootDir, s3.StorageClassStandard)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := dcontext.Background()
	n := 6
	prefix := "/test-list-objects-v2"
	var filePaths []string
	for i := 0; i < n; i++ {
		filePaths = append(filePaths, fmt.Sprintf("%s/%d", prefix, i))
	}
	for _, p := range filePaths {
		if err := d.PutContent(ctx, p, []byte(p)); err != nil {
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

package s3

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/check.v1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
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
	sessionToken := os.Getenv("AWS_SESSION_TOKEN")
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

		parameters := DriverParameters{
			nil,
			accessKey,
			secretKey,
			bucket,
			region,
			regionEndpoint,
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
			false,
			map[string]string{},
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

func TestStorageClass(t *testing.T) {
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

	rrDriver, err := s3DriverConstructor(rootDir, s3.StorageClassReducedRedundancy)
	if err != nil {
		t.Fatalf("unexpected error creating driver with reduced redundancy storage: %v", err)
	}

	if _, err = s3DriverConstructor(rootDir, noStorageClass); err != nil {
		t.Fatalf("unexpected error creating driver without storage class: %v", err)
	}

	standardFilename := "/test-standard"
	rrFilename := "/test-rr"
	contents := []byte("contents")
	ctx := context.Background()

	err = standardDriver.PutContent(ctx, standardFilename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer standardDriver.Delete(ctx, standardFilename)

	err = rrDriver.PutContent(ctx, rrFilename, contents)
	if err != nil {
		t.Fatalf("unexpected error creating content: %v", err)
	}
	defer rrDriver.Delete(ctx, rrFilename)

	standardDriverUnwrapped := standardDriver.Base.StorageDriver.(*driver)
	resp, err := standardDriverUnwrapped.S3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(standardDriverUnwrapped.Bucket),
		Key:    aws.String(standardDriverUnwrapped.s3Path(standardFilename)),
	})
	if err != nil {
		t.Fatalf("unexpected error retrieving standard storage file: %v", err)
	}
	defer resp.Body.Close()
	// Amazon only populates this header value for non-standard storage classes
	if resp.StorageClass != nil {
		t.Fatalf("unexpected storage class for standard file: %v", resp.StorageClass)
	}

	rrDriverUnwrapped := rrDriver.Base.StorageDriver.(*driver)
	resp, err = rrDriverUnwrapped.S3.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(rrDriverUnwrapped.Bucket),
		Key:    aws.String(rrDriverUnwrapped.s3Path(rrFilename)),
	})
	if err != nil {
		t.Fatalf("unexpected error retrieving reduced-redundancy storage file: %v", err)
	}
	defer resp.Body.Close()
	if resp.StorageClass == nil {
		t.Fatalf("unexpected storage class for reduced-redundancy file: %v", s3.StorageClassStandard)
	} else if *resp.StorageClass != s3.StorageClassReducedRedundancy {
		t.Fatalf("unexpected storage class for reduced-redundancy file: %v", *resp.StorageClass)
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

	var fileset = map[string][]string{
		"/":        {"/file1", "/folder1", "/folder2"},
		"/folder1": {"/folder1/file1"},
		"/folder2": {"/folder2/file1"},
	}
	isDir := func(path string) bool {
		_, isDir := fileset[path]
		return isDir
	}

	var created []string
	for _, paths := range fileset {
		for _, path := range paths {
			if _, isDir := fileset[path]; isDir {
				continue // skip directories
			}
			err := driver.PutContent(context.Background(), path, []byte("content "+path))
			if err != nil {
				fmt.Printf("unable to create file %s: %s\n", path, err)
			}
			created = append(created, path)
		}
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
				if fileInfo.IsDir() != isDir(fileInfo.Path()) {
					t.Fatalf("fileInfo isDir not matching file system: expected %t actual %t", isDir(fileInfo.Path()), fileInfo.IsDir())
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

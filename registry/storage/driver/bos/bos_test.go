// +build include_bos

package bos

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/baidubce/bce-sdk-go/bos/api"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var bosDriverConstructor func(rootDirectory string, storageClass string) (*Driver, error)

var skipCheck func() string

func init() {
	accessKey := os.Getenv("ACCESS_KEY_ID")
	secretKey := os.Getenv("SECRET_ACCESS_KEY")
	bucket := os.Getenv("BUCKET")
	region := os.Getenv("REGION")
	endpoint := os.Getenv("ENDPOINT")
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	bosDriverConstructor = func(rootDirectory string, storageClass string) (*Driver, error) {
		parameters := DriverParameters{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
			Bucket:          bucket,
			Region:          region,
			Endpoint:        endpoint,
			RootDirectory:   rootDirectory,
			StorageClass:    storageClass,
			ChunkSize:       defaultChunkSize,
		}

		return New(parameters)
	}

	// Skip BOS storage driver tests if environment variable parameters are not provided
	skipCheck = func() string {
		if accessKey == "" || secretKey == "" || bucket == "" || region == "" || endpoint == "" {
			return "Must set ACCESS_KEY_ID, SECRET_ACCESS_KEY, BUCKET, REGION, ENDPOINT"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return bosDriverConstructor(root, api.STORAGE_CLASS_STANDARD)
	}, skipCheck)
}

func TestEmptyRootList(t *testing.T) {
	if skipCheck() != "" {
		t.Skip(skipCheck())
	}

	validRoot, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(validRoot)

	rootedDriver, err := bosDriverConstructor(validRoot, api.STORAGE_CLASS_STANDARD)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := bosDriverConstructor("", api.STORAGE_CLASS_STANDARD)
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := bosDriverConstructor("/", api.STORAGE_CLASS_STANDARD)
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

	keys, err := emptyRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}

	keys, err = slashRootDriver.List(ctx, "/")
	for _, path := range keys {
		if !storagedriver.PathRegexp.MatchString(path) {
			t.Fatalf("unexpected string in path: %q != %q", path, storagedriver.PathRegexp)
		}
	}
}

var storageClassTestcases = []struct {
	storageClass string
	filename     string
}{
	{api.STORAGE_CLASS_STANDARD, "/standard"},
	{api.STORAGE_CLASS_STANDARD_IA, "/standard-ia"},
	{api.STORAGE_CLASS_COLD, "/cold"},
}

func TestStorageClass(t *testing.T) {
	if skipCheck() != "" {
		t.Skip(skipCheck())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	if _, err = bosDriverConstructor(rootDir, ""); err != nil {
		t.Fatalf("unexpected error creating driver without storage class: %v", err)
	}

	contents := []byte("contents")
	ctx := context.Background()

	for _, testcase := range storageClassTestcases {
		storageDriver, err := bosDriverConstructor(rootDir, testcase.storageClass)
		if err != nil {
			t.Fatalf("unexpected error creating driver with standard storage: %v", err)
		}

		err = storageDriver.PutContent(ctx, testcase.filename, contents)
		if err != nil {
			t.Fatalf("unexpected error creating content: %v", err)
		}
		defer storageDriver.Delete(ctx, testcase.filename)

		driverUnwrapped := storageDriver.Base.StorageDriver.(*driver)
		resp, err := driverUnwrapped.Client.GetObject(driverUnwrapped.Bucket,
			driverUnwrapped.bosPath(testcase.filename), nil)
		if err != nil {
			t.Fatalf("unexpected error retrieving file: %v", err)
		}
		defer resp.Body.Close()
		if resp.StorageClass != testcase.storageClass {
			t.Fatalf("unexpected storage class for standard file: %v", resp.StorageClass)
		}
	}
}

// it seems aws has a problem when delete per 1000 times, it's bug for bug testcase
func TestOverThousandBlobs(t *testing.T) {
	if skipCheck() != "" {
		t.Skip(skipCheck())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	standardDriver, err := bosDriverConstructor(rootDir, api.STORAGE_CLASS_STANDARD)
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
	if skipCheck() != "" {
		t.Skip(skipCheck())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	d, err := bosDriverConstructor(rootDir, api.STORAGE_CLASS_STANDARD)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := context.Background()
	sourcePath := "/source"
	destPath := "/dest"

	defer d.Delete(ctx, sourcePath)
	defer d.Delete(ctx, destPath)

	// BOS not has this threshold now, so just pat my head
	multipartCopyThresholdSize := defaultChunkSize * 8
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

func TestPutAndGet(t *testing.T) {
	if skipCheck() != "" {
		t.Skip(skipCheck())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	d, err := bosDriverConstructor(rootDir, api.STORAGE_CLASS_STANDARD)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := context.Background()
	destPath := "/dest"

	filesize := defaultChunkSize * 2
	contents := make([]byte, filesize)
	rand.Read(contents)

	err = d.PutContent(ctx, destPath, contents)
	if err != nil {
		t.Fatalf("PutContent fail, error: %v", err)
	}
	defer d.Delete(ctx, destPath)

	received, err := d.GetContent(ctx, destPath)
	if err != nil {
		t.Fatalf("GetContent fail, error: %v", err)
	}

	if !bytes.Equal(contents, received) {
		t.Fatalf("content differs")
	}
}

func TestWriterAndReader(t *testing.T) {
	if skipCheck() != "" {
		t.Skip(skipCheck())
	}

	rootDir, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(rootDir)

	d, err := bosDriverConstructor(rootDir, api.STORAGE_CLASS_STANDARD)
	if err != nil {
		t.Fatalf("unexpected error creating driver: %v", err)
	}

	ctx := context.Background()
	destPath := "/dest"

	filesize := defaultChunkSize * 4
	contents := make([]byte, filesize)
	rand.Read(contents)

	defer d.Delete(ctx, destPath)

	writer, err := d.Writer(ctx, destPath, false)
	if err != nil {
		t.Fatalf("Writer fail, error: %v", err)
	}
	defer writer.Close()

	nw, err := writer.Write(contents)
	t.Logf("Write size, %v, %v", nw, writer.Size())
	if err != nil {
		t.Fatalf("Write fail, error: %v", err)
	}
	err = writer.Commit()
	if err != nil {
		t.Fatalf("Commit fail, error: %v", err)
	}

	reader, err := d.Reader(ctx, destPath, 0)
	if err != nil {
		t.Fatalf("Reader fail, error: %v", err)
	}
	defer reader.Close()

	received, err := ioutil.ReadAll(reader)
	t.Logf("Read size: %v", len(received))
	if err != nil {
		t.Fatalf("ReadAll fail, error: %v", err)
	}

	if !bytes.Equal(contents, received) {
		t.Fatalf("content differs")
	}
}

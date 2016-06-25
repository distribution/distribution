package hdfsweb

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"strconv"
	"testing"

	ctx "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

var HdfsDriverConstructor func(rootDirectory string) (*Driver, error)
var skipHdfs func() string

func init() {
	nameNodeHost := os.Getenv("HDFS_NAME_NODE_HOST")
	nameNodePort := os.Getenv("HDFS_NAME_NODE_PORT")
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)
	HdfsDriverConstructor = func(rootDirectory string) (*Driver, error) {
		var (
			blockSize   = defaultBlockSize
			bufferSize  = defaultBufferSize
			replication = defaultReplication
		)
		if nameNodeHost == "" {
			return nil, fmt.Errorf("not provide HDFS_NAME_NODE_HOST")
		}

		if nameNodePort == "" {
			return nil, fmt.Errorf("not provide HDFS_NAME_NODE_HOST")
		}

		if v := os.Getenv("HDFS_BLOCK_SIZE"); v != "" {
			blockSize, err = strconv.ParseInt(v, 0, 64)
			if err != nil {
				return nil, err
			}
		}

		if v := os.Getenv("HDFS_BUFFER_SIZE"); v != "" {
			i, err := strconv.ParseInt(v, 0, 32)
			if err != nil {
				return nil, err
			}
			bufferSize = int32(i)
		}

		if v := os.Getenv("HDFS_REPLICATION"); v != "" {
			i, err := strconv.ParseInt(v, 0, 16)
			if err != nil {
				return nil, err
			}
			replication = int16(i)
		}

		userName := os.Getenv("HDFS_USER_NAME")
		if userName == "" {
			usr, err := user.Current()
			if err != nil {
				return nil, err
			}
			userName = usr.Username
		}

		return New(DriverParameters{
			NameNodeHost:  nameNodeHost,
			NameNodePort:  nameNodePort,
			RootDirectory: rootDirectory,
			UserName:      userName,
			BlockSize:     blockSize,
			BufferSize:    int32(bufferSize),
			Replication:   int16(replication),
		})
	}

	// Skip HDFS storage driver tests if environment variable parameters are not provided
	skipHdfs = func() string {
		if nameNodeHost == "" || nameNodePort == "" {
			return "Must set HDFS_NAME_NODE_HOST, HDFS_NAME_NODE_PORT and HDFS_ROOT_DIRECTORY to run HDFS tests"
		}
		return ""
	}

	testsuites.RegisterSuite(func() (storagedriver.StorageDriver, error) {
		return HdfsDriverConstructor(root)
	}, skipHdfs)
}

func TestEmptyRootList(t *testing.T) {
	if skipHdfs() != "" {
		t.Skip(skipHdfs())
	}

	validRoot, err := ioutil.TempDir("", "driver-")
	if err != nil {
		t.Fatalf("unexpected error creating temporary directory: %v", err)
	}
	defer os.Remove(validRoot)

	rootedDriver, err := HdfsDriverConstructor(validRoot)
	if err != nil {
		t.Fatalf("unexpected error creating rooted driver: %v", err)
	}

	emptyRootDriver, err := HdfsDriverConstructor("")
	if err != nil {
		t.Fatalf("unexpected error creating empty root driver: %v", err)
	}

	slashRootDriver, err := HdfsDriverConstructor("/")
	if err != nil {
		t.Fatalf("unexpected error creating slash root driver: %v", err)
	}

	filename := "/test"
	contents := []byte("contents")
	ctx := ctx.Background()
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

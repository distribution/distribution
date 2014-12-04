package testsuites

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/ipc"

	"gopkg.in/check.v1"
)

// Test hooks up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

// RegisterInProcessSuite registers an in-process storage driver test suite with
// the go test runner.
func RegisterInProcessSuite(driverConstructor DriverConstructor, skipCheck SkipCheck) {
	check.Suite(&DriverSuite{
		Constructor: driverConstructor,
		SkipCheck:   skipCheck,
	})
}

// RegisterIPCSuite registers a storage driver test suite which runs the named
// driver as a child process with the given parameters.
func RegisterIPCSuite(driverName string, ipcParams map[string]string, skipCheck SkipCheck) {
	suite := &DriverSuite{
		Constructor: func() (storagedriver.StorageDriver, error) {
			d, err := ipc.NewDriverClient(driverName, ipcParams)
			if err != nil {
				return nil, err
			}
			err = d.Start()
			if err != nil {
				return nil, err
			}
			return d, nil
		},
		SkipCheck: skipCheck,
	}
	suite.Teardown = func() error {
		if suite.StorageDriver == nil {
			return nil
		}

		driverClient := suite.StorageDriver.(*ipc.StorageDriverClient)
		return driverClient.Stop()
	}
	check.Suite(suite)
}

// SkipCheck is a function used to determine if a test suite should be skipped.
// If a SkipCheck returns a non-empty skip reason, the suite is skipped with
// the given reason.
type SkipCheck func() (reason string)

// NeverSkip is a default SkipCheck which never skips the suite.
var NeverSkip SkipCheck = func() string { return "" }

// DriverConstructor is a function which returns a new
// storagedriver.StorageDriver.
type DriverConstructor func() (storagedriver.StorageDriver, error)

// DriverTeardown is a function which cleans up a suite's
// storagedriver.StorageDriver.
type DriverTeardown func() error

// DriverSuite is a gocheck test suite designed to test a
// storagedriver.StorageDriver.
// The intended way to create a DriverSuite is with RegisterInProcessSuite or
// RegisterIPCSuite.
type DriverSuite struct {
	Constructor DriverConstructor
	Teardown    DriverTeardown
	SkipCheck
	storagedriver.StorageDriver
}

// SetUpSuite sets up the gocheck test suite.
func (suite *DriverSuite) SetUpSuite(c *check.C) {
	if reason := suite.SkipCheck(); reason != "" {
		c.Skip(reason)
	}
	d, err := suite.Constructor()
	c.Assert(err, check.IsNil)
	suite.StorageDriver = d
}

// TearDownSuite tears down the gocheck test suite.
func (suite *DriverSuite) TearDownSuite(c *check.C) {
	if suite.Teardown != nil {
		err := suite.Teardown()
		c.Assert(err, check.IsNil)
	}
}

// TestWriteRead1 tests a simple write-read workflow.
func (suite *DriverSuite) TestWriteRead1(c *check.C) {
	filename := randomString(32)
	contents := []byte("a")
	suite.writeReadCompare(c, filename, contents)
}

// TestWriteRead2 tests a simple write-read workflow with unicode data.
func (suite *DriverSuite) TestWriteRead2(c *check.C) {
	filename := randomString(32)
	contents := []byte("\xc3\x9f")
	suite.writeReadCompare(c, filename, contents)
}

// TestWriteRead3 tests a simple write-read workflow with a small string.
func (suite *DriverSuite) TestWriteRead3(c *check.C) {
	filename := randomString(32)
	contents := []byte(randomString(32))
	suite.writeReadCompare(c, filename, contents)
}

// TestWriteRead4 tests a simple write-read workflow with 1MB of data.
func (suite *DriverSuite) TestWriteRead4(c *check.C) {
	filename := randomString(32)
	contents := []byte(randomString(1024 * 1024))
	suite.writeReadCompare(c, filename, contents)
}

// TestReadNonexistent tests reading content from an empty path.
func (suite *DriverSuite) TestReadNonexistent(c *check.C) {
	filename := randomString(32)
	_, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestWriteReadStreams1 tests a simple write-read streaming workflow.
func (suite *DriverSuite) TestWriteReadStreams1(c *check.C) {
	filename := randomString(32)
	contents := []byte("a")
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadStreams2 tests a simple write-read streaming workflow with
// unicode data.
func (suite *DriverSuite) TestWriteReadStreams2(c *check.C) {
	filename := randomString(32)
	contents := []byte("\xc3\x9f")
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadStreams3 tests a simple write-read streaming workflow with a
// small amount of data.
func (suite *DriverSuite) TestWriteReadStreams3(c *check.C) {
	filename := randomString(32)
	contents := []byte(randomString(32))
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadStreams4 tests a simple write-read streaming workflow with 1MB
// of data.
func (suite *DriverSuite) TestWriteReadStreams4(c *check.C) {
	filename := randomString(32)
	contents := []byte(randomString(1024 * 1024))
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestReadStreamWithOffset tests that the appropriate data is streamed when
// reading with a given offset.
func (suite *DriverSuite) TestReadStreamWithOffset(c *check.C) {
	filename := randomString(32)
	defer suite.StorageDriver.Delete(filename)

	chunkSize := int64(32)

	contentsChunk1 := []byte(randomString(chunkSize))
	contentsChunk2 := []byte(randomString(chunkSize))
	contentsChunk3 := []byte(randomString(chunkSize))

	err := suite.StorageDriver.PutContent(filename, append(append(contentsChunk1, contentsChunk2...), contentsChunk3...))
	c.Assert(err, check.IsNil)

	reader, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, check.IsNil)
	defer reader.Close()

	readContents, err := ioutil.ReadAll(reader)
	c.Assert(err, check.IsNil)

	c.Assert(readContents, check.DeepEquals, append(append(contentsChunk1, contentsChunk2...), contentsChunk3...))

	reader, err = suite.StorageDriver.ReadStream(filename, chunkSize)
	c.Assert(err, check.IsNil)
	defer reader.Close()

	readContents, err = ioutil.ReadAll(reader)
	c.Assert(err, check.IsNil)

	c.Assert(readContents, check.DeepEquals, append(contentsChunk2, contentsChunk3...))

	reader, err = suite.StorageDriver.ReadStream(filename, chunkSize*2)
	c.Assert(err, check.IsNil)
	defer reader.Close()

	readContents, err = ioutil.ReadAll(reader)
	c.Assert(err, check.IsNil)
	c.Assert(readContents, check.DeepEquals, contentsChunk3)
}

// TestContinueStreamAppend tests that a stream write can be appended to without
// corrupting the data.
func (suite *DriverSuite) TestContinueStreamAppend(c *check.C) {
	filename := randomString(32)
	defer suite.StorageDriver.Delete(filename)

	chunkSize := int64(10 * 1024 * 1024)

	contentsChunk1 := []byte(randomString(chunkSize))
	contentsChunk2 := []byte(randomString(chunkSize))
	contentsChunk3 := []byte(randomString(chunkSize))
	contentsChunk4 := []byte(randomString(chunkSize))
	zeroChunk := make([]byte, int64(chunkSize))

	fullContents := append(append(contentsChunk1, contentsChunk2...), contentsChunk3...)

	nn, err := suite.StorageDriver.WriteStream(filename, 0, bytes.NewReader(contentsChunk1))
	c.Assert(err, check.IsNil)
	c.Assert(nn, check.Equals, int64(len(contentsChunk1)))

	fi, err := suite.StorageDriver.Stat(filename)
	c.Assert(err, check.IsNil)
	c.Assert(fi, check.NotNil)
	c.Assert(fi.Size(), check.Equals, int64(len(contentsChunk1)))

	if fi.Size() > chunkSize {
		c.Fatalf("Offset too large, %d > %d", fi.Size(), chunkSize)
	}
	nn, err = suite.StorageDriver.WriteStream(filename, fi.Size(), bytes.NewReader(contentsChunk2))
	c.Assert(err, check.IsNil)
	c.Assert(nn, check.Equals, int64(len(contentsChunk2)))

	fi, err = suite.StorageDriver.Stat(filename)
	c.Assert(err, check.IsNil)
	c.Assert(fi, check.NotNil)
	c.Assert(fi.Size(), check.Equals, 2*chunkSize)

	if fi.Size() > 2*chunkSize {
		c.Fatalf("Offset too large, %d > %d", fi.Size(), 2*chunkSize)
	}

	nn, err = suite.StorageDriver.WriteStream(filename, fi.Size(), bytes.NewReader(fullContents[fi.Size():]))
	c.Assert(err, check.IsNil)
	c.Assert(nn, check.Equals, int64(len(fullContents[fi.Size():])))

	received, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, check.IsNil)
	c.Assert(received, check.DeepEquals, fullContents)

	// Writing past size of file extends file (no offest error). We would like
	// to write chunk 4 one chunk length past chunk 3. It should be successful
	// and the resulting file will be 5 chunks long, with a chunk of all
	// zeros.

	fullContents = append(fullContents, zeroChunk...)
	fullContents = append(fullContents, contentsChunk4...)

	nn, err = suite.StorageDriver.WriteStream(filename, int64(len(fullContents))-chunkSize, bytes.NewReader(contentsChunk4))
	c.Assert(err, check.IsNil)
	c.Assert(nn, check.Equals, chunkSize)

	fi, err = suite.StorageDriver.Stat(filename)
	c.Assert(err, check.IsNil)
	c.Assert(fi, check.NotNil)
	c.Assert(fi.Size(), check.Equals, int64(len(fullContents)))

	received, err = suite.StorageDriver.GetContent(filename)
	c.Assert(err, check.IsNil)
	c.Assert(len(received), check.Equals, len(fullContents))
	c.Assert(received[chunkSize*3:chunkSize*4], check.DeepEquals, zeroChunk)
	c.Assert(received[chunkSize*4:chunkSize*5], check.DeepEquals, contentsChunk4)
	c.Assert(received, check.DeepEquals, fullContents)

	// Ensure that negative offsets return correct error.
	nn, err = suite.StorageDriver.WriteStream(filename, -1, bytes.NewReader(zeroChunk))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.InvalidOffsetError{})
	c.Assert(err.(storagedriver.InvalidOffsetError).Path, check.Equals, filename)
	c.Assert(err.(storagedriver.InvalidOffsetError).Offset, check.Equals, int64(-1))
}

// TestReadNonexistentStream tests that reading a stream for a nonexistent path
// fails.
func (suite *DriverSuite) TestReadNonexistentStream(c *check.C) {
	filename := randomString(32)
	_, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestList checks the returned list of keys after populating a directory tree.
func (suite *DriverSuite) TestList(c *check.C) {
	rootDirectory := "/" + randomString(int64(8+rand.Intn(8)))
	defer suite.StorageDriver.Delete(rootDirectory)

	parentDirectory := rootDirectory + "/" + randomString(int64(8+rand.Intn(8)))
	childFiles := make([]string, 50)
	for i := 0; i < len(childFiles); i++ {
		childFile := parentDirectory + "/" + randomString(int64(8+rand.Intn(8)))
		childFiles[i] = childFile
		err := suite.StorageDriver.PutContent(childFile, []byte(randomString(32)))
		c.Assert(err, check.IsNil)
	}
	sort.Strings(childFiles)

	keys, err := suite.StorageDriver.List("/")
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []string{rootDirectory})

	keys, err = suite.StorageDriver.List(rootDirectory)
	c.Assert(err, check.IsNil)
	c.Assert(keys, check.DeepEquals, []string{parentDirectory})

	keys, err = suite.StorageDriver.List(parentDirectory)
	c.Assert(err, check.IsNil)

	sort.Strings(keys)
	c.Assert(keys, check.DeepEquals, childFiles)
}

// TestMove checks that a moved object no longer exists at the source path and
// does exist at the destination.
func (suite *DriverSuite) TestMove(c *check.C) {
	contents := []byte(randomString(32))
	sourcePath := randomString(32)
	destPath := randomString(32)

	defer suite.StorageDriver.Delete(sourcePath)
	defer suite.StorageDriver.Delete(destPath)

	err := suite.StorageDriver.PutContent(sourcePath, contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.Move(sourcePath, destPath)
	c.Assert(err, check.IsNil)

	received, err := suite.StorageDriver.GetContent(destPath)
	c.Assert(err, check.IsNil)
	c.Assert(received, check.DeepEquals, contents)

	_, err = suite.StorageDriver.GetContent(sourcePath)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestMoveNonexistent checks that moving a nonexistent key fails
func (suite *DriverSuite) TestMoveNonexistent(c *check.C) {
	sourcePath := randomString(32)
	destPath := randomString(32)

	err := suite.StorageDriver.Move(sourcePath, destPath)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestDelete checks that the delete operation removes data from the storage
// driver
func (suite *DriverSuite) TestDelete(c *check.C) {
	filename := randomString(32)
	contents := []byte(randomString(32))

	defer suite.StorageDriver.Delete(filename)

	err := suite.StorageDriver.PutContent(filename, contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.Delete(filename)
	c.Assert(err, check.IsNil)

	_, err = suite.StorageDriver.GetContent(filename)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestDeleteNonexistent checks that removing a nonexistent key fails.
func (suite *DriverSuite) TestDeleteNonexistent(c *check.C) {
	filename := randomString(32)
	err := suite.StorageDriver.Delete(filename)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestDeleteFolder checks that deleting a folder removes all child elements.
func (suite *DriverSuite) TestDeleteFolder(c *check.C) {
	dirname := randomString(32)
	filename1 := randomString(32)
	filename2 := randomString(32)
	contents := []byte(randomString(32))

	defer suite.StorageDriver.Delete(path.Join(dirname, filename1))
	defer suite.StorageDriver.Delete(path.Join(dirname, filename2))

	err := suite.StorageDriver.PutContent(path.Join(dirname, filename1), contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.PutContent(path.Join(dirname, filename2), contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.Delete(dirname)
	c.Assert(err, check.IsNil)

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename1))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename2))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

func (suite *DriverSuite) TestStatCall(c *check.C) {
	content := randomString(4096)
	dirPath := randomString(32)
	fileName := randomString(32)
	filePath := path.Join(dirPath, fileName)

	// Call on non-existent file/dir, check error.
	fi, err := suite.StorageDriver.Stat(filePath)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
	c.Assert(fi, check.IsNil)

	err = suite.StorageDriver.PutContent(filePath, []byte(content))
	c.Assert(err, check.IsNil)

	// Call on regular file, check results
	start := time.Now().Truncate(time.Second) // truncated for filesystem
	fi, err = suite.StorageDriver.Stat(filePath)
	c.Assert(err, check.IsNil)
	expectedModTime := time.Now()
	c.Assert(fi, check.NotNil)
	c.Assert(fi.Path(), check.Equals, filePath)
	c.Assert(fi.Size(), check.Equals, int64(len(content)))
	c.Assert(fi.IsDir(), check.Equals, false)

	if start.After(fi.ModTime()) {
		c.Fatalf("modtime %s before file created (%v)", fi.ModTime(), start)
	}

	if fi.ModTime().After(expectedModTime) {
		c.Fatalf("modtime %s after file created (%v)", fi.ModTime(), expectedModTime)
	}

	// Call on directory
	start = time.Now().Truncate(time.Second)
	fi, err = suite.StorageDriver.Stat(dirPath)
	c.Assert(err, check.IsNil)
	expectedModTime = time.Now()
	c.Assert(fi, check.NotNil)
	c.Assert(fi.Path(), check.Equals, dirPath)
	c.Assert(fi.Size(), check.Equals, int64(0))
	c.Assert(fi.IsDir(), check.Equals, true)

	if start.After(fi.ModTime()) {
		c.Fatalf("modtime %s before file created (%v)", fi.ModTime(), start)
	}

	if fi.ModTime().After(expectedModTime) {
		c.Fatalf("modtime %s after file created (%v)", fi.ModTime(), expectedModTime)
	}
}

// TestConcurrentFileStreams checks that multiple *os.File objects can be passed
// in to WriteStream concurrently without hanging.
// TODO(bbland): fix this test...
func (suite *DriverSuite) TestConcurrentFileStreams(c *check.C) {
	if _, isIPC := suite.StorageDriver.(*ipc.StorageDriverClient); isIPC {
		c.Skip("Need to fix out-of-process concurrency")
	}

	var wg sync.WaitGroup

	testStream := func(size int64) {
		defer wg.Done()
		suite.testFileStreams(c, size)
	}

	wg.Add(6)
	go testStream(8 * 1024 * 1024)
	go testStream(4 * 1024 * 1024)
	go testStream(2 * 1024 * 1024)
	go testStream(1024 * 1024)
	go testStream(1024)
	go testStream(64)

	wg.Wait()
}

func (suite *DriverSuite) testFileStreams(c *check.C, size int64) {
	tf, err := ioutil.TempFile("", "tf")
	c.Assert(err, check.IsNil)
	defer os.Remove(tf.Name())

	tfName := path.Base(tf.Name())
	defer suite.StorageDriver.Delete(tfName)

	contents := []byte(randomString(size))

	_, err = tf.Write(contents)
	c.Assert(err, check.IsNil)

	tf.Sync()
	tf.Seek(0, os.SEEK_SET)

	nn, err := suite.StorageDriver.WriteStream(tfName, 0, tf)
	c.Assert(err, check.IsNil)
	c.Assert(nn, check.Equals, size)

	reader, err := suite.StorageDriver.ReadStream(tfName, 0)
	c.Assert(err, check.IsNil)
	defer reader.Close()

	readContents, err := ioutil.ReadAll(reader)
	c.Assert(err, check.IsNil)

	c.Assert(readContents, check.DeepEquals, contents)
}

func (suite *DriverSuite) writeReadCompare(c *check.C, filename string, contents []byte) {
	defer suite.StorageDriver.Delete(filename)

	err := suite.StorageDriver.PutContent(filename, contents)
	c.Assert(err, check.IsNil)

	readContents, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, check.IsNil)

	c.Assert(readContents, check.DeepEquals, contents)
}

func (suite *DriverSuite) writeReadCompareStreams(c *check.C, filename string, contents []byte) {
	defer suite.StorageDriver.Delete(filename)

	nn, err := suite.StorageDriver.WriteStream(filename, 0, bytes.NewReader(contents))
	c.Assert(err, check.IsNil)
	c.Assert(nn, check.Equals, int64(len(contents)))

	reader, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, check.IsNil)
	defer reader.Close()

	readContents, err := ioutil.ReadAll(reader)
	c.Assert(err, check.IsNil)

	c.Assert(readContents, check.DeepEquals, contents)
}

var pathChars = []byte("abcdefghijklmnopqrstuvwxyz")

func randomString(length int64) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = pathChars[rand.Intn(len(pathChars))]
	}
	return string(b)
}

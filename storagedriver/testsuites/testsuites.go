package testsuites

import (
	"bytes"
	"crypto/sha1"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker-registry/storagedriver"

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
	panic("ipc testing is disabled for now")

	// NOTE(stevvooe): IPC testing is disabled for now. Uncomment the code
	// block before and remove the panic when we phase it back in.

	// suite := &DriverSuite{
	// 	Constructor: func() (storagedriver.StorageDriver, error) {
	// 		d, err := ipc.NewDriverClient(driverName, ipcParams)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		err = d.Start()
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		return d, nil
	// 	},
	// 	SkipCheck: skipCheck,
	// }
	// suite.Teardown = func() error {
	// 	if suite.StorageDriver == nil {
	// 		return nil
	// 	}

	// 	driverClient := suite.StorageDriver.(*ipc.StorageDriverClient)
	// 	return driverClient.Stop()
	// }
	// check.Suite(suite)
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
	filename := randomPath(32)
	contents := []byte("a")
	suite.writeReadCompare(c, filename, contents)
}

// TestWriteRead2 tests a simple write-read workflow with unicode data.
func (suite *DriverSuite) TestWriteRead2(c *check.C) {
	filename := randomPath(32)
	contents := []byte("\xc3\x9f")
	suite.writeReadCompare(c, filename, contents)
}

// TestWriteRead3 tests a simple write-read workflow with a small string.
func (suite *DriverSuite) TestWriteRead3(c *check.C) {
	filename := randomPath(32)
	contents := randomContents(32)
	suite.writeReadCompare(c, filename, contents)
}

// TestWriteRead4 tests a simple write-read workflow with 1MB of data.
func (suite *DriverSuite) TestWriteRead4(c *check.C) {
	filename := randomPath(32)
	contents := randomContents(1024 * 1024)
	suite.writeReadCompare(c, filename, contents)
}

// TestWriteReadNonUTF8 tests that non-utf8 data may be written to the storage
// driver safely.
func (suite *DriverSuite) TestWriteReadNonUTF8(c *check.C) {
	filename := randomPath(32)
	contents := []byte{0x80, 0x80, 0x80, 0x80}
	suite.writeReadCompare(c, filename, contents)
}

// TestTruncate tests that putting smaller contents than an original file does
// remove the excess contents.
func (suite *DriverSuite) TestTruncate(c *check.C) {
	filename := randomPath(32)
	contents := randomContents(1024 * 1024)
	suite.writeReadCompare(c, filename, contents)

	contents = randomContents(1024)
	suite.writeReadCompare(c, filename, contents)
}

// TestReadNonexistent tests reading content from an empty path.
func (suite *DriverSuite) TestReadNonexistent(c *check.C) {
	filename := randomPath(32)
	_, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestWriteReadStreams1 tests a simple write-read streaming workflow.
func (suite *DriverSuite) TestWriteReadStreams1(c *check.C) {
	filename := randomPath(32)
	contents := []byte("a")
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadStreams2 tests a simple write-read streaming workflow with
// unicode data.
func (suite *DriverSuite) TestWriteReadStreams2(c *check.C) {
	filename := randomPath(32)
	contents := []byte("\xc3\x9f")
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadStreams3 tests a simple write-read streaming workflow with a
// small amount of data.
func (suite *DriverSuite) TestWriteReadStreams3(c *check.C) {
	filename := randomPath(32)
	contents := randomContents(32)
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadStreams4 tests a simple write-read streaming workflow with 1MB
// of data.
func (suite *DriverSuite) TestWriteReadStreams4(c *check.C) {
	filename := randomPath(32)
	contents := randomContents(1024 * 1024)
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadStreamsNonUTF8 tests that non-utf8 data may be written to the
// storage driver safely.
func (suite *DriverSuite) TestWriteReadStreamsNonUTF8(c *check.C) {
	filename := randomPath(32)
	contents := []byte{0x80, 0x80, 0x80, 0x80}
	suite.writeReadCompareStreams(c, filename, contents)
}

// TestWriteReadLargeStreams tests that a 5GB file may be written to the storage
// driver safely.
func (suite *DriverSuite) TestWriteReadLargeStreams(c *check.C) {
	if testing.Short() {
		c.Skip("Skipping test in short mode")
	}

	filename := randomPath(32)
	defer suite.StorageDriver.Delete(firstPart(filename))

	checksum := sha1.New()
	var offset int64
	var chunkSize int64 = 1024 * 1024

	for i := 0; i < 5*1024; i++ {
		contents := randomContents(chunkSize)
		written, err := suite.StorageDriver.WriteStream(filename, offset, io.TeeReader(bytes.NewReader(contents), checksum))
		c.Assert(err, check.IsNil)
		c.Assert(written, check.Equals, chunkSize)
		offset += chunkSize
	}
	reader, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, check.IsNil)

	writtenChecksum := sha1.New()
	io.Copy(writtenChecksum, reader)

	c.Assert(writtenChecksum.Sum(nil), check.DeepEquals, checksum.Sum(nil))
}

// TestReadStreamWithOffset tests that the appropriate data is streamed when
// reading with a given offset.
func (suite *DriverSuite) TestReadStreamWithOffset(c *check.C) {
	filename := randomPath(32)
	defer suite.StorageDriver.Delete(firstPart(filename))

	chunkSize := int64(32)

	contentsChunk1 := randomContents(chunkSize)
	contentsChunk2 := randomContents(chunkSize)
	contentsChunk3 := randomContents(chunkSize)

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

	// Ensure we get invalid offest for negative offsets.
	reader, err = suite.StorageDriver.ReadStream(filename, -1)
	c.Assert(err, check.FitsTypeOf, storagedriver.InvalidOffsetError{})
	c.Assert(err.(storagedriver.InvalidOffsetError).Offset, check.Equals, int64(-1))
	c.Assert(err.(storagedriver.InvalidOffsetError).Path, check.Equals, filename)
	c.Assert(reader, check.IsNil)

	// Read past the end of the content and make sure we get a reader that
	// returns 0 bytes and io.EOF
	reader, err = suite.StorageDriver.ReadStream(filename, chunkSize*3)
	c.Assert(err, check.IsNil)
	defer reader.Close()

	buf := make([]byte, chunkSize)
	n, err := reader.Read(buf)
	c.Assert(err, check.Equals, io.EOF)
	c.Assert(n, check.Equals, 0)

	// Check the N-1 boundary condition, ensuring we get 1 byte then io.EOF.
	reader, err = suite.StorageDriver.ReadStream(filename, chunkSize*3-1)
	c.Assert(err, check.IsNil)
	defer reader.Close()

	n, err = reader.Read(buf)
	c.Assert(n, check.Equals, 1)

	// We don't care whether the io.EOF comes on the this read or the first
	// zero read, but the only error acceptable here is io.EOF.
	if err != nil {
		c.Assert(err, check.Equals, io.EOF)
	}

	// Any more reads should result in zero bytes and io.EOF
	n, err = reader.Read(buf)
	c.Assert(n, check.Equals, 0)
	c.Assert(err, check.Equals, io.EOF)
}

// TestContinueStreamAppend tests that a stream write can be appended to without
// corrupting the data.
func (suite *DriverSuite) TestContinueStreamAppend(c *check.C) {
	filename := randomPath(32)
	defer suite.StorageDriver.Delete(firstPart(filename))

	chunkSize := int64(10 * 1024 * 1024)

	contentsChunk1 := randomContents(chunkSize)
	contentsChunk2 := randomContents(chunkSize)
	contentsChunk3 := randomContents(chunkSize)
	contentsChunk4 := randomContents(chunkSize)
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
	filename := randomPath(32)

	_, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})

	_, err = suite.StorageDriver.ReadStream(filename, 64)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestList checks the returned list of keys after populating a directory tree.
func (suite *DriverSuite) TestList(c *check.C) {
	rootDirectory := "/" + randomFilename(int64(8+rand.Intn(8)))
	defer suite.StorageDriver.Delete("/")

	parentDirectory := rootDirectory + "/" + randomFilename(int64(8+rand.Intn(8)))
	childFiles := make([]string, 50)
	for i := 0; i < len(childFiles); i++ {
		childFile := parentDirectory + "/" + randomFilename(int64(8+rand.Intn(8)))
		childFiles[i] = childFile
		err := suite.StorageDriver.PutContent(childFile, randomContents(32))
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

	// A few checks to add here (check out #819 for more discussion on this):
	// 1. Ensure that all paths are absolute.
	// 2. Ensure that listings only include direct children.
	// 3. Ensure that we only respond to directory listings that end with a slash (maybe?).
}

// TestMove checks that a moved object no longer exists at the source path and
// does exist at the destination.
func (suite *DriverSuite) TestMove(c *check.C) {
	contents := randomContents(32)
	sourcePath := randomPath(32)
	destPath := randomPath(32)

	defer suite.StorageDriver.Delete(firstPart(sourcePath))
	defer suite.StorageDriver.Delete(firstPart(destPath))

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

// TestMoveOverwrite checks that a moved object no longer exists at the source
// path and overwrites the contents at the destination.
func (suite *DriverSuite) TestMoveOverwrite(c *check.C) {
	sourcePath := randomPath(32)
	destPath := randomPath(32)
	sourceContents := randomContents(32)
	destContents := randomContents(64)

	defer suite.StorageDriver.Delete(firstPart(sourcePath))
	defer suite.StorageDriver.Delete(firstPart(destPath))

	err := suite.StorageDriver.PutContent(sourcePath, sourceContents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.PutContent(destPath, destContents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.Move(sourcePath, destPath)
	c.Assert(err, check.IsNil)

	received, err := suite.StorageDriver.GetContent(destPath)
	c.Assert(err, check.IsNil)
	c.Assert(received, check.DeepEquals, sourceContents)

	_, err = suite.StorageDriver.GetContent(sourcePath)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestMoveNonexistent checks that moving a nonexistent key fails and does not
// delete the data at the destination path.
func (suite *DriverSuite) TestMoveNonexistent(c *check.C) {
	contents := randomContents(32)
	sourcePath := randomPath(32)
	destPath := randomPath(32)

	defer suite.StorageDriver.Delete(firstPart(destPath))

	err := suite.StorageDriver.PutContent(destPath, contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.Move(sourcePath, destPath)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})

	received, err := suite.StorageDriver.GetContent(destPath)
	c.Assert(err, check.IsNil)
	c.Assert(received, check.DeepEquals, contents)
}

// TestDelete checks that the delete operation removes data from the storage
// driver
func (suite *DriverSuite) TestDelete(c *check.C) {
	filename := randomPath(32)
	contents := randomContents(32)

	defer suite.StorageDriver.Delete(firstPart(filename))

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
	filename := randomPath(32)
	err := suite.StorageDriver.Delete(filename)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestDeleteFolder checks that deleting a folder removes all child elements.
func (suite *DriverSuite) TestDeleteFolder(c *check.C) {
	dirname := randomPath(32)
	filename1 := randomPath(32)
	filename2 := randomPath(32)
	filename3 := randomPath(32)
	contents := randomContents(32)

	defer suite.StorageDriver.Delete(firstPart(dirname))

	err := suite.StorageDriver.PutContent(path.Join(dirname, filename1), contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.PutContent(path.Join(dirname, filename2), contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.PutContent(path.Join(dirname, filename3), contents)
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.Delete(path.Join(dirname, filename1))
	c.Assert(err, check.IsNil)

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename1))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename2))
	c.Assert(err, check.IsNil)

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename3))
	c.Assert(err, check.IsNil)

	err = suite.StorageDriver.Delete(dirname)
	c.Assert(err, check.IsNil)

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename1))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename2))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename3))
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
}

// TestStatCall runs verifies the implementation of the storagedriver's Stat call.
func (suite *DriverSuite) TestStatCall(c *check.C) {
	content := randomContents(4096)
	dirPath := randomPath(32)
	fileName := randomFilename(32)
	filePath := path.Join(dirPath, fileName)

	defer suite.StorageDriver.Delete(firstPart(dirPath))

	// Call on non-existent file/dir, check error.
	fi, err := suite.StorageDriver.Stat(dirPath)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
	c.Assert(fi, check.IsNil)

	fi, err = suite.StorageDriver.Stat(filePath)
	c.Assert(err, check.NotNil)
	c.Assert(err, check.FitsTypeOf, storagedriver.PathNotFoundError{})
	c.Assert(fi, check.IsNil)

	err = suite.StorageDriver.PutContent(filePath, content)
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
	// if _, isIPC := suite.StorageDriver.(*ipc.StorageDriverClient); isIPC {
	// 	c.Skip("Need to fix out-of-process concurrency")
	// }

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

	contents := randomContents(size)

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
	defer suite.StorageDriver.Delete(firstPart(filename))

	err := suite.StorageDriver.PutContent(filename, contents)
	c.Assert(err, check.IsNil)

	readContents, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, check.IsNil)

	c.Assert(readContents, check.DeepEquals, contents)
}

func (suite *DriverSuite) writeReadCompareStreams(c *check.C, filename string, contents []byte) {
	defer suite.StorageDriver.Delete(firstPart(filename))

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

var filenameChars = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

func randomPath(length int64) string {
	path := ""
	for int64(len(path)) < length {
		chunkLength := rand.Int63n(length-int64(len(path))) + 1
		chunk := randomFilename(chunkLength)
		path += chunk
		if length-int64(len(path)) == 1 {
			path += randomFilename(1)
		} else if length-int64(len(path)) > 1 {
			path += "/"
		}
	}
	return path
}

func randomFilename(length int64) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = filenameChars[rand.Intn(len(filenameChars))]
	}
	return string(b)
}

func randomContents(length int64) []byte {
	b := make([]byte, length)
	for i := range b {
		b[i] = byte(rand.Intn(2 << 8))
	}
	return b
}

func firstPart(filePath string) string {
	for {
		if filePath[len(filePath)-1] == '/' {
			filePath = filePath[:len(filePath)-1]
		}

		dir, file := path.Split(filePath)
		if dir == "" && file == "" {
			return "/"
		}
		if dir == "" {
			return file
		}
		if file == "" {
			return dir
		}
		filePath = dir
	}
}

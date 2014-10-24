package testsuites

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"path"
	"sort"
	"testing"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/ipc"

	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner
func Test(t *testing.T) { TestingT(t) }

func RegisterInProcessSuite(driverConstructor DriverConstructor) {
	Suite(&DriverSuite{
		Constructor: driverConstructor,
	})
}

func RegisterIPCSuite(driverName string, ipcParams map[string]string) {
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
	}
	suite.Teardown = func() error {
		driverClient := suite.StorageDriver.(*ipc.StorageDriverClient)
		return driverClient.Stop()
	}
	Suite(suite)
}

type DriverConstructor func() (storagedriver.StorageDriver, error)
type DriverTeardown func() error

type DriverSuite struct {
	Constructor DriverConstructor
	Teardown    DriverTeardown
	storagedriver.StorageDriver
}

type TestDriverConfig struct {
	name   string
	params map[string]string
}

func (suite *DriverSuite) SetUpSuite(c *C) {
	d, err := suite.Constructor()
	c.Assert(err, IsNil)
	suite.StorageDriver = d
}

func (suite *DriverSuite) TearDownSuite(c *C) {
	if suite.Teardown != nil {
		err := suite.Teardown()
		c.Assert(err, IsNil)
	}
}

func (suite *DriverSuite) TestWriteRead1(c *C) {
	filename := randomString(32)
	contents := []byte("a")
	suite.writeReadCompare(c, filename, contents, contents)
}

func (suite *DriverSuite) TestWriteRead2(c *C) {
	filename := randomString(32)
	contents := []byte("\xc3\x9f")
	suite.writeReadCompare(c, filename, contents, contents)
}

func (suite *DriverSuite) TestWriteRead3(c *C) {
	filename := randomString(32)
	contents := []byte(randomString(32))
	suite.writeReadCompare(c, filename, contents, contents)
}

func (suite *DriverSuite) TestWriteRead4(c *C) {
	filename := randomString(32)
	contents := []byte(randomString(1024 * 1024))
	suite.writeReadCompare(c, filename, contents, contents)
}

func (suite *DriverSuite) TestReadNonexistent(c *C) {
	filename := randomString(32)
	_, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, NotNil)
}

func (suite *DriverSuite) TestWriteReadStreams1(c *C) {
	filename := randomString(32)
	contents := []byte("a")
	suite.writeReadCompareStreams(c, filename, contents, contents)
}

func (suite *DriverSuite) TestWriteReadStreams2(c *C) {
	filename := randomString(32)
	contents := []byte("\xc3\x9f")
	suite.writeReadCompareStreams(c, filename, contents, contents)
}

func (suite *DriverSuite) TestWriteReadStreams3(c *C) {
	filename := randomString(32)
	contents := []byte(randomString(32))
	suite.writeReadCompareStreams(c, filename, contents, contents)
}

func (suite *DriverSuite) TestWriteReadStreams4(c *C) {
	filename := randomString(32)
	contents := []byte(randomString(1024 * 1024))
	suite.writeReadCompareStreams(c, filename, contents, contents)
}

func (suite *DriverSuite) TestContinueStreamAppend(c *C) {
	filename := randomString(32)
	defer suite.StorageDriver.Delete(filename)

	chunkSize := uint64(5 * 1024 * 1024)

	contentsChunk1 := []byte(randomString(chunkSize))
	contentsChunk2 := []byte(randomString(chunkSize))
	contentsChunk3 := []byte(randomString(chunkSize))

	fullContents := append(append(contentsChunk1, contentsChunk2...), contentsChunk3...)

	err := suite.StorageDriver.WriteStream(filename, 0, 3*chunkSize, ioutil.NopCloser(bytes.NewReader(contentsChunk1)))
	c.Assert(err, IsNil)

	offset, err := suite.StorageDriver.ResumeWritePosition(filename)
	c.Assert(err, IsNil)
	if offset > chunkSize {
		c.Fatalf("Offset too large, %d > %d", offset, chunkSize)
	}
	err = suite.StorageDriver.WriteStream(filename, offset, 3*chunkSize, ioutil.NopCloser(bytes.NewReader(fullContents[offset:2*chunkSize])))
	c.Assert(err, IsNil)

	offset, err = suite.StorageDriver.ResumeWritePosition(filename)
	c.Assert(err, IsNil)
	if offset > 2*chunkSize {
		c.Fatalf("Offset too large, %d > %d", offset, 2*chunkSize)
	}

	err = suite.StorageDriver.WriteStream(filename, offset, 3*chunkSize, ioutil.NopCloser(bytes.NewReader(fullContents[offset:])))
	c.Assert(err, IsNil)

	received, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, IsNil)
	c.Assert(received, DeepEquals, fullContents)
}

func (suite *DriverSuite) TestReadStreamWithOffset(c *C) {
	filename := randomString(32)
	defer suite.StorageDriver.Delete(filename)

	chunkSize := uint64(32)

	contentsChunk1 := []byte(randomString(chunkSize))
	contentsChunk2 := []byte(randomString(chunkSize))
	contentsChunk3 := []byte(randomString(chunkSize))

	err := suite.StorageDriver.PutContent(filename, append(append(contentsChunk1, contentsChunk2...), contentsChunk3...))
	c.Assert(err, IsNil)

	reader, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, IsNil)
	defer reader.Close()

	readContents, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)

	c.Assert(readContents, DeepEquals, append(append(contentsChunk1, contentsChunk2...), contentsChunk3...))

	reader, err = suite.StorageDriver.ReadStream(filename, chunkSize)
	c.Assert(err, IsNil)
	defer reader.Close()

	readContents, err = ioutil.ReadAll(reader)
	c.Assert(err, IsNil)

	c.Assert(readContents, DeepEquals, append(contentsChunk2, contentsChunk3...))

	reader, err = suite.StorageDriver.ReadStream(filename, chunkSize*2)
	c.Assert(err, IsNil)
	defer reader.Close()

	readContents, err = ioutil.ReadAll(reader)
	c.Assert(err, IsNil)

	c.Assert(readContents, DeepEquals, contentsChunk3)
}

func (suite *DriverSuite) TestReadNonexistentStream(c *C) {
	filename := randomString(32)
	_, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, NotNil)
}

func (suite *DriverSuite) TestList(c *C) {
	rootDirectory := randomString(uint64(8 + rand.Intn(8)))
	defer suite.StorageDriver.Delete(rootDirectory)

	parentDirectory := rootDirectory + "/" + randomString(uint64(8+rand.Intn(8)))
	childFiles := make([]string, 50)
	for i := 0; i < len(childFiles); i++ {
		childFile := parentDirectory + "/" + randomString(uint64(8+rand.Intn(8)))
		childFiles[i] = childFile
		err := suite.StorageDriver.PutContent(childFile, []byte(randomString(32)))
		c.Assert(err, IsNil)
	}
	sort.Strings(childFiles)

	keys, err := suite.StorageDriver.List(rootDirectory)
	c.Assert(err, IsNil)
	c.Assert(keys, DeepEquals, []string{parentDirectory})

	keys, err = suite.StorageDriver.List(parentDirectory)
	c.Assert(err, IsNil)

	sort.Strings(keys)
	c.Assert(keys, DeepEquals, childFiles)
}

func (suite *DriverSuite) TestMove(c *C) {
	contents := []byte(randomString(32))
	sourcePath := randomString(32)
	destPath := randomString(32)

	defer suite.StorageDriver.Delete(sourcePath)
	defer suite.StorageDriver.Delete(destPath)

	err := suite.StorageDriver.PutContent(sourcePath, contents)
	c.Assert(err, IsNil)

	err = suite.StorageDriver.Move(sourcePath, destPath)
	c.Assert(err, IsNil)

	received, err := suite.StorageDriver.GetContent(destPath)
	c.Assert(err, IsNil)
	c.Assert(received, DeepEquals, contents)

	_, err = suite.StorageDriver.GetContent(sourcePath)
	c.Assert(err, NotNil)
}

func (suite *DriverSuite) TestMoveNonexistent(c *C) {
	sourcePath := randomString(32)
	destPath := randomString(32)

	err := suite.StorageDriver.Move(sourcePath, destPath)
	c.Assert(err, NotNil)
}

func (suite *DriverSuite) TestRemove(c *C) {
	filename := randomString(32)
	contents := []byte(randomString(32))

	defer suite.StorageDriver.Delete(filename)

	err := suite.StorageDriver.PutContent(filename, contents)
	c.Assert(err, IsNil)

	err = suite.StorageDriver.Delete(filename)
	c.Assert(err, IsNil)

	_, err = suite.StorageDriver.GetContent(filename)
	c.Assert(err, NotNil)
}

func (suite *DriverSuite) TestRemoveNonexistent(c *C) {
	filename := randomString(32)
	err := suite.StorageDriver.Delete(filename)
	c.Assert(err, NotNil)
}

func (suite *DriverSuite) TestRemoveFolder(c *C) {
	dirname := randomString(32)
	filename1 := randomString(32)
	filename2 := randomString(32)
	contents := []byte(randomString(32))

	defer suite.StorageDriver.Delete(path.Join(dirname, filename1))
	defer suite.StorageDriver.Delete(path.Join(dirname, filename2))

	err := suite.StorageDriver.PutContent(path.Join(dirname, filename1), contents)
	c.Assert(err, IsNil)

	err = suite.StorageDriver.PutContent(path.Join(dirname, filename2), contents)
	c.Assert(err, IsNil)

	err = suite.StorageDriver.Delete(dirname)
	c.Assert(err, IsNil)

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename1))
	c.Assert(err, NotNil)

	_, err = suite.StorageDriver.GetContent(path.Join(dirname, filename2))
	c.Assert(err, NotNil)
}

func (suite *DriverSuite) writeReadCompare(c *C, filename string, contents, expected []byte) {
	defer suite.StorageDriver.Delete(filename)

	err := suite.StorageDriver.PutContent(filename, contents)
	c.Assert(err, IsNil)

	readContents, err := suite.StorageDriver.GetContent(filename)
	c.Assert(err, IsNil)

	c.Assert(readContents, DeepEquals, contents)
}

func (suite *DriverSuite) writeReadCompareStreams(c *C, filename string, contents, expected []byte) {
	defer suite.StorageDriver.Delete(filename)

	err := suite.StorageDriver.WriteStream(filename, 0, uint64(len(contents)), ioutil.NopCloser(bytes.NewReader(contents)))
	c.Assert(err, IsNil)

	reader, err := suite.StorageDriver.ReadStream(filename, 0)
	c.Assert(err, IsNil)
	defer reader.Close()

	readContents, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)

	c.Assert(readContents, DeepEquals, contents)
}

var pathChars = []byte("abcdefghijklmnopqrstuvwxyz")

func randomString(length uint64) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = pathChars[rand.Intn(len(pathChars))]
	}
	return string(b)
}

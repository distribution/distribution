package filesystem

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/factory"
)

const DriverName = "filesystem"
const DefaultRootDirectory = "/tmp/registry/storage"

func init() {
	factory.Register(DriverName, &filesystemDriverFactory{})
}

// filesystemDriverFactory implements the factory.StorageDriverFactory interface
type filesystemDriverFactory struct{}

func (factory *filesystemDriverFactory) Create(parameters map[string]string) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters), nil
}

// FilesystemDriver is a storagedriver.StorageDriver implementation backed by a local filesystem
// All provided paths will be subpaths of the RootDirectory
type FilesystemDriver struct {
	rootDirectory string
}

// FromParameters constructs a new FilesystemDriver with a given parameters map
// Optional Parameters:
// - rootdirectory
func FromParameters(parameters map[string]string) *FilesystemDriver {
	var rootDirectory = DefaultRootDirectory
	if parameters != nil {
		rootDir, ok := parameters["rootdirectory"]
		if ok {
			rootDirectory = rootDir
		}
	}
	return New(rootDirectory)
}

// New constructs a new FilesystemDriver with a given rootDirectory
func New(rootDirectory string) *FilesystemDriver {
	return &FilesystemDriver{rootDirectory}
}

// subPath returns the absolute path of a key within the FilesystemDriver's storage
func (d *FilesystemDriver) subPath(subPath string) string {
	return path.Join(d.rootDirectory, subPath)
}

// Implement the storagedriver.StorageDriver interface

func (d *FilesystemDriver) GetContent(path string) ([]byte, error) {
	contents, err := ioutil.ReadFile(d.subPath(path))
	if err != nil {
		return nil, storagedriver.PathNotFoundError{path}
	}
	return contents, nil
}

func (d *FilesystemDriver) PutContent(subPath string, contents []byte) error {
	fullPath := d.subPath(subPath)
	parentDir := path.Dir(fullPath)
	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(fullPath, contents, 0644)
	return err
}

func (d *FilesystemDriver) ReadStream(path string, offset uint64) (io.ReadCloser, error) {
	file, err := os.OpenFile(d.subPath(path), os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}

	seekPos, err := file.Seek(int64(offset), os.SEEK_SET)
	if err != nil {
		file.Close()
		return nil, err
	} else if seekPos < int64(offset) {
		file.Close()
		return nil, storagedriver.InvalidOffsetError{path, offset}
	}

	return file, nil
}

func (d *FilesystemDriver) WriteStream(subPath string, offset, size uint64, reader io.ReadCloser) error {
	defer reader.Close()

	resumableOffset, err := d.CurrentSize(subPath)
	if _, pathNotFound := err.(storagedriver.PathNotFoundError); err != nil && !pathNotFound {
		return err
	}

	if offset > resumableOffset {
		return storagedriver.InvalidOffsetError{subPath, offset}
	}

	fullPath := d.subPath(subPath)
	parentDir := path.Dir(fullPath)
	err = os.MkdirAll(parentDir, 0755)
	if err != nil {
		return err
	}

	var file *os.File
	if offset == 0 {
		file, err = os.Create(fullPath)
	} else {
		file, err = os.OpenFile(fullPath, os.O_WRONLY|os.O_APPEND, 0)
	}

	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, 32*1024)
	for {
		bytesRead, er := reader.Read(buf)
		if bytesRead > 0 {
			bytesWritten, ew := file.WriteAt(buf[0:bytesRead], int64(offset))
			if bytesWritten > 0 {
				offset += uint64(bytesWritten)
			}
			if ew != nil {
				err = ew
				break
			}
			if bytesRead != bytesWritten {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return err
}

func (d *FilesystemDriver) CurrentSize(subPath string) (uint64, error) {
	fullPath := d.subPath(subPath)

	fileInfo, err := os.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	} else if err != nil {
		return 0, storagedriver.PathNotFoundError{subPath}
	}
	return uint64(fileInfo.Size()), nil
}

func (d *FilesystemDriver) List(subPath string) ([]string, error) {
	subPath = strings.TrimRight(subPath, "/")
	fullPath := d.subPath(subPath)

	dir, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}

	fileNames, err := dir.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(fileNames))
	for _, fileName := range fileNames {
		keys = append(keys, path.Join(subPath, fileName))
	}

	return keys, nil
}

func (d *FilesystemDriver) Move(sourcePath string, destPath string) error {
	err := os.Rename(d.subPath(sourcePath), d.subPath(destPath))
	return err
}

func (d *FilesystemDriver) Delete(subPath string) error {
	fullPath := d.subPath(subPath)

	_, err := os.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err != nil {
		return storagedriver.PathNotFoundError{subPath}
	}

	err = os.RemoveAll(fullPath)
	return err
}

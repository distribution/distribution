package filesystem

import (
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/factory"
)

const driverName = "filesystem"
const defaultRootDirectory = "/tmp/registry/storage"

func init() {
	factory.Register(driverName, &filesystemDriverFactory{})
}

// filesystemDriverFactory implements the factory.StorageDriverFactory interface
type filesystemDriverFactory struct{}

func (factory *filesystemDriverFactory) Create(parameters map[string]string) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters), nil
}

// Driver is a storagedriver.StorageDriver implementation backed by a local
// filesystem. All provided paths will be subpaths of the RootDirectory
type Driver struct {
	rootDirectory string
}

// FromParameters constructs a new Driver with a given parameters map
// Optional Parameters:
// - rootdirectory
func FromParameters(parameters map[string]string) *Driver {
	var rootDirectory = defaultRootDirectory
	if parameters != nil {
		rootDir, ok := parameters["rootdirectory"]
		if ok {
			rootDirectory = rootDir
		}
	}
	return New(rootDirectory)
}

// New constructs a new Driver with a given rootDirectory
func New(rootDirectory string) *Driver {
	return &Driver{rootDirectory}
}

// subPath returns the absolute path of a key within the Driver's storage
func (d *Driver) subPath(subPath string) string {
	return path.Join(d.rootDirectory, subPath)
}

// Implement the storagedriver.StorageDriver interface

// GetContent retrieves the content stored at "path" as a []byte.
func (d *Driver) GetContent(path string) ([]byte, error) {
	contents, err := ioutil.ReadFile(d.subPath(path))
	if err != nil {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return contents, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *Driver) PutContent(subPath string, contents []byte) error {
	fullPath := d.subPath(subPath)
	parentDir := path.Dir(fullPath)
	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(fullPath, contents, 0644)
	return err
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *Driver) ReadStream(path string, offset int64) (io.ReadCloser, error) {
	file, err := os.OpenFile(d.subPath(path), os.O_RDONLY, 0644)
	if err != nil {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	seekPos, err := file.Seek(int64(offset), os.SEEK_SET)
	if err != nil {
		file.Close()
		return nil, err
	} else if seekPos < int64(offset) {
		file.Close()
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	return file, nil
}

// WriteStream stores the contents of the provided io.ReadCloser at a location
// designated by the given path.
func (d *Driver) WriteStream(subPath string, offset, size int64, reader io.ReadCloser) error {
	defer reader.Close()

	resumableOffset, err := d.CurrentSize(subPath)
	if _, pathNotFound := err.(storagedriver.PathNotFoundError); err != nil && !pathNotFound {
		return err
	}

	if offset > int64(resumableOffset) {
		return storagedriver.InvalidOffsetError{Path: subPath, Offset: offset}
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

	// TODO(sday): Use Seek + Copy here.

	buf := make([]byte, 32*1024)
	for {
		bytesRead, er := reader.Read(buf)
		if bytesRead > 0 {
			bytesWritten, ew := file.WriteAt(buf[0:bytesRead], int64(offset))
			if bytesWritten > 0 {
				offset += int64(bytesWritten)
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

// CurrentSize retrieves the curernt size in bytes of the object at the given
// path.
func (d *Driver) CurrentSize(subPath string) (uint64, error) {
	fullPath := d.subPath(subPath)

	fileInfo, err := os.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	} else if err != nil {
		return 0, storagedriver.PathNotFoundError{Path: subPath}
	}
	return uint64(fileInfo.Size()), nil
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (d *Driver) List(subPath string) ([]string, error) {
	if subPath[len(subPath)-1] != '/' {
		subPath += "/"
	}
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

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *Driver) Move(sourcePath string, destPath string) error {
	source := d.subPath(sourcePath)
	dest := d.subPath(destPath)

	if _, err := os.Stat(source); os.IsNotExist(err) {
		return storagedriver.PathNotFoundError{Path: sourcePath}
	}

	err := os.Rename(source, dest)
	return err
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *Driver) Delete(subPath string) error {
	fullPath := d.subPath(subPath)

	_, err := os.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err != nil {
		return storagedriver.PathNotFoundError{Path: subPath}
	}

	err = os.RemoveAll(fullPath)
	return err
}

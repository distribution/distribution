package inmemory

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/factory"
)

const driverName = "inmemory"

func init() {
	factory.Register(driverName, &inMemoryDriverFactory{})
}

// inMemoryDriverFacotry implements the factory.StorageDriverFactory interface.
type inMemoryDriverFactory struct{}

func (factory *inMemoryDriverFactory) Create(parameters map[string]string) (storagedriver.StorageDriver, error) {
	return New(), nil
}

// Driver is a storagedriver.StorageDriver implementation backed by a local map.
// Intended solely for example and testing purposes.
type Driver struct {
	storage map[string][]byte
	mutex   sync.RWMutex
}

// New constructs a new Driver.
func New() *Driver {
	return &Driver{storage: make(map[string][]byte)}
}

// Implement the storagedriver.StorageDriver interface.

// GetContent retrieves the content stored at "path" as a []byte.
func (d *Driver) GetContent(path string) ([]byte, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	contents, ok := d.storage[path]
	if !ok {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return contents, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *Driver) PutContent(path string, contents []byte) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.storage[path] = contents
	return nil
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *Driver) ReadStream(path string, offset uint64) (io.ReadCloser, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	contents, err := d.GetContent(path)
	if err != nil {
		return nil, err
	} else if len(contents) <= int(offset) {
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	src := contents[offset:]
	buf := make([]byte, len(src))
	copy(buf, src)
	return ioutil.NopCloser(bytes.NewReader(buf)), nil
}

// WriteStream stores the contents of the provided io.ReadCloser at a location
// designated by the given path.
func (d *Driver) WriteStream(path string, offset, size uint64, reader io.ReadCloser) error {
	defer reader.Close()
	d.mutex.RLock()
	defer d.mutex.RUnlock()

	resumableOffset, err := d.CurrentSize(path)
	if err != nil {
		return err
	}

	if offset > resumableOffset {
		return storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	contents, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}

	if offset > 0 {
		contents = append(d.storage[path][0:offset], contents...)
	}

	d.storage[path] = contents
	return nil
}

// CurrentSize retrieves the curernt size in bytes of the object at the given
// path.
func (d *Driver) CurrentSize(path string) (uint64, error) {
	d.mutex.RLock()
	defer d.mutex.RUnlock()
	contents, ok := d.storage[path]
	if !ok {
		return 0, nil
	}
	return uint64(len(contents)), nil
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (d *Driver) List(path string) ([]string, error) {
	if path[len(path)-1] != '/' {
		path += "/"
	}
	subPathMatcher, err := regexp.Compile(fmt.Sprintf("^%s[^/]+", path))
	if err != nil {
		return nil, err
	}

	d.mutex.RLock()
	defer d.mutex.RUnlock()
	// we use map to collect unique keys
	keySet := make(map[string]struct{})
	for k := range d.storage {
		if key := subPathMatcher.FindString(k); key != "" {
			keySet[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	return keys, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *Driver) Move(sourcePath string, destPath string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	contents, ok := d.storage[sourcePath]
	if !ok {
		return storagedriver.PathNotFoundError{Path: sourcePath}
	}
	d.storage[destPath] = contents
	delete(d.storage, sourcePath)
	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *Driver) Delete(path string) error {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	var subPaths []string
	for k := range d.storage {
		if strings.HasPrefix(k, path) {
			subPaths = append(subPaths, k)
		}
	}

	if len(subPaths) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}

	for _, subPath := range subPaths {
		delete(d.storage, subPath)
	}
	return nil
}

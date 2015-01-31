package storagedriver

import (
	"io"
)

// wrapper wraps the underlying StorageDriver with common operations like
// path checks for each method.
type wrapper struct {
	driver StorageDriver
}

// Wrap creates a wrapper for given storage driver to apply common operations
// on each StorageDriver interface methods.
func Wrap(d StorageDriver) StorageDriver {
	return wrapper{driver: d}
}

// GetContent wraps GetContent of underlying storage driver.
func (d wrapper) GetContent(path string) ([]byte, error) {
	if !PathRegexp.MatchString(path) {
		return nil, InvalidPathError{Path: path}
	}

	return d.driver.GetContent(path)
}

// PutContent wraps PutContent of underlying storage driver.
func (d wrapper) PutContent(path string, content []byte) error {
	if !PathRegexp.MatchString(path) {
		return InvalidPathError{Path: path}
	}

	return d.driver.PutContent(path, content)
}

// ReadStream wraps ReadStream of underlying storage driver.
func (d wrapper) ReadStream(path string, offset int64) (io.ReadCloser, error) {
	if !PathRegexp.MatchString(path) {
		return nil, InvalidPathError{Path: path}
	}

	return d.driver.ReadStream(path, offset)
}

// WriteStream wraps WriteStream of underlying storage driver.
func (d wrapper) WriteStream(path string, offset int64, reader io.Reader) (nn int64, err error) {
	if !PathRegexp.MatchString(path) {
		return 0, InvalidPathError{Path: path}
	}

	return d.driver.WriteStream(path, offset, reader)
}

// Stat wraps Stat of underlying storage driver.
func (d wrapper) Stat(path string) (FileInfo, error) {
	if !PathRegexp.MatchString(path) {
		return nil, InvalidPathError{Path: path}
	}

	return d.driver.Stat(path)
}

// List wraps List of underlying storage driver.
func (d wrapper) List(path string) ([]string, error) {
	if !PathRegexp.MatchString(path) && path != "/" {
		return nil, InvalidPathError{Path: path}
	}

	return d.driver.List(path)
}

// Move wraps Move of underlying storage driver.
func (d wrapper) Move(sourcePath string, destPath string) error {
	if !PathRegexp.MatchString(sourcePath) {
		return InvalidPathError{Path: sourcePath}
	} else if !PathRegexp.MatchString(destPath) {
		return InvalidPathError{Path: destPath}
	}

	return d.driver.Move(sourcePath, destPath)
}

// Delete wraps Delete of underlying storage driver.
func (d wrapper) Delete(path string) error {
	if !PathRegexp.MatchString(path) {
		return InvalidPathError{Path: path}
	}

	return d.driver.Delete(path)
}

// URLFor wraps URLFor of underlying storage driver.
func (d wrapper) URLFor(path string, options map[string]interface{}) (string, error) {
	if !PathRegexp.MatchString(path) {
		return "", InvalidPathError{Path: path}
	}

	return d.driver.URLFor(path, options)
}

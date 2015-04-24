// Package base provides a base implementation of the storage driver that can
// be used to implement common checks. The goal is to increase the amount of
// code sharing.
//
// The canonical approach to use this class is to embed in the exported driver
// struct such that calls are proxied through this implementation. First,
// declare the internal driver, as follows:
//
// 	type driver struct { ... internal ...}
//
// The resulting type should implement StorageDriver such that it can be the
// target of a Base struct. The exported type can then be declared as follows:
//
// 	type Driver struct {
// 		Base
// 	}
//
// Because Driver embeds Base, it effectively implements Base. If the driver
// needs to intercept a call, before going to base, Driver should implement
// that method. Effectively, Driver can intercept calls before coming in and
// driver implements the actual logic.
//
// To further shield the embed from other packages, it is recommended to
// employ a private embed struct:
//
// 	type baseEmbed struct {
// 		base.Base
// 	}
//
// Then, declare driver to embed baseEmbed, rather than Base directly:
//
// 	type Driver struct {
// 		baseEmbed
// 	}
//
// The type now implements StorageDriver, proxying through Base, without
// exporting an unnecessary field.
package base

import (
	"io"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// Base provides a wrapper around a storagedriver implementation that provides
// common path and bounds checking.
type Base struct {
	storagedriver.StorageDriver
}

// GetContent wraps GetContent of underlying storage driver.
func (base *Base) GetContent(path string) ([]byte, error) {
	_, done := context.WithTrace(context.Background())
	defer done("%s.GetContent(%q)", base.Name(), path)

	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.GetContent(path)
}

// PutContent wraps PutContent of underlying storage driver.
func (base *Base) PutContent(path string, content []byte) error {
	_, done := context.WithTrace(context.Background())
	defer done("%s.PutContent(%q)", base.Name(), path)

	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.PutContent(path, content)
}

// ReadStream wraps ReadStream of underlying storage driver.
func (base *Base) ReadStream(path string, offset int64) (io.ReadCloser, error) {
	_, done := context.WithTrace(context.Background())
	defer done("%s.ReadStream(%q, %d)", base.Name(), path, offset)

	if offset < 0 {
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.ReadStream(path, offset)
}

// WriteStream wraps WriteStream of underlying storage driver.
func (base *Base) WriteStream(path string, offset int64, reader io.Reader) (nn int64, err error) {
	_, done := context.WithTrace(context.Background())
	defer done("%s.WriteStream(%q, %d)", base.Name(), path, offset)

	if offset < 0 {
		return 0, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	if !storagedriver.PathRegexp.MatchString(path) {
		return 0, storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.WriteStream(path, offset, reader)
}

// Stat wraps Stat of underlying storage driver.
func (base *Base) Stat(path string) (storagedriver.FileInfo, error) {
	_, done := context.WithTrace(context.Background())
	defer done("%s.Stat(%q)", base.Name(), path)

	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.Stat(path)
}

// List wraps List of underlying storage driver.
func (base *Base) List(path string) ([]string, error) {
	_, done := context.WithTrace(context.Background())
	defer done("%s.List(%q)", base.Name(), path)

	if !storagedriver.PathRegexp.MatchString(path) && path != "/" {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.List(path)
}

// Move wraps Move of underlying storage driver.
func (base *Base) Move(sourcePath string, destPath string) error {
	_, done := context.WithTrace(context.Background())
	defer done("%s.Move(%q, %q", base.Name(), sourcePath, destPath)

	if !storagedriver.PathRegexp.MatchString(sourcePath) {
		return storagedriver.InvalidPathError{Path: sourcePath}
	} else if !storagedriver.PathRegexp.MatchString(destPath) {
		return storagedriver.InvalidPathError{Path: destPath}
	}

	return base.StorageDriver.Move(sourcePath, destPath)
}

// Delete wraps Delete of underlying storage driver.
func (base *Base) Delete(path string) error {
	_, done := context.WithTrace(context.Background())
	defer done("%s.Delete(%q)", base.Name(), path)

	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.Delete(path)
}

// URLFor wraps URLFor of underlying storage driver.
func (base *Base) URLFor(path string, options map[string]interface{}) (string, error) {
	_, done := context.WithTrace(context.Background())
	defer done("%s.URLFor(%q)", base.Name(), path)

	if !storagedriver.PathRegexp.MatchString(path) {
		return "", storagedriver.InvalidPathError{Path: path}
	}

	return base.StorageDriver.URLFor(path, options)
}

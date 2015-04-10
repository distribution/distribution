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
// exporting an unnessecary field.
package base

import (
	"io"
	"time"

	ctxu "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"golang.org/x/net/context"
)

// Base provides a wrapper around a storagedriver implementation that provides
// common path and bounds checking.
type Base struct {
	storagedriver.StorageDriver
}

// durationDebugLog returns a deferrable function which when invoked produces
// debug logging output with the method method name duration.
func durationDebugLog(methodName string) (deferrable func()) {
	startedAt := time.Now()

	return func() {
		ctx := context.WithValue(context.Background(), "duration", time.Since(startedAt))
		ctxu.GetLogger(ctx, "duration").Debug("Storage.Driver." + methodName)
	}
}

// GetContent wraps GetContent of underlying storage driver.
func (base *Base) GetContent(path string) ([]byte, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("GetContent")()

	return base.StorageDriver.GetContent(path)
}

// PutContent wraps PutContent of underlying storage driver.
func (base *Base) PutContent(path string, content []byte) error {
	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("PutContent")()

	return base.StorageDriver.PutContent(path, content)
}

// ReadStream wraps ReadStream of underlying storage driver.
func (base *Base) ReadStream(path string, offset int64) (io.ReadCloser, error) {
	if offset < 0 {
		return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("ReadStream")()

	return base.StorageDriver.ReadStream(path, offset)
}

// WriteStream wraps WriteStream of underlying storage driver.
func (base *Base) WriteStream(path string, offset int64, reader io.Reader) (nn int64, err error) {
	if offset < 0 {
		return 0, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	if !storagedriver.PathRegexp.MatchString(path) {
		return 0, storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("WriteStream")()

	return base.StorageDriver.WriteStream(path, offset, reader)
}

// Stat wraps Stat of underlying storage driver.
func (base *Base) Stat(path string) (storagedriver.FileInfo, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("Stat")()

	return base.StorageDriver.Stat(path)
}

// List wraps List of underlying storage driver.
func (base *Base) List(path string) ([]string, error) {
	if !storagedriver.PathRegexp.MatchString(path) && path != "/" {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("List")()

	return base.StorageDriver.List(path)
}

// Move wraps Move of underlying storage driver.
func (base *Base) Move(sourcePath string, destPath string) error {
	if !storagedriver.PathRegexp.MatchString(sourcePath) {
		return storagedriver.InvalidPathError{Path: sourcePath}
	} else if !storagedriver.PathRegexp.MatchString(destPath) {
		return storagedriver.InvalidPathError{Path: destPath}
	}

	defer durationDebugLog("Move")()

	return base.StorageDriver.Move(sourcePath, destPath)
}

// Delete wraps Delete of underlying storage driver.
func (base *Base) Delete(path string) error {
	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("Delete")()

	return base.StorageDriver.Delete(path)
}

// URLFor wraps URLFor of underlying storage driver.
func (base *Base) URLFor(path string, options map[string]interface{}) (string, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return "", storagedriver.InvalidPathError{Path: path}
	}

	defer durationDebugLog("URLFor")()

	return base.StorageDriver.URLFor(path, options)
}

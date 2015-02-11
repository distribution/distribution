package storage

import (
	"bytes"
	"fmt"
	"io"
	"os"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// fileWriter implements a remote file writer backed by a storage driver.
type fileWriter struct {
	driver storagedriver.StorageDriver

	// identifying fields
	path string

	// mutable fields
	size   int64 // size of the file, aka the current end
	offset int64 // offset is the current write offset
	err    error // terminal error, if set, reader is closed
}

// fileWriterInterface makes the desired io compliant interface that the
// filewriter should implement.
type fileWriterInterface interface {
	io.WriteSeeker
	io.WriterAt
	io.ReaderFrom
	io.Closer
}

var _ fileWriterInterface = &fileWriter{}

// newFileWriter returns a prepared fileWriter for the driver and path. This
// could be considered similar to an "open" call on a regular filesystem.
func newFileWriter(driver storagedriver.StorageDriver, path string) (*fileWriter, error) {
	fw := fileWriter{
		driver: driver,
		path:   path,
	}

	if fi, err := driver.Stat(path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// ignore, offset is zero
		default:
			return nil, err
		}
	} else {
		if fi.IsDir() {
			return nil, fmt.Errorf("cannot write to a directory")
		}

		fw.size = fi.Size()
	}

	return &fw, nil
}

// Write writes the buffer p at the current write offset.
func (fw *fileWriter) Write(p []byte) (n int, err error) {
	nn, err := fw.readFromAt(bytes.NewReader(p), -1)
	return int(nn), err
}

// WriteAt writes p at the specified offset. The underlying offset does not
// change.
func (fw *fileWriter) WriteAt(p []byte, offset int64) (n int, err error) {
	nn, err := fw.readFromAt(bytes.NewReader(p), offset)
	return int(nn), err
}

// ReadFrom reads reader r until io.EOF writing the contents at the current
// offset.
func (fw *fileWriter) ReadFrom(r io.Reader) (n int64, err error) {
	return fw.readFromAt(r, -1)
}

// Seek moves the write position do the requested offest based on the whence
// argument, which can be os.SEEK_CUR, os.SEEK_END, or os.SEEK_SET.
func (fw *fileWriter) Seek(offset int64, whence int) (int64, error) {
	if fw.err != nil {
		return 0, fw.err
	}

	var err error
	newOffset := fw.offset

	switch whence {
	case os.SEEK_CUR:
		newOffset += int64(offset)
	case os.SEEK_END:
		newOffset = fw.size + int64(offset)
	case os.SEEK_SET:
		newOffset = int64(offset)
	}

	if newOffset < 0 {
		err = fmt.Errorf("cannot seek to negative position")
	} else {
		// No problems, set the offset.
		fw.offset = newOffset
	}

	return fw.offset, err
}

// Close closes the fileWriter for writing.
func (fw *fileWriter) Close() error {
	if fw.err != nil {
		return fw.err
	}

	fw.err = fmt.Errorf("filewriter@%v: closed", fw.path)

	return fw.err
}

// readFromAt writes to fw from r at the specified offset. If offset is less
// than zero, the value of fw.offset is used and updated after the operation.
func (fw *fileWriter) readFromAt(r io.Reader, offset int64) (n int64, err error) {
	if fw.err != nil {
		return 0, fw.err
	}

	var updateOffset bool
	if offset < 0 {
		offset = fw.offset
		updateOffset = true
	}

	nn, err := fw.driver.WriteStream(fw.path, offset, r)

	if updateOffset {
		// We should forward the offset, whether or not there was an error.
		// Basically, we keep the filewriter in sync with the reader's head. If an
		// error is encountered, the whole thing should be retried but we proceed
		// from an expected offset, even if the data didn't make it to the
		// backend.
		fw.offset += nn

		if fw.offset > fw.size {
			fw.size = fw.offset
		}
	}

	return nn, err
}

package storagedriver

import (
	"fmt"
	"io"
)

// StorageDriver defines methods that a Storage Driver must implement for a filesystem-like
// key/value object storage
type StorageDriver interface {
	// GetContent retrieves the content stored at "path" as a []byte
	// Should primarily be used for small objects
	GetContent(path string) ([]byte, error)

	// PutContent stores the []byte content at a location designated by "path"
	// Should primarily be used for small objects
	PutContent(path string, content []byte) error

	// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a given byte
	// offset
	// May be used to resume reading a stream by providing a nonzero offset
	ReadStream(path string, offset uint64) (io.ReadCloser, error)

	// WriteStream stores the contents of the provided io.ReadCloser at a location designated by
	// the given path
	// The driver will know it has received the full contents when it has read "size" bytes
	// May be used to resume writing a stream by providing a nonzero offset
	// The offset must be no larger than the ResumeWritePosition for this path
	WriteStream(path string, offset, size uint64, readCloser io.ReadCloser) error

	// ResumeWritePosition retrieves the byte offset at which it is safe to continue writing at the
	// given path
	ResumeWritePosition(path string) (uint64, error)

	// List returns a list of the objects that are direct descendants of the given path
	List(path string) ([]string, error)

	// Move moves an object stored at sourcePath to destPath, removing the original object
	// Note: This may be no more efficient than a copy followed by a delete for many implementations
	Move(sourcePath string, destPath string) error

	// Delete recursively deletes all objects stored at "path" and its subpaths
	Delete(path string) error
}

// PathNotFoundError is returned when operating on a nonexistent path
type PathNotFoundError struct {
	Path string
}

func (err PathNotFoundError) Error() string {
	return fmt.Sprintf("Path not found: %s", err.Path)
}

// InvalidOffsetError is returned when attempting to read or write from an invalid offset
type InvalidOffsetError struct {
	Path   string
	Offset uint64
}

func (err InvalidOffsetError) Error() string {
	return fmt.Sprintf("Invalid offset: %d for path: %s", err.Offset, err.Path)
}

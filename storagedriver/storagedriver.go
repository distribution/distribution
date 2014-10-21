package storagedriver

import (
	"fmt"
	"io"
)

type StorageDriver interface {
	GetContent(path string) ([]byte, error)
	PutContent(path string, content []byte) error
	ReadStream(path string, offset uint64) (io.ReadCloser, error)
	WriteStream(path string, offset, size uint64, readCloser io.ReadCloser) error
	ResumeWritePosition(path string) (uint64, error)
	List(prefix string) ([]string, error)
	Move(sourcePath string, destPath string) error
	Delete(path string) error
}

type PathNotFoundError struct {
	Path string
}

func (err PathNotFoundError) Error() string {
	return fmt.Sprintf("Path not found: %s", err.Path)
}

type InvalidOffsetError struct {
	Path   string
	Offset uint64
}

func (err InvalidOffsetError) Error() string {
	return fmt.Sprintf("Invalid offset: %d for path: %s", err.Offset, err.Path)
}

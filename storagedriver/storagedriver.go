package storagedriver

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Version is a string representing the storage driver version, of the form
// Major.Minor.
// The registry must accept storage drivers with equal major version and greater
// minor version, but may not be compatible with older storage driver versions.
type Version string

// Major returns the major (primary) component of a version.
func (version Version) Major() uint {
	majorPart := strings.Split(string(version), ".")[0]
	major, _ := strconv.ParseUint(majorPart, 10, 0)
	return uint(major)
}

// Minor returns the minor (secondary) component of a version.
func (version Version) Minor() uint {
	minorPart := strings.Split(string(version), ".")[1]
	minor, _ := strconv.ParseUint(minorPart, 10, 0)
	return uint(minor)
}

// CurrentVersion is the current storage driver Version.
const CurrentVersion Version = "0.1"

// StorageDriver defines methods that a Storage Driver must implement for a
// filesystem-like key/value object storage.
type StorageDriver interface {
	// GetContent retrieves the content stored at "path" as a []byte.
	// This should primarily be used for small objects.
	GetContent(path string) ([]byte, error)

	// PutContent stores the []byte content at a location designated by "path".
	// This should primarily be used for small objects.
	PutContent(path string, content []byte) error

	// ReadStream retrieves an io.ReadCloser for the content stored at "path"
	// with a given byte offset.
	// May be used to resume reading a stream by providing a nonzero offset.
	ReadStream(path string, offset int64) (io.ReadCloser, error)

	// WriteStream stores the contents of the provided io.ReadCloser at a
	// location designated by the given path.
	// May be used to resume writing a stream by providing a nonzero offset.
	// The offset must be no larger than the CurrentSize for this path.
	WriteStream(path string, offset int64, reader io.Reader) (nn int64, err error)

	// Stat retrieves the FileInfo for the given path, including the current
	// size in bytes and the creation time.
	Stat(path string) (FileInfo, error)

	// List returns a list of the objects that are direct descendants of the
	//given path.
	List(path string) ([]string, error)

	// Move moves an object stored at sourcePath to destPath, removing the
	// original object.
	// Note: This may be no more efficient than a copy followed by a delete for
	// many implementations.
	Move(sourcePath string, destPath string) error

	// Delete recursively deletes all objects stored at "path" and its subpaths.
	Delete(path string) error
}

// PathComponentRegexp is the regular expression which each repository path
// component must match.
// A component of a repository path must be at least two characters, optionally
// separated by periods, dashes or underscores.
var PathComponentRegexp = regexp.MustCompile(`[a-z0-9]+([._-]?[a-z0-9])+`)

// PathRegexp is the regular expression which each repository path must match.
// A repository path is absolute, beginning with a slash and containing a
// positive number of path components separated by slashes.
var PathRegexp = regexp.MustCompile(`^(/[a-z0-9]+([._-]?[a-z0-9])+)+$`)

// PathNotFoundError is returned when operating on a nonexistent path.
type PathNotFoundError struct {
	Path string
}

func (err PathNotFoundError) Error() string {
	return fmt.Sprintf("Path not found: %s", err.Path)
}

// InvalidPathError is returned when the provided path is malformed.
type InvalidPathError struct {
	Path string
}

func (err InvalidPathError) Error() string {
	return fmt.Sprintf("Invalid path: %s", err.Path)
}

// InvalidOffsetError is returned when attempting to read or write from an
// invalid offset.
type InvalidOffsetError struct {
	Path   string
	Offset int64
}

func (err InvalidOffsetError) Error() string {
	return fmt.Sprintf("Invalid offset: %d for path: %s", err.Offset, err.Path)
}

package estuary

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
)

const driverName = "estuary"

func init() {
	factory.Register(driverName, &estuaryDriverFactory{})
}

// testDriverFactory implements the factory.StorageDriverFactory interface.
type estuaryDriverFactory struct{}

func (factory *estuaryDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return New(), nil
}

// TestDriver is a StorageDriver for testing purposes. The Writer returned by this driver
// simulates the case where Write operations are buffered. This causes the value returned by Size to lag
// behind until Close (or Commit, or Cancel) is called.
type EstuaryDriver struct {
}

var _ storagedriver.StorageDriver = &EstuaryDriver{}

// New constructs a new StorageDriver for testing purposes. The Writer returned by this driver
// simulates the case where Write operations are buffered. This causes the value returned by Size to lag
// behind until Close (or Commit, or Cancel) is called.
func New() *EstuaryDriver {
	return &EstuaryDriver{}
}

////
// Name returns the human-readable "name" of the driver, useful in error
// messages and logging. By convention, this will just be the registration
// name, but drivers may provide other information here.
func (driver *EstuaryDriver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
// This should primarily be used for small objects.
func (driver *EstuaryDriver) GetContent(ctx context.Context, path string) ([]byte, error) {
	fmt.Printf("GetContent, %v", path)
	return nil, nil
}

// PutContent stores the []byte content at a location designated by "path".
// This should primarily be used for small objects.
func (driver *EstuaryDriver) PutContent(ctx context.Context, path string, content []byte) error {
	fmt.Printf("PutContent, %v", path)
	return nil
}

// Reader retrieves an io.ReadCloser for the content stored at "path"
// with a given byte offset.
// May be used to resume reading a stream by providing a nonzero offset.
func (driver *EstuaryDriver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	fmt.Printf("Reader, %v", path)
	return nil, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (driver *EstuaryDriver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	fmt.Printf("Writer, %v", path)
	return nil, nil
}

// Stat retrieves the FileInfo for the given path, including the current
// size in bytes and the creation time.
func (driver *EstuaryDriver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	fmt.Printf("Stat, %v", path)
	return nil, nil
}

// List returns a list of the objects that are direct descendants of the
//given path.
func (driver *EstuaryDriver) List(ctx context.Context, path string) ([]string, error) {
	fmt.Printf("List, %v", path)
	return nil, nil
}

// Move moves an object stored at sourcePath to destPath, removing the
// original object.
// Note: This may be no more efficient than a copy followed by a delete for
// many implementations.
func (driver *EstuaryDriver) Move(ctx context.Context, sourcePath string, destPath string) error {
	fmt.Printf("Move, %v, %v", sourcePath, destPath)
	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (driver *EstuaryDriver) Delete(ctx context.Context, path string) error {
	fmt.Printf("Delete, %v", path)
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at
// the given path, possibly using the given options.
// May return an ErrUnsupportedMethod in certain StorageDriver
// implementations.
func (driver *EstuaryDriver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	fmt.Printf("URLFor, %v", path)
	return path, nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file.
// If the returned error from the WalkFn is ErrSkipDir and fileInfo refers
// to a directory, the directory will not be entered and Walk
// will continue the traversal.  If fileInfo refers to a normal file, processing stops
func (driver *EstuaryDriver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	fmt.Printf("Walk, %v", path)
	return nil
}

///////
type fileWriter struct {
	file      *os.File
	size      int64
	bw        *bufio.Writer
	closed    bool
	committed bool
	cancelled bool
}

func newFileWriter(file *os.File, size int64) *fileWriter {
	return &fileWriter{
		file: file,
		size: size,
		bw:   bufio.NewWriter(file),
	}
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	if fw.closed {
		return 0, fmt.Errorf("already closed")
	} else if fw.committed {
		return 0, fmt.Errorf("already committed")
	} else if fw.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}
	n, err := fw.bw.Write(p)
	fw.size += int64(n)
	return n, err
}

func (fw *fileWriter) Size() int64 {
	return fw.size
}

func (fw *fileWriter) Close() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	if err := fw.bw.Flush(); err != nil {
		return err
	}

	if err := fw.file.Sync(); err != nil {
		return err
	}

	if err := fw.file.Close(); err != nil {
		return err
	}
	fw.closed = true
	return nil
}

func (fw *fileWriter) Cancel() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	fw.cancelled = true
	fw.file.Close()
	return os.Remove(fw.file.Name())
}

func (fw *fileWriter) Commit() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	} else if fw.committed {
		return fmt.Errorf("already committed")
	} else if fw.cancelled {
		return fmt.Errorf("already cancelled")
	}

	if err := fw.bw.Flush(); err != nil {
		return err
	}

	if err := fw.file.Sync(); err != nil {
		return err
	}

	fw.committed = true
	return nil
}

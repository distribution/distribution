package base

import (
	"io"
	"sync"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

type regulator struct {
	storagedriver.StorageDriver
	sync.Cond

	available uint
}

// NewRegulator wraps the given driver and is used to regulate concurrent calls
// to the given storage driver to a maximum of the given limit. This is useful
// for storage drivers that would otherwise create an unbounded number of OS
// threads if allowed to be called unregulated.
func NewRegulator(driver storagedriver.StorageDriver, limit uint) storagedriver.StorageDriver {
	return &regulator{
		StorageDriver: driver,
		Cond: sync.Cond{
			L: &sync.Mutex{},
		},
		available: limit,
	}
}

func (r *regulator) condition() bool {
	return r.available > 0
}

func (r *regulator) enter() {
	r.L.Lock()
	defer r.L.Unlock()

	for !r.condition() {
		r.Wait()
	}

	r.available--
}

func (r *regulator) exit() {
	r.L.Lock()
	defer r.Signal()
	defer r.L.Unlock()

	r.available++
}

// Name returns the human-readable "name" of the driver, useful in error
// messages and logging. By convention, this will just be the registration
// name, but drivers may provide other information here.
func (r *regulator) Name() string {
	r.enter()
	defer r.exit()

	return r.StorageDriver.Name()
}

// GetContent retrieves the content stored at "path" as a []byte.
// This should primarily be used for small objects.
func (r *regulator) GetContent(ctx context.Context, path string) ([]byte, error) {
	r.enter()
	defer r.exit()

	return r.StorageDriver.GetContent(ctx, path)
}

// PutContent stores the []byte content at a location designated by "path".
// This should primarily be used for small objects.
func (r *regulator) PutContent(ctx context.Context, path string, content []byte) error {
	r.enter()
	defer r.exit()

	return r.StorageDriver.PutContent(ctx, path, content)
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path"
// with a given byte offset.
// May be used to resume reading a stream by providing a nonzero offset.
func (r *regulator) ReadStream(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	r.enter()
	defer r.exit()

	return r.StorageDriver.ReadStream(ctx, path, offset)
}

// WriteStream stores the contents of the provided io.ReadCloser at a
// location designated by the given path.
// May be used to resume writing a stream by providing a nonzero offset.
// The offset must be no larger than the CurrentSize for this path.
func (r *regulator) WriteStream(ctx context.Context, path string, offset int64, reader io.Reader) (nn int64, err error) {
	r.enter()
	defer r.exit()

	return r.StorageDriver.WriteStream(ctx, path, offset, reader)
}

// Stat retrieves the FileInfo for the given path, including the current
// size in bytes and the creation time.
func (r *regulator) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	r.enter()
	defer r.exit()

	return r.StorageDriver.Stat(ctx, path)
}

// List returns a list of the objects that are direct descendants of the
//given path.
func (r *regulator) List(ctx context.Context, path string) ([]string, error) {
	r.enter()
	defer r.exit()

	return r.StorageDriver.List(ctx, path)
}

// Move moves an object stored at sourcePath to destPath, removing the
// original object.
// Note: This may be no more efficient than a copy followed by a delete for
// many implementations.
func (r *regulator) Move(ctx context.Context, sourcePath string, destPath string) error {
	r.enter()
	defer r.exit()

	return r.StorageDriver.Move(ctx, sourcePath, destPath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (r *regulator) Delete(ctx context.Context, path string) error {
	r.enter()
	defer r.exit()

	return r.StorageDriver.Delete(ctx, path)
}

// URLFor returns a URL which may be used to retrieve the content stored at
// the given path, possibly using the given options.
// May return an ErrUnsupportedMethod in certain StorageDriver
// implementations.
func (r *regulator) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	r.enter()
	defer r.exit()

	return r.StorageDriver.URLFor(ctx, path, options)
}

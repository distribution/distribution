package containerd

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/namespaces"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	// TODO: make this configurable
	containerdAddress = "/run/containerd/containerd.sock"
	containerdNamespace = "default"
)

// driver implements the storagedriver.StorageDriver interface
type driver struct {
	client *containerd.Client
}

// New creates a new Driver
func New(ctx context.Context, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	client, err := containerd.New(containerdAddress)
	if err != nil {
		return nil, err
	}
	return &driver{client: client}, nil
}

func init() {
	factory.Register("containerd", &containerdDriverFactory{})
}

type containerdDriverFactory struct{}

func (factory *containerdDriverFactory) Create(ctx context.Context, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return New(ctx, parameters)
}

// Implement the storagedriver.StorageDriver interface
func (d *driver) Name() string {
	return "containerd"
}

func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	return nil, storagedriver.ErrUnsupportedMethod
}

func (d *driver) PutContent(ctx context.Context, path string, content []byte) error {
	return storagedriver.ErrUnsupportedMethod
}

func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	if d.client == nil {
		return nil, storagedriver.ErrUnsupportedMethod
	}
	ctx = namespaces.WithNamespace(ctx, containerdNamespace)
	cs := d.client.ContentStore()
	info, err := cs.Info(ctx, path)
	if err != nil {
		// TODO: Convert containerd errors to storagedriver errors
		return nil, err
	}

	ra, err := cs.ReaderAt(ctx, info.Digest)
	if err != nil {
		return nil, err
	}

	return io.NewSectionReader(ra, offset, info.Size-offset), nil
}

// Writer returns a FileWriter which will store the content at the given path.
// The received FileWriter will be responsible for cleaning up any resources
// on error and must be closed when the operation is complete.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	if d.client == nil {
		return nil, storagedriver.ErrUnsupportedMethod
	}
	ctx = namespaces.WithNamespace(ctx, containerdNamespace)
	return newFileWriter(d, path, ctx)
}

type fileWriter struct {
	driver    *driver
	path      string
	tempFile  *os.File // Temporary file to store content
	closed    bool
	committed bool
	cancelled bool
	ctx       context.Context
}

func newFileWriter(d *driver, path string, ctx context.Context) (*fileWriter, error) {
	// Create a temporary file to buffer the write
	tempFile, err := os.CreateTemp("", "containerd-driver-")
	if err != nil {
		return nil, err
	}

	return &fileWriter{
		driver:   d,
		path:     path,
		tempFile: tempFile,
		ctx:      ctx,
	}, nil
}


func (fw *fileWriter) Write(p []byte) (int, error) {
	if fw.closed {
		return 0, errors.New("already closed")
	}
	if fw.cancelled {
		return 0, errors.New("already cancelled")
	}
	return fw.tempFile.Write(p)
}

func (fw *fileWriter) Close() error {
	if fw.closed {
		return errors.New("already closed")
	}
	fw.closed = true

	if err := fw.tempFile.Close(); err != nil {
		return err
	}

	// If not committed or cancelled, treat as cancel
	if !fw.committed && !fw.cancelled {
		fw.Cancel()
	}
	return nil
}


func (fw *fileWriter) Size() int64 {
	fi, err := fw.tempFile.Stat()
	if err != nil {
		return 0 // Or handle error appropriately
	}
	return fi.Size()
}


func (fw *fileWriter) Cancel() error {
	if fw.closed && !fw.committed { // Can only cancel if closed and not committed
		fw.cancelled = true
		os.Remove(fw.tempFile.Name()) // Clean up temp file
		return nil
	}
	return errors.New("cannot cancel committed or open writer")
}

func (fw *fileWriter) Commit() error {
	if fw.closed && fw.cancelled {
		return errors.New("cannot commit a cancelled writer")
	}
	if !fw.closed {
		return errors.New("cannot commit open writer, close first")
	}
	if fw.committed {
		return errors.New("already committed")
	}

	fw.committed = true

	// Open the temp file for reading
	file, err := os.Open(fw.tempFile.Name())
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove(fw.tempFile.Name()) // Clean up temp file after commit

	// TODO: This is a simplified example. In a real scenario, you would
	// interact with the containerd content store here.
	// For example, using client.ContentStore().Writer(...)
	// For now, we'll just simulate a successful commit.

	// Get the descriptor
	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayer, // Or detect based on content
		Size:      fw.Size(),
		Digest:    "", // Calculate digest
	}


	// In a real implementation, you'd use the containerd client to write this.
	// For example:
	cs := fw.driver.client.ContentStore()
	writer, err := content.OpenWriter(fw.ctx, cs, content.WithRef(fw.path), content.WithDescriptor(desc))
	if err != nil {
		return err
	}
	defer writer.Close()
	if _, err := io.Copy(writer, file); err != nil {
		// TODO: Need to call writer.Truncate(0) to clean up?
		return err
	}
	return writer.Commit(fw.ctx, desc.Size, desc.Digest, content.WithLabels(map[string]string{"path": fw.path}))
}

func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	if d.client == nil {
		return nil, storagedriver.ErrUnsupportedMethod
	}
	ctx = namespaces.WithNamespace(ctx, containerdNamespace)
	cs := d.client.ContentStore()

	info, err := cs.Info(ctx, path)
	if err != nil {
		// TODO: Convert containerd error to PathNotFoundError
		return nil, err
	}

	return storagedriver.FileInfoInternal{FileInfoFields: storagedriver.FileInfoFields{
		Path:    path,
		Size:    info.Size,
		ModTime: info.UpdatedAt, // Or CreatedAt?
		IsDir:   false,          // containerd content store doesn't have directories
	}}, nil
}

func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	// TODO: This is tricky as containerd content store is flat.
	// We might need to use labels to simulate a directory structure.
	return nil, storagedriver.ErrUnsupportedMethod
}

func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	// TODO: This might involve copying and deleting, or updating labels.
	return storagedriver.ErrUnsupportedMethod
}

func (d *driver) Delete(ctx context.Context, path string) error {
	if d.client == nil {
		return storagedriver.ErrUnsupportedMethod
	}
	ctx = namespaces.WithNamespace(ctx, containerdNamespace)
	cs := d.client.ContentStore()

	// TODO: Get digest from path if path is not digest
	// For now, assume path is the digest
	err := cs.Delete(ctx, path)
	if err != nil {
		// TODO: Convert containerd error
		return err
	}
	return nil
}

func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod
}

func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.ErrUnsupportedMethod
}

// baseEmbed will be used for common Promote method implementations.
type baseEmbed struct {
	base.Base
}

// StorageDriver is the interface that any storage driver must implement.
// It is embedded in the driver struct to provide default implementations of certain methods.
type StorageDriver struct {
	baseEmbed
}

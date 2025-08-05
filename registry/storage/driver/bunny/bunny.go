// Package bunny provides a storagedriver.StorageDriver implementation to
// store blobs in Bunny.net's Object Storage.
package bunny

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
	bunny "github.com/l0wl3vel/bunny-storage-go-sdk"
)

func init() {
	factory.Register(driverName, &bunnyDriverFactory{})
}

const (
	driverName = "bunny"
)

type bunnyDriverFactory struct{}

func (factory *bunnyDriverFactory) Create(ctx context.Context, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	params, err := NewParameters(parameters)
	if err != nil {
		return nil, err
	}
	return New(ctx, params)
}

func New(ctx context.Context, params *DriverParameters) (storagedriver.StorageDriver, error) {
	client := bunny.NewClient(*params.Hostname.JoinPath(params.StorageZone), params.apiKey)
	return &driver{
		pullZone: params.Pullzone,
		client:   client,
	}, nil
}

var _ storagedriver.StorageDriver = &driver{}

type driver struct {
	pullZone url.URL
	client   bunny.Client
}

// Delete implements driver.StorageDriver.
func (d *driver) Delete(ctx context.Context, path string) error {
	info, err := d.client.Describe(path)
	if err != nil {
		return err
	}
	return d.client.Delete(path, info.IsDirectory)
}

// GetContent implements driver.StorageDriver.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	return d.client.Download(path)
}

// List implements driver.StorageDriver.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	entries, err := d.client.List(path)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, entry := range entries {
		result = append(result, entry.ObjectName)
	}
	return result, nil
}

// Move implements driver.StorageDriver.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	// Bunny Storage does not support moving files directly, so we need to download and re-upload.
	content, err := d.client.Download(sourcePath)
	if err != nil {
		return err
	}
	return d.client.Upload(destPath, content, true)
}

// Name implements driver.StorageDriver.
func (d *driver) Name() string {
	return driverName
}

// PutContent implements driver.StorageDriver.
func (d *driver) PutContent(ctx context.Context, path string, content []byte) error {
	return d.client.Upload(path, content, true)
}

// Reader implements driver.StorageDriver.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	return &bunnyFileReader{
		client: d.client,
		path:   path,
		offset: offset,
	}, nil
}

// RedirectURL implements driver.StorageDriver.
func (d *driver) RedirectURL(r *http.Request, path string) (string, error) {
	return d.pullZone.JoinPath(path).String(), nil
}

// Stat implements driver.StorageDriver.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	info, err := d.client.Describe(path)
	if err != nil {
		return nil, err
	}
	modTime, err := time.Parse(time.RFC3339, info.LastChanged)
	if err != nil {
		return nil, err
	}
	return &storagedriver.FileInfoInternal{
		FileInfoFields: storagedriver.FileInfoFields{
			Path:    info.Path,
			Size:    int64(info.Length),
			IsDir:   info.IsDirectory,
			ModTime: modTime,
		},
	}, nil
}

// Walk implements driver.StorageDriver.
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn, options ...func(*storagedriver.WalkOptions)) error {
	return storagedriver.WalkFallback(ctx, d, path, f, options...)
}

// Writer implements driver.StorageDriver.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	if append {
		// Get the current file as the starting buffer
		content, err := d.client.Download(path)
		if err != nil {
			return nil, err
		}
		return &BunnyFileWriter{
			client: d.client,
			path:   path,
			buffer: content,
		}, nil
	} else {
		// Start with an empty buffer for new files
		return &BunnyFileWriter{
			client: d.client,
			path:   path,
			buffer: []byte{},
		}, nil
	}
}

type bunnyFileReader struct {
	client bunny.Client
	path   string
	offset int64
}

// Close implements io.ReadCloser.
func (b *bunnyFileReader) Close() error {
	return nil
}

// Read implements io.ReadCloser.
func (b *bunnyFileReader) Read(p []byte) (n int, err error) {
	data, err := b.client.DownloadPartial(b.path, b.offset, int64(len(p)))
	if err != nil {
		return 0, err
	}
	n = copy(p, data)
	b.offset += int64(n)
	if n == 0 && err == nil {
		return 0, io.EOF
	}
	return n, nil
}

var _ io.ReadCloser = &bunnyFileReader{}

type BunnyFileWriter struct {
	client      bunny.Client
	path        string
	buffer      []byte
	isCancelled bool
}

// Cancel implements driver.FileWriter.
func (b *BunnyFileWriter) Cancel(context.Context) error {
	b.isCancelled = true
	return nil
}

// Commit implements driver.FileWriter.
func (b *BunnyFileWriter) Commit(context.Context) error {
	return nil
}

// Size implements driver.FileWriter.
func (b *BunnyFileWriter) Size() int64 {
	return int64(len(b.buffer))
}

var _ storagedriver.FileWriter = &BunnyFileWriter{}

func (b *BunnyFileWriter) Write(p []byte) (n int, err error) {
	b.buffer = append(b.buffer, p...)
	return len(p), nil
}

func (b *BunnyFileWriter) Close() error {
	if len(b.buffer) == 0 {
		return nil // Nothing to write
	}
	if b.isCancelled {
		return nil // If cancelled, do not write
	}
	err := b.client.Upload(b.path, b.buffer, true)
	b.buffer = nil // Clear the buffer after writing
	return err
}

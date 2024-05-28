// Package azure provides a storagedriver.StorageDriver implementation to
// store blobs in Microsoft Azure Blob Storage Service.
package azure

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

const (
	driverName   = "azure"
	maxChunkSize = 4 * 1024 * 1024
)

var _ storagedriver.StorageDriver = &driver{}

type driver struct {
	azClient               *azureClient
	client                 *container.Client
	rootDirectory          string
	copyStatusPollMaxRetry int
	copyStatusPollDelay    time.Duration
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by
// Microsoft Azure Blob Storage Service.
type Driver struct {
	baseEmbed
}

func init() {
	factory.Register(driverName, &azureDriverFactory{})
}

type azureDriverFactory struct{}

func (factory *azureDriverFactory) Create(ctx context.Context, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	params, err := NewParameters(parameters)
	if err != nil {
		return nil, err
	}
	return New(ctx, params)
}

// New constructs a new Driver from parameters
func New(ctx context.Context, params *Parameters) (*Driver, error) {
	azClient, err := newAzureClient(params)
	if err != nil {
		return nil, err
	}

	copyStatusPollDelay, err := time.ParseDuration(params.CopyStatusPollDelay)
	if err != nil {
		return nil, err
	}

	client := azClient.ContainerClient()
	d := &driver{
		azClient:               azClient,
		client:                 client,
		rootDirectory:          params.RootDirectory,
		copyStatusPollMaxRetry: params.CopyStatusPollMaxRetry,
		copyStatusPollDelay:    copyStatusPollDelay,
	}
	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		}}, nil
}

// Implement the storagedriver.StorageDriver interface.
func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	// TODO(milosgajdos): should we get a RetryReader here?
	resp, err := d.client.NewBlobClient(d.blobName(path)).DownloadStream(ctx, nil)
	if err != nil {
		if is404(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	// TODO(milosgajdos): this check is not needed as UploadBuffer will return error if we exceed the max blockbytes limit
	// max size for block blobs uploaded via single "Put Blob" for version after "2016-05-31"
	// https://docs.microsoft.com/en-us/rest/api/storageservices/put-blob#remarks
	if len(contents) > blockblob.MaxUploadBlobBytes {
		return fmt.Errorf("content size exceeds max allowed limit (%d): %d", blockblob.MaxUploadBlobBytes, len(contents))
	}

	// Historically, blobs uploaded via PutContent used to be of type AppendBlob
	// (https://github.com/distribution/distribution/pull/1438). We can't replace
	// these blobs atomically via a single "Put Blob" operation without
	// deleting them first. Once we detect they are BlockBlob type, we can
	// overwrite them with an atomically "Put Blob" operation.
	//
	// While we delete the blob and create a new one, there will be a small
	// window of inconsistency and if the Put Blob fails, we may end up with
	// losing the existing data while migrating it to BlockBlob type. However,
	// expectation is the clients pushing will be retrying when they get an error
	// response.
	blobName := d.blobName(path)
	blobRef := d.client.NewBlobClient(blobName)
	props, err := blobRef.GetProperties(ctx, nil)
	if err != nil && !is404(err) {
		return fmt.Errorf("failed to get blob properties: %v", err)
	}
	if err == nil && props.BlobType != nil && *props.BlobType != blob.BlobTypeBlockBlob {
		if _, err := blobRef.Delete(ctx, nil); err != nil {
			return fmt.Errorf("failed to delete legacy blob (%v): %v", *props.BlobType, err)
		}
	}

	// TODO(milosgajdos): should we set some concurrency options on UploadBuffer
	_, err = d.client.NewBlockBlobClient(blobName).UploadBuffer(ctx, contents, nil)
	return err
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	blobRef := d.client.NewBlobClient(d.blobName(path))
	options := blob.DownloadStreamOptions{
		Range: blob.HTTPRange{
			Offset: offset,
		},
	}
	props, err := blobRef.GetProperties(ctx, nil)
	if err != nil {
		if is404(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, fmt.Errorf("failed to get blob properties: %v", err)
	}
	if props.ContentLength == nil {
		return nil, fmt.Errorf("missing ContentLength: %s", path)
	}
	size := *props.ContentLength
	if offset >= size {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}

	// TODO(milosgajdos): should we get a RetryReader here?
	resp, err := blobRef.DownloadStream(ctx, &options)
	if err != nil {
		if is404(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}
	return resp.Body, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, appendMode bool) (storagedriver.FileWriter, error) {
	blobName := d.blobName(path)
	blobRef := d.client.NewBlobClient(blobName)

	props, err := blobRef.GetProperties(ctx, nil)
	blobExists := true
	if err != nil {
		if !is404(err) {
			return nil, err
		}
		blobExists = false
	}

	var size int64
	if blobExists {
		if appendMode {
			if props.ContentLength == nil {
				return nil, fmt.Errorf("missing ContentLength: %s", blobName)
			}
			size = *props.ContentLength
		} else {
			if _, err := blobRef.Delete(ctx, nil); err != nil {
				return nil, err
			}
		}
	} else {
		if appendMode {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		if _, err = d.client.NewAppendBlobClient(blobName).Create(ctx, nil); err != nil {
			return nil, err
		}
	}

	return d.newWriter(ctx, blobName, size), nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	blobName := d.blobName(path)
	blobRef := d.client.NewBlobClient(blobName)
	// Check if the path is a blob
	props, err := blobRef.GetProperties(ctx, nil)
	if err != nil && !is404(err) {
		return nil, err
	}
	if err == nil {
		var missing []string
		if props.ContentLength == nil {
			missing = append(missing, "ContentLength")
		}
		if props.LastModified == nil {
			missing = append(missing, "LastModified")
		}

		if len(missing) > 0 {
			return nil, fmt.Errorf("missing required prroperties (%s) for blob: %s", missing, blobName)
		}
		return storagedriver.FileInfoInternal{
			FileInfoFields: storagedriver.FileInfoFields{
				Path:    path,
				Size:    *props.ContentLength,
				ModTime: *props.LastModified,
				IsDir:   false,
			}}, nil
	}

	// Check if path is a virtual container
	virtContainerPath := blobName
	if !strings.HasSuffix(virtContainerPath, "/") {
		virtContainerPath += "/"
	}

	maxResults := int32(1)
	pager := d.client.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		MaxResults: &maxResults,
		Prefix:     &virtContainerPath,
	})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		if len(resp.Segment.BlobItems) > 0 {
			// path is a virtual container
			return storagedriver.FileInfoInternal{
				FileInfoFields: storagedriver.FileInfoFields{
					Path:  path,
					IsDir: true,
				}}, nil
		}
	}

	// path is not a blob or virtual container
	return nil, storagedriver.PathNotFoundError{Path: path}
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	if path == "/" {
		path = ""
	}

	blobs, err := d.listBlobs(ctx, path)
	if err != nil {
		return blobs, err
	}

	list := directDescendants(blobs, path)
	if path != "" && len(list) == 0 {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return list, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	sourceBlobURL, err := d.signBlobURL(ctx, sourcePath)
	if err != nil {
		return err
	}
	destBlobRef := d.client.NewBlockBlobClient(d.blobName(destPath))
	resp, err := destBlobRef.StartCopyFromURL(ctx, sourceBlobURL, nil)
	if err != nil {
		if is404(err) {
			return storagedriver.PathNotFoundError{Path: sourcePath}
		}
		return err
	}

	copyStatus := *resp.CopyStatus

	if d.copyStatusPollMaxRetry == -1 && copyStatus == blob.CopyStatusTypePending {
		if _, err := destBlobRef.AbortCopyFromURL(ctx, *resp.CopyID, nil); err != nil {
			return err
		}
		return nil
	}

	retryCount := 1
	for copyStatus == blob.CopyStatusTypePending {
		props, err := destBlobRef.GetProperties(ctx, nil)
		if err != nil {
			return err
		}

		if retryCount >= d.copyStatusPollMaxRetry {
			if _, err := destBlobRef.AbortCopyFromURL(ctx, *props.CopyID, nil); err != nil {
				return err
			}
			return fmt.Errorf("max retries for copy polling reached, aborting copy")
		}

		copyStatus = *props.CopyStatus
		if copyStatus == blob.CopyStatusTypeAborted || copyStatus == blob.CopyStatusTypeFailed {
			if props.CopyStatusDescription != nil {
				return fmt.Errorf("failed to move blob: %s", *props.CopyStatusDescription)
			}
			return fmt.Errorf("failed to move blob with copy id %s", *props.CopyID)
		}

		if copyStatus == blob.CopyStatusTypePending {
			time.Sleep(d.copyStatusPollDelay * time.Duration(retryCount))
		}
		retryCount++
	}

	_, err = d.client.NewBlobClient(d.blobName(sourcePath)).Delete(ctx, nil)
	return err
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	blobRef := d.client.NewBlobClient(d.blobName(path))
	_, err := blobRef.Delete(ctx, nil)
	if err == nil {
		// was a blob and deleted, return
		return nil
	} else if !is404(err) {
		return err
	}

	// Not a blob, see if path is a virtual container with blobs
	blobs, err := d.listBlobs(ctx, path)
	if err != nil {
		return err
	}

	for _, b := range blobs {
		blobRef := d.client.NewBlobClient(d.blobName(b))
		if _, err := blobRef.Delete(ctx, nil); err != nil {
			return err
		}
	}

	if len(blobs) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return nil
}

// RedirectURL returns a publicly accessible URL for the blob stored at given path
// for specified duration by making use of Azure Storage Shared Access Signatures (SAS).
// See https://msdn.microsoft.com/en-us/library/azure/ee395415.aspx for more info.
func (d *driver) RedirectURL(req *http.Request, path string) (string, error) {
	return d.signBlobURL(req.Context(), path)
}

func (d *driver) signBlobURL(ctx context.Context, path string) (string, error) {
	expiresTime := time.Now().UTC().Add(20 * time.Minute) // default expiration
	blobName := d.blobName(path)
	blobRef := d.client.NewBlobClient(blobName)
	return d.azClient.SignBlobURL(ctx, blobRef.URL(), expiresTime)
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file and directory
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn, options ...func(*storagedriver.WalkOptions)) error {
	return storagedriver.WalkFallback(ctx, d, path, f, options...)
}

// directDescendants will find direct descendants (blobs or virtual containers)
// of from list of blob paths and will return their full paths. Elements in blobs
// list must be prefixed with a "/" and
//
// Example: direct descendants of "/" in {"/foo", "/bar/1", "/bar/2"} is
// {"/foo", "/bar"} and direct descendants of "bar" is {"/bar/1", "/bar/2"}
func directDescendants(blobs []string, prefix string) []string {
	if !strings.HasPrefix(prefix, "/") { // add trailing '/'
		prefix = "/" + prefix
	}
	if !strings.HasSuffix(prefix, "/") { // containerify the path
		prefix += "/"
	}

	out := make(map[string]bool)
	for _, b := range blobs {
		if strings.HasPrefix(b, prefix) {
			rel := b[len(prefix):]
			c := strings.Count(rel, "/")
			if c == 0 {
				out[b] = true
			} else {
				out[prefix+rel[:strings.Index(rel, "/")]] = true
			}
		}
	}

	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	return keys
}

func (d *driver) listBlobs(ctx context.Context, virtPath string) ([]string, error) {
	if virtPath != "" && !strings.HasSuffix(virtPath, "/") { // containerify the path
		virtPath += "/"
	}

	// we will replace the root directory prefix before returning blob names
	blobPrefix := d.blobName("")

	// This is to cover for the cases when the rootDirectory of the driver is either "" or "/".
	// In those cases, there is no root prefix to replace and we must actually add a "/" to all
	// results in order to keep them as valid paths as recognized by storagedriver.PathRegexp
	prefix := ""
	if blobPrefix == "" {
		prefix = "/"
	}

	out := []string{}

	listPrefix := d.blobName(virtPath)
	pager := d.client.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix: &listPrefix,
	})
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, blob := range resp.Segment.BlobItems {
			if blob.Name == nil {
				return nil, fmt.Errorf("missing blob Name when listing prefix: %s", listPrefix)
			}
			out = append(out, strings.Replace(*blob.Name, blobPrefix, prefix, 1))
		}
	}

	return out, nil
}

func (d *driver) blobName(path string) string {
	// avoid returning an empty blob name.
	// this will happen when rootDirectory is unset, and path == "/",
	// which is what we get from the storage driver health check Stat call.
	if d.rootDirectory == "" && path == "/" {
		return path
	}

	return strings.TrimLeft(strings.TrimRight(d.rootDirectory, "/")+path, "/")
}

// TODO(milosgajdos): consider renaming this func
func is404(err error) bool {
	return bloberror.HasCode(
		err,
		bloberror.BlobNotFound,
		bloberror.ContainerNotFound,
		bloberror.ResourceNotFound,
		bloberror.CannotVerifyCopySource,
	)
}

var _ storagedriver.FileWriter = &writer{}

type writer struct {
	driver    *driver
	path      string
	size      int64
	bw        *bufio.Writer
	closed    bool
	committed bool
	cancelled bool
}

func (d *driver) newWriter(ctx context.Context, path string, size int64) storagedriver.FileWriter {
	return &writer{
		driver: d,
		path:   path,
		size:   size,
		// TODO(milosgajdos): I'm not sure about the maxChunkSize
		bw: bufio.NewWriterSize(&blockWriter{
			ctx:    ctx,
			client: d.client,
			path:   path,
		}, maxChunkSize),
	}
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("already closed")
	} else if w.committed {
		return 0, fmt.Errorf("already committed")
	} else if w.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}

	n, err := w.bw.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *writer) Size() int64 {
	return w.size
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}
	w.closed = true
	return w.bw.Flush()
}

func (w *writer) Cancel(ctx context.Context) error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true
	blobRef := w.driver.client.NewBlobClient(w.path)
	_, err := blobRef.Delete(ctx, nil)
	return err
}

func (w *writer) Commit(ctx context.Context) error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	} else if w.cancelled {
		return fmt.Errorf("already cancelled")
	}
	w.committed = true
	return w.bw.Flush()
}

type blockWriter struct {
	// We construct transient blockWriter objects to encapsulate a write
	// and need to keep the context passed in to the original FileWriter.Write
	ctx    context.Context
	client *container.Client
	path   string
}

func (bw *blockWriter) Write(p []byte) (int, error) {
	blobRef := bw.client.NewAppendBlobClient(bw.path)
	_, err := blobRef.AppendBlock(bw.ctx, streaming.NopCloser(bytes.NewReader(p)), nil)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

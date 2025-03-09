// Package azure provides a storagedriver.StorageDriver implementation to
// store blobs in Microsoft Azure Blob Storage Service.
package azure

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/streaming"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/appendblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

func init() {
	factory.Register(driverName, &azureDriverFactory{})
}

var ErrCorruptedData = errors.New("corrupted data found in the uploaded data")

const (
	driverName   = "azure"
	maxChunkSize = 4 * 1024 * 1024
)

type azureDriverFactory struct{}

func (factory *azureDriverFactory) Create(ctx context.Context, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	params, err := NewParameters(parameters)
	if err != nil {
		return nil, err
	}
	return New(ctx, params)
}

var _ storagedriver.StorageDriver = &driver{}

type driver struct {
	azClient      *azureClient
	client        *container.Client
	rootDirectory string
	maxRetries    int
	retryDelay    time.Duration
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by
// Microsoft Azure Blob Storage Service.
type Driver struct {
	baseEmbed
}

// New constructs a new Driver from parameters
func New(ctx context.Context, params *DriverParameters) (*Driver, error) {
	azClient, err := newClient(params)
	if err != nil {
		return nil, err
	}

	retryDelay, err := time.ParseDuration(params.RetryDelay)
	if err != nil {
		return nil, err
	}

	client := azClient.ContainerClient()
	d := &driver{
		azClient:      azClient,
		client:        client,
		rootDirectory: params.RootDirectory,
		maxRetries:    params.MaxRetries,
		retryDelay:    retryDelay,
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

	// Always create as AppendBlob
	appendBlobRef := d.client.NewAppendBlobClient(blobName)
	if _, err := appendBlobRef.Create(ctx, nil); err != nil {
		return fmt.Errorf("failed to create append blob: %v", err)
	}

	// If we have content, append it
	if len(contents) > 0 {
		// Write in chunks of maxChunkSize otherwise Azure can barf
		// when writing large piece of data in one sot:
		// RESPONSE 413: 413 The uploaded entity blob is too large.
		for offset := 0; offset < len(contents); offset += maxChunkSize {
			end := offset + maxChunkSize
			if end > len(contents) {
				end = len(contents)
			}

			chunk := contents[offset:end]
			_, err := appendBlobRef.AppendBlock(ctx, streaming.NopCloser(bytes.NewReader(chunk)), nil)
			if err != nil {
				return fmt.Errorf("failed to append content: %v", err)
			}
		}
	}

	return nil
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
	eTag := props.ETag

	var size int64
	if blobExists {
		if appendMode {
			if props.ContentLength == nil {
				return nil, fmt.Errorf("missing ContentLength: %s", blobName)
			}
			size = *props.ContentLength
		} else {
			if _, err := blobRef.Delete(ctx, nil); err != nil && !is404(err) {
				return nil, fmt.Errorf("deleting existing blob before write: %w", err)
			}
			res, err := d.client.NewAppendBlobClient(blobName).Create(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("creating new append blob: %w", err)
			}
			eTag = res.ETag
		}
	} else {
		if appendMode {
			return nil, storagedriver.PathNotFoundError{Path: path, DriverName: driverName}
		}
		res, err := d.client.NewAppendBlobClient(blobName).Create(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("creating new append blob: %w", err)
		}
		eTag = res.ETag
	}

	return d.newWriter(ctx, blobName, size, eTag), nil
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
	srcBlobRef := d.client.NewBlobClient(d.blobName(sourcePath))
	sourceBlobURL := srcBlobRef.URL()

	destBlobRef := d.client.NewBlockBlobClient(d.blobName(destPath))
	resp, err := destBlobRef.StartCopyFromURL(ctx, sourceBlobURL, nil)
	if err != nil {
		if is404(err) {
			return storagedriver.PathNotFoundError{Path: sourcePath, DriverName: "azure"}
		}
		return err
	}

	copyStatus := *resp.CopyStatus

	if d.maxRetries == -1 && copyStatus == blob.CopyStatusTypePending {
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

		if retryCount >= d.maxRetries {
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
			time.Sleep(d.retryDelay * time.Duration(retryCount))
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
	size      *atomic.Int64
	bw        *bufio.Writer
	closed    bool
	committed bool
	cancelled bool
}

func (d *driver) newWriter(ctx context.Context, path string, size int64, eTag *azcore.ETag) storagedriver.FileWriter {
	w := &writer{
		driver: d,
		path:   path,
		size:   new(atomic.Int64),
	}
	w.size.Store(size)
	bw := bufio.NewWriterSize(&blockWriter{
		ctx:        ctx,
		client:     d.client,
		path:       path,
		size:       w.size,
		maxRetries: int32(d.maxRetries),
		eTag:       eTag,
	}, maxChunkSize)
	w.bw = bw
	return w
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
	return n, err
}

func (w *writer) Size() int64 {
	return w.size.Load()
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
	client     *container.Client
	path       string
	maxRetries int32
	ctx        context.Context
	size       *atomic.Int64
	eTag       *azcore.ETag
}

func (bw *blockWriter) Write(p []byte) (int, error) {
	appendBlobRef := bw.client.NewAppendBlobClient(bw.path)
	n := 0
	offsetRetryCount := int32(0)

	for n < len(p) {
		appendPos := bw.size.Load()
		chunkSize := min(maxChunkSize, len(p)-n)
		timeoutFromCtx := false
		ctxTimeoutNotify := withTimeoutNotification(bw.ctx, &timeoutFromCtx)

		resp, err := appendBlobRef.AppendBlock(
			ctxTimeoutNotify,
			streaming.NopCloser(bytes.NewReader(p[n:n+chunkSize])),
			&appendblob.AppendBlockOptions{
				AppendPositionAccessConditions: &appendblob.AppendPositionAccessConditions{
					AppendPosition: to.Ptr(appendPos),
				},
				AccessConditions: &blob.AccessConditions{
					ModifiedAccessConditions: &blob.ModifiedAccessConditions{
						IfMatch: bw.eTag,
					},
				},
			},
		)
		if err == nil {
			n += chunkSize // number of bytes uploaded in this call to Write()
			bw.eTag = resp.ETag
			bw.size.Add(int64(chunkSize)) // total size of the blob in the backend
			continue
		}
		appendposFailed := bloberror.HasCode(err, bloberror.AppendPositionConditionNotMet)
		etagFailed := bloberror.HasCode(err, bloberror.ConditionNotMet)
		if !(appendposFailed || etagFailed) || !timeoutFromCtx {
			// Error was not caused by an operation timeout, abort!
			return n, fmt.Errorf("appending blob: %w", err)
		}

		if offsetRetryCount >= bw.maxRetries {
			return n, fmt.Errorf("max number of retries (%d) reached while handling backend operation timeout", bw.maxRetries)
		}

		correctlyUploadedBytes, newEtag, err := bw.chunkUploadVerify(appendPos, p[n:n+chunkSize])
		if err != nil {
			return n, fmt.Errorf("failed handling operation timeout during blob append: %w", err)
		}
		bw.eTag = newEtag
		if correctlyUploadedBytes == 0 {
			offsetRetryCount++
			continue
		}
		offsetRetryCount = 0

		// MD5 is correct, data was uploaded. Let's bump the counters and
		// continue with the upload
		n += int(correctlyUploadedBytes)    // number of bytes uploaded in this call to Write()
		bw.size.Add(correctlyUploadedBytes) // total size of the blob in the backend
	}

	return n, nil
}

// NOTE: this is more or less copy-pasta from the GitLab fix introduced by @vespian
// https://gitlab.com/gitlab-org/container-registry/-/commit/959132477ef719249270b87ce2a7a05abcd6e1ed?merge_request_iid=2059
func (bw *blockWriter) chunkUploadVerify(appendPos int64, chunk []byte) (int64, *azcore.ETag, error) {
	// NOTE(prozlach): We need to see if the chunk uploaded or not. As per
	// the documentation, the operation __might__ have succeeded. There are
	// three options:
	// * chunk did not upload, the file size will be the same as bw.size.
	// In this case we simply need to re-upload the last chunk
	// * chunk or part of it was uploaded - we need to verify the contents
	// of what has been uploaded with MD5 hash and either:
	//   * MD5 is ok - let's continue uploading data starting from the next
	//   chunk
	//   * MD5 is not OK - we have garbadge at the end of the file and
	//   AppendBlock supports only appending, we need to abort and return
	//   permament error to the caller.

	blobRef := bw.client.NewBlobClient(bw.path)
	props, err := blobRef.GetProperties(bw.ctx, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("determining the end of the blob: %v", err)
	}
	if props.ContentLength == nil {
		return 0, nil, fmt.Errorf("ContentLength in blob properties is missing in reply: %v", err)
	}
	reuploadedBytes := *props.ContentLength - appendPos
	if reuploadedBytes == 0 {
		// NOTE(prozlach): This should never happen really and is here only as
		// a precaution in case something changes in the future. The idea is
		// that if the HTTP call did not succed and nothing was uploaded, then
		// this code path is not going to be triggered as there will be no
		// AppendPos condition violation during the retry. OTOH, if the write
		// succeeded even partially, then the reuploadedBytes will be greater
		// than zero.
		return 0, props.ETag, nil
	}

	resp, err := blobRef.DownloadStream(
		bw.ctx,
		&blob.DownloadStreamOptions{
			Range:              blob.HTTPRange{Offset: appendPos, Count: reuploadedBytes},
			RangeGetContentMD5: to.Ptr(true), // we always upload <= 4MiB (i.e the maxChunkSize), so we can try to offload the MD5 calculation to azure
		},
	)
	if err != nil {
		return 0, nil, fmt.Errorf("determining the MD5 of the upload blob chunk: %v", err)
	}
	var uploadedMD5 []byte
	// If upstream makes this extra check, then let's be paranoid too.
	if len(resp.ContentMD5) > 0 {
		uploadedMD5 = resp.ContentMD5
	} else {
		// compute md5
		body := resp.NewRetryReader(bw.ctx, &blob.RetryReaderOptions{MaxRetries: bw.maxRetries})
		h := md5.New() // nolint: gosec // ok for content verification
		_, err = io.Copy(h, body)
		// nolint:errcheck
		defer body.Close()
		if err != nil {
			return 0, nil, fmt.Errorf("calculating the MD5 of the uploaded blob chunk: %v", err)
		}
		uploadedMD5 = h.Sum(nil)
	}

	h := md5.New() // nolint: gosec // ok for content verification
	if _, err = io.Copy(h, bytes.NewReader(chunk)); err != nil {
		return 0, nil, fmt.Errorf("calculating the MD5 of the local blob chunk: %v", err)
	}
	localMD5 := h.Sum(nil)

	if !bytes.Equal(uploadedMD5, localMD5) {
		return 0, nil, fmt.Errorf("verifying contents of the uploaded blob chunk: %v", ErrCorruptedData)
	}

	return reuploadedBytes, resp.ETag, nil
}

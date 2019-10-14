// Package azure provides a storagedriver.StorageDriver implementation to
// store blobs in Microsoft Azure Blob Storage Service.
package azure

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"

	"github.com/Azure/azure-storage-blob-go/azblob"
)

const driverName = "azure"

const (
	paramAccountName = "accountname"
	paramAccountKey  = "accountkey"
	paramContainer   = "container"
	paramRealm       = "realm"
	maxChunkSize     = 4 * 1024 * 1024
)

type driver struct {
	client     azblob.ContainerURL
	credential azblob.StorageAccountCredential
	container  string
}

type baseEmbed struct{ base.Base }

// Driver is a storagedriver.StorageDriver implementation backed by
// Microsoft Azure Blob Storage Service.
type Driver struct{ baseEmbed }

func init() {
	factory.Register(driverName, &azureDriverFactory{})
}

type azureDriverFactory struct{}

func (factory *azureDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

// FromParameters constructs a new Driver with a given parameters map.
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	accountName, ok := parameters[paramAccountName]
	if !ok || fmt.Sprint(accountName) == "" {
		return nil, fmt.Errorf("no %s parameter provided", paramAccountName)
	}

	accountKey, ok := parameters[paramAccountKey]
	if !ok || fmt.Sprint(accountKey) == "" {
		return nil, fmt.Errorf("no %s parameter provided", paramAccountKey)
	}

	container, ok := parameters[paramContainer]
	if !ok || fmt.Sprint(container) == "" {
		return nil, fmt.Errorf("no %s parameter provided", paramContainer)
	}

	realm, ok := parameters[paramRealm]
	if !ok || fmt.Sprint(realm) == "" {
		realm = "core.windows.net"
	}

	return New(fmt.Sprint(accountName), fmt.Sprint(accountKey), fmt.Sprint(container), fmt.Sprint(realm))
}

// New constructs a new Driver with the given Azure Storage Account credentials
func New(accountName, accountKey, container, realm string) (*Driver, error) {
	credential, err := azblob.NewSharedKeyCredential(accountName, accountKey)
	if err != nil {
		return nil, err
	}
	pipeline := azblob.NewPipeline(credential, azblob.PipelineOptions{})
	containerRef := fmt.Sprintf("https://%s.blob.%s/%s", accountName, realm, container)
	containerURL, err := url.Parse(containerRef)
	if err != nil {
		return nil, err
	}
	client := azblob.NewContainerURL(*containerURL, pipeline)

	// Create registry container
	if err := createContainerIfNotExists(context.Background(), client); err != nil {
		return nil, err
	}

	d := &driver{
		client:     client,
		credential: credential,
		container:  container}
	return &Driver{baseEmbed: baseEmbed{Base: base.Base{StorageDriver: d}}}, nil
}

func createContainerIfNotExists(ctx context.Context, containerURL azblob.ContainerURL) error {
	if _, err := containerURL.Create(ctx, nil, azblob.PublicAccessNone); err != nil {
		if err, ok := err.(azblob.StorageError); ok && err.ServiceCode() == azblob.ServiceCodeContainerAlreadyExists {
			return nil
		}
		return err
	}
	return nil
}

// Implement the storagedriver.StorageDriver interface.
func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	blobURL := d.client.NewBlobURL(path)
	resp, err := blobURL.Download(ctx, 0, 0, azblob.BlobAccessConditions{}, false)
	if err != nil {
		if is404(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}
	blob := resp.Body(azblob.RetryReaderOptions{})

	defer blob.Close()
	return ioutil.ReadAll(blob)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	// max size for block blobs uploaded via single "Put Blob" for version after "2016-05-31"
	// https://docs.microsoft.com/en-us/rest/api/storageservices/put-blob#remarks
	const limit = 256 * 1024 * 1024
	if len(contents) > limit {
		return fmt.Errorf("uploading %d bytes with PutContent is not supported; limit: %d bytes", len(contents), limit)
	}

	// Historically, blobs uploaded via PutContent used to be of type AppendBlob
	// (https://github.com/docker/distribution/pull/1438). We can't replace
	// these blobs atomically via a single "Put Blob" operation without
	// deleting them first. Once we detect they are BlockBlob type, we can
	// overwrite them with an atomically "Put Blob" operation.
	//
	// While we delete the blob and create a new one, there will be a small
	// window of inconsistency and if the Put Blob fails, we may end up with
	// losing the existing data while migrating it to BlockBlob type. However,
	// expectation is the clients pushing will be retrying when they get an error
	// response.
	blobURL := d.client.NewBlockBlobURL(path)
	properties, err := blobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
	if err != nil && !is404(err) {
		return fmt.Errorf("failed to get blob properties: %v", err)
	}
	if err == nil {
		if blobType := properties.BlobType(); blobType != azblob.BlobBlockBlob {
			if _, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{}); err != nil {
				return fmt.Errorf("failed to delete legacy blob (%s): %v", blobType, err)
			}
		}
	}

	r := bytes.NewReader(contents)
	_, err = blobURL.Upload(ctx, r, azblob.BlobHTTPHeaders{}, nil, azblob.BlobAccessConditions{})
	return err
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	blobURL := d.client.NewBlobURL(path)
	properties, err := blobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
	if err != nil {
		if is404(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}
	size := properties.ContentLength()
	if offset >= size {
		return ioutil.NopCloser(bytes.NewReader(nil)), nil
	}

	resp, err := blobURL.Download(ctx, offset, 0, azblob.BlobAccessConditions{}, false)
	if err != nil {
		return nil, err
	}
	return resp.Body(azblob.RetryReaderOptions{}), nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	blobURL := d.client.NewAppendBlobURL(path)
	properties, err := blobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
	if err != nil && !is404(err) {
		return nil, err
	}
	blobExists := err == nil

	var size int64
	if blobExists {
		if append {
			size = properties.ContentLength()
		} else {
			_, err = blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{})
			if err != nil {
				return nil, err
			}
		}
	} else {
		if append {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		_, err = blobURL.Create(ctx, azblob.BlobHTTPHeaders{}, nil, azblob.BlobAccessConditions{})
		if err != nil {
			return nil, err
		}
	}

	return d.newWriter(ctx, path, size), nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	blobURL := d.client.NewAppendBlobURL(path)

	// Check if the path is a blob
	properties, err := blobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
	if err != nil && !is404(err) {
		return nil, err
	}
	blobExists := err == nil
	if blobExists {
		return storagedriver.FileInfoInternal{FileInfoFields: storagedriver.FileInfoFields{
			Path:    path,
			Size:    properties.ContentLength(),
			ModTime: properties.LastModified(),
			IsDir:   false,
		}}, nil
	}

	// Check if path is a virtual container
	virtContainerPath := path
	if !strings.HasSuffix(virtContainerPath, "/") {
		virtContainerPath += "/"
	}

	blobs, err := d.client.ListBlobsFlatSegment(ctx, azblob.Marker{}, azblob.ListBlobsSegmentOptions{
		Prefix:     virtContainerPath,
		MaxResults: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(blobs.Segment.BlobItems) > 0 {
		// path is a virtual container
		return storagedriver.FileInfoInternal{FileInfoFields: storagedriver.FileInfoFields{
			Path:  path,
			IsDir: true,
		}}, nil
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
	srcBlobURL := d.client.NewBlobURL(sourcePath)
	destBlobURL := d.client.NewBlobURL(destPath)
	err := copyBlob(ctx, srcBlobURL, destBlobURL)
	if err != nil {
		if is404(err) {
			return storagedriver.PathNotFoundError{Path: sourcePath}
		}
		return err
	}

	_, err = srcBlobURL.Delete(ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{})
	return err
}

func copyBlob(ctx context.Context, src, dst azblob.BlobURL) error {
	resp, err := dst.StartCopyFromURL(ctx, src.URL(), nil, azblob.ModifiedAccessConditions{}, azblob.BlobAccessConditions{})
	if err != nil {
		return err
	}

	return waitForBlobCopy(ctx, dst, resp.CopyID())
}

func waitForBlobCopy(ctx context.Context, blobURL azblob.BlobURL, copyID string) error {
	for {
		properties, err := blobURL.GetProperties(ctx, azblob.BlobAccessConditions{})
		if err != nil {
			return err
		}

		if properties.CopyID() != copyID {
			return errors.New("azure: blob copy id is a mismatch")
		}

		switch properties.CopyStatus() {
		case azblob.CopyStatusSuccess:
			return nil
		case azblob.CopyStatusPending:
			continue
		case azblob.CopyStatusAborted:
			return errors.New("azure: blob copy is aborted")
		case azblob.CopyStatusFailed:
			return fmt.Errorf("azure: blob copy failed. Id=%s Description=%s", copyID, properties.CopyStatusDescription())
		default:
			return fmt.Errorf("azure: unhandled blob copy status: '%s'", properties.CopyStatus())
		}
	}
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	blobURL := d.client.NewBlobURL(path)
	ok, err := deleteBlobIfNotExists(ctx, blobURL)
	if err != nil {
		return err
	}
	if ok {
		return nil // was a blob and deleted, return
	}

	// Not a blob, see if path is a virtual container with blobs
	blobs, err := d.listBlobs(ctx, path)
	if err != nil {
		return err
	}

	for _, b := range blobs {
		blobURL = d.client.NewBlobURL(b)
		if _, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{}); err != nil {
			return err
		}
	}

	if len(blobs) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return nil
}

func deleteBlobIfNotExists(ctx context.Context, blobURL azblob.BlobURL) (bool, error) {
	if _, err := blobURL.Delete(ctx, azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{}); err != nil {
		if err, ok := err.(azblob.StorageError); ok && err.ServiceCode() == azblob.ServiceCodeBlobNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// URLFor returns a publicly accessible URL for the blob stored at given path
// for specified duration by making use of Azure Storage Shared Access Signatures (SAS).
// See https://msdn.microsoft.com/en-us/library/azure/ee395415.aspx for more info.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	expiresTime := time.Now().UTC().Add(20 * time.Minute) // default expiration
	expires, ok := options["expiry"]
	if ok {
		t, ok := expires.(time.Time)
		if ok {
			expiresTime = t
		}
	}
	blobURL := d.client.NewBlobURL(path)
	sasQuery, err := azblob.BlobSASSignatureValues{
		Protocol:      azblob.SASProtocolHTTPS,
		ExpiryTime:    expiresTime,
		ContainerName: d.container,
		BlobName:      path,
		Permissions:   azblob.BlobSASPermissions{Read: true}.String(),
	}.NewSASQueryParameters(d.credential)
	if err != nil {
		return "", err
	}
	sasURL := blobURL.String() + "?" + sasQuery.Encode()
	return sasURL, nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
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

	var keys []string
	for k := range out {
		keys = append(keys, k)
	}
	return keys
}

func (d *driver) listBlobs(ctx context.Context, virtPath string) ([]string, error) {
	if virtPath != "" && !strings.HasSuffix(virtPath, "/") { // containerify the path
		virtPath += "/"
	}

	out := []string{}
	var marker azblob.Marker
	for {
		resp, err := d.client.ListBlobsFlatSegment(ctx, marker, azblob.ListBlobsSegmentOptions{
			Prefix: virtPath,
		})

		if err != nil {
			return out, err
		}

		for _, b := range resp.Segment.BlobItems {
			out = append(out, b.Name)
		}

		if len(resp.Segment.BlobItems) == 0 || !resp.NextMarker.NotDone() {
			break
		}
		marker = resp.NextMarker
	}
	return out, nil
}

func is404(err error) bool {
	storageErr, ok := err.(azblob.StorageError)
	return ok && storageErr.ServiceCode() == azblob.ServiceCodeBlobNotFound
}

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
		bw: bufio.NewWriterSize(&blockWriter{
			context: ctx,
			client:  d.client,
			path:    path,
		}, maxChunkSize),
	}
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, errors.New("already closed")
	} else if w.committed {
		return 0, errors.New("already committed")
	} else if w.cancelled {
		return 0, errors.New("already cancelled")
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
		return errors.New("already closed")
	}
	w.closed = true
	return w.bw.Flush()
}

func (w *writer) Cancel() error {
	if w.closed {
		return errors.New("already closed")
	} else if w.committed {
		return errors.New("already committed")
	}
	w.cancelled = true
	blobURL := w.driver.client.NewBlobURL(w.path)
	// Delete canceled blob even if the context is cancelled
	_, err := blobURL.Delete(context.Background(), azblob.DeleteSnapshotsOptionNone, azblob.BlobAccessConditions{})
	return err
}

func (w *writer) Commit() error {
	if w.closed {
		return errors.New("already closed")
	} else if w.committed {
		return errors.New("already committed")
	} else if w.cancelled {
		return errors.New("already cancelled")
	}
	w.committed = true
	return w.bw.Flush()
}

type blockWriter struct {
	context context.Context
	client  azblob.ContainerURL
	path    string
}

func (bw *blockWriter) Write(p []byte) (int, error) {
	n := 0
	blobURL := bw.client.NewAppendBlobURL(bw.path)
	for offset := 0; offset < len(p); offset += maxChunkSize {
		chunkSize := maxChunkSize
		if offset+chunkSize > len(p) {
			chunkSize = len(p) - offset
		}
		chunk := bytes.NewReader(p[offset : offset+chunkSize])
		_, err := blobURL.AppendBlock(bw.context, chunk, azblob.AppendBlobAccessConditions{}, nil)
		if err != nil {
			return n, err
		}

		n += chunkSize
	}

	return n, nil
}

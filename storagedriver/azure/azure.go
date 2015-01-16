// Package azure provides a storagedriver.StorageDriver implementation to
// store blobs in Microsoft Azure Blob Storage Service.
package azure

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/docker/distribution/storagedriver"
	"github.com/docker/distribution/storagedriver/factory"

	azure "github.com/MSOpenTech/azure-sdk-for-go/clients/storage"
)

const driverName = "azure"

const (
	paramAccountName = "accountname"
	paramAccountKey  = "accountkey"
	paramContainer   = "container"
)

// Driver is a storagedriver.StorageDriver implementation backed by
// Microsoft Azure Blob Storage Service.
type Driver struct {
	client    *azure.BlobStorageClient
	container string
}

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
		return nil, fmt.Errorf("No %s parameter provided", paramAccountName)
	}

	accountKey, ok := parameters[paramAccountKey]
	if !ok || fmt.Sprint(accountKey) == "" {
		return nil, fmt.Errorf("No %s parameter provided", paramAccountKey)
	}

	container, ok := parameters[paramContainer]
	if !ok || fmt.Sprint(container) == "" {
		return nil, fmt.Errorf("No %s parameter provided", paramContainer)
	}

	return New(fmt.Sprint(accountName), fmt.Sprint(accountKey), fmt.Sprint(container))
}

// New constructs a new Driver with the given Azure Storage Account credentials
func New(accountName, accountKey, container string) (*Driver, error) {
	api, err := azure.NewBasicClient(accountName, accountKey)
	if err != nil {
		return nil, err
	}

	blobClient := api.GetBlobService()

	// Create registry container
	if _, err = blobClient.CreateContainerIfNotExists(container, azure.ContainerAccessTypePrivate); err != nil {
		return nil, err
	}

	return &Driver{
		client:    blobClient,
		container: container}, nil
}

// Implement the storagedriver.StorageDriver interface.

// GetContent retrieves the content stored at "path" as a []byte.
func (d *Driver) GetContent(path string) ([]byte, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	blob, err := d.client.GetBlob(d.container, path)
	if err != nil {
		if is404(err) {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}

	return ioutil.ReadAll(blob)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *Driver) PutContent(path string, contents []byte) error {
	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}
	return d.client.PutBlockBlob(d.container, path, ioutil.NopCloser(bytes.NewReader(contents)))
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) ReadStream(path string, offset int64) (io.ReadCloser, error) {
	if ok, err := d.client.BlobExists(d.container, path); err != nil {
		return nil, err
	} else if !ok {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	info, err := d.client.GetBlobProperties(d.container, path)
	if err != nil {
		return nil, err
	}

	size := int64(info.ContentLength)
	if offset >= size {
		return ioutil.NopCloser(bytes.NewReader(nil)), nil
	}

	bytesRange := fmt.Sprintf("%v-", offset)
	resp, err := d.client.GetBlobRange(d.container, path, bytesRange)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// WriteStream stores the contents of the provided io.ReadCloser at a location
// designated by the given path.
func (d *Driver) WriteStream(path string, offset int64, reader io.Reader) (int64, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return 0, storagedriver.InvalidPathError{Path: path}
	}

	var (
		lastBlockNum    int
		resumableOffset int64
		blocks          []azure.Block
	)

	if blobExists, err := d.client.BlobExists(d.container, path); err != nil {
		return 0, err
	} else if !blobExists { // new blob
		lastBlockNum = 0
		resumableOffset = 0
	} else { // append
		if parts, err := d.client.GetBlockList(d.container, path, azure.BlockListTypeCommitted); err != nil {
			return 0, err
		} else if len(parts.CommittedBlocks) == 0 {
			lastBlockNum = 0
			resumableOffset = 0
		} else {
			lastBlock := parts.CommittedBlocks[len(parts.CommittedBlocks)-1]
			if lastBlockNum, err = fromBlockID(lastBlock.Name); err != nil {
				return 0, fmt.Errorf("Cannot parse block name as number '%s': %s", lastBlock.Name, err.Error())
			}

			var totalSize int64
			for _, v := range parts.CommittedBlocks {
				blocks = append(blocks, azure.Block{
					Id:     v.Name,
					Status: azure.BlockStatusCommitted})
				totalSize += int64(v.Size)
			}

			// NOTE: Azure driver currently supports only append mode (resumable
			// index is exactly where the committed blocks of the blob end).
			// In order to support writing to offsets other than last index,
			// adjacent blocks overlapping with the [offset:offset+size] area
			// must be fetched, splitted and should be overwritten accordingly.
			// As the current use of this method is append only, that implementation
			// is omitted.
			resumableOffset = totalSize
		}
	}

	if offset < resumableOffset {
		// only writing at the end or after the end of the file is supported
		return 0, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	} else if offset > resumableOffset {
		// zero-fill in between, construct a multi-reader
		zeroReader := bytes.NewReader(make([]byte, offset-resumableOffset))
		reader = io.MultiReader(zeroReader, reader)
	}

	// Put content
	var nn int64
	buf := make([]byte, azure.MaxBlobBlockSize)
	for {
		// Read chunks of exactly size N except the last chunk to
		// maximize block size and minimize block count.
		n, err := io.ReadFull(reader, buf)
		if err == io.EOF {
			break
		}
		nn += int64(n)

		data := buf[:n]
		lastBlockNum++
		blockID := toBlockID(lastBlockNum)
		if err = d.client.PutBlock(d.container, path, blockID, data); err != nil {
			return 0, err
		}

		blocks = append(blocks, azure.Block{
			Id:     blockID,
			Status: azure.BlockStatusLatest})
	}

	// If there was a zero-fill, adjust nn to exclude zeros
	if offset > resumableOffset {
		nn -= offset - resumableOffset
	}

	// Commit block list
	return nn, d.client.PutBlockList(d.container, path, blocks)
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *Driver) Stat(path string) (storagedriver.FileInfo, error) {
	if !storagedriver.PathRegexp.MatchString(path) {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	// Check if the path is a blob
	if ok, err := d.client.BlobExists(d.container, path); err != nil {
		return nil, err
	} else if ok {
		blob, err := d.client.GetBlobProperties(d.container, path)
		if err != nil {
			return nil, err
		}

		mtim, err := time.Parse(http.TimeFormat, blob.LastModified)
		if err != nil {
			return nil, err
		}

		return storagedriver.FileInfoInternal{FileInfoFields: storagedriver.FileInfoFields{
			Path:    path,
			Size:    int64(blob.ContentLength),
			ModTime: mtim,
			IsDir:   false,
		}}, nil
	}

	// Check if path is a virtual container
	virtContainerPath := path
	if !strings.HasSuffix(virtContainerPath, "/") {
		virtContainerPath += "/"
	}
	blobs, err := d.client.ListBlobs(d.container, azure.ListBlobsParameters{
		Prefix:     virtContainerPath,
		MaxResults: 1,
	})
	if err != nil {
		return nil, err
	}
	if len(blobs.Blobs) > 0 {
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
func (d *Driver) List(path string) ([]string, error) {
	if !storagedriver.PathRegexp.MatchString(path) && path != "/" {
		return nil, storagedriver.InvalidPathError{Path: path}
	}

	if path == "/" {
		path = ""
	}

	blobs, err := d.listBlobs(d.container, path)
	if err != nil {
		return blobs, err
	}

	list := directDescendants(blobs, path)
	return list, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *Driver) Move(sourcePath string, destPath string) error {
	if !storagedriver.PathRegexp.MatchString(sourcePath) {
		return storagedriver.InvalidPathError{Path: sourcePath}
	} else if !storagedriver.PathRegexp.MatchString(destPath) {
		return storagedriver.InvalidPathError{Path: destPath}
	}

	sourceBlobURL := d.client.GetBlobUrl(d.container, sourcePath)
	err := d.client.CopyBlob(d.container, destPath, sourceBlobURL)
	if err != nil {
		if is404(err) {
			return storagedriver.PathNotFoundError{Path: sourcePath}
		}
		return err
	}

	return d.client.DeleteBlob(d.container, sourcePath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *Driver) Delete(path string) error {
	if !storagedriver.PathRegexp.MatchString(path) {
		return storagedriver.InvalidPathError{Path: path}
	}

	ok, err := d.client.DeleteBlobIfExists(d.container, path)
	if err != nil {
		return err
	}
	if ok {
		return nil // was a blob and deleted, return
	}

	// Not a blob, see if path is a virtual container with blobs
	blobs, err := d.listBlobs(d.container, path)
	if err != nil {
		return err
	}

	for _, b := range blobs {
		if err = d.client.DeleteBlob(d.container, b); err != nil {
			return err
		}
	}

	if len(blobs) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return nil
}

// URLFor returns a publicly accessible URL for the blob stored at given path
// for specified duration by making use of Azure Storage Shared Access Signatures (SAS).
// See https://msdn.microsoft.com/en-us/library/azure/ee395415.aspx for more info.
func (d *driver) URLFor(path string, options map[string]interface{}) (string, error) {
	expiresTime := time.Now().UTC().Add(20 * time.Minute) // default expiration
	expires, ok := options["expiry"]
	if ok {
		t, ok := expires.(time.Time)
		if ok {
			expiresTime = t
		}
	}
	return d.client.GetBlobSASURI(d.container, path, expiresTime, "r")
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

func (d *Driver) listBlobs(container, virtPath string) ([]string, error) {
	if virtPath != "" && !strings.HasSuffix(virtPath, "/") { // containerify the path
		virtPath += "/"
	}

	out := []string{}
	marker := ""
	for {
		resp, err := d.client.ListBlobs(d.container, azure.ListBlobsParameters{
			Marker: marker,
			Prefix: virtPath,
		})

		if err != nil {
			return out, err
		}

		for _, b := range resp.Blobs {
			out = append(out, b.Name)
		}

		if len(resp.Blobs) == 0 || resp.NextMarker == "" {
			break
		}
		marker = resp.NextMarker
	}
	return out, nil
}

func is404(err error) bool {
	e, ok := err.(azure.StorageServiceError)
	return ok && e.StatusCode == 404
}

func fromBlockID(b64Name string) (int, error) {
	s, err := base64.StdEncoding.DecodeString(b64Name)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(string(s))
}

func toBlockID(i int) string {
	s := fmt.Sprintf("%010d", i) // add zero padding
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// Package bos provides a storagedriver.StorageDriver implementation to
// store blobs in Baidu BOS cloud storage.
//
// This package leverages the guoyao/baidubce-sdk-go client library for interfacing with
// bos.
//
// Because BOS is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
//
// +build include_bos

package bos

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"

	"github.com/guoyao/baidubce-sdk-go/bce"
	"github.com/guoyao/baidubce-sdk-go/bos"
)

const driverName = "bos"

// minChunkSize defines the minimum multipart upload chunk size
// BOS API requires multipart upload chunks to be at least 5MB
const minChunkSize = 5 << 20

const defaultChunkSize = 2 * minChunkSize
const defaultTimeout = 2 * time.Minute // 2 minute timeout per chunk

// listMax is the largest amount of objects you can request from BOS in a list call
const listMax = 1000

//DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
	Region          string
	Secure          bool
	ChunkSize       int64
	Endpoint        string
	RootDirectory   string
	Debug           bool
}

func init() {
	factory.Register(driverName, &bosDriverFactory{})
}

// bosDriverFactory implements the factory.StorageDriverFactory interface
type bosDriverFactory struct{}

func (factory *bosDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	Client        *bos.Client
	Bucket        string
	ChunkSize     int64
	RootDirectory string
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Baidu BOS
// Objects are stored at absolute keys in the provided bucket.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
// Required parameters:
// - accesskey
// - secretkey
// - region
// - bucket
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	// Providing no values for these is valid in case the user is authenticating

	accessKey, ok := parameters["accesskeyid"]
	if !ok {
		return nil, fmt.Errorf("No accesskeyid parameter provided")
	}

	secretKey, ok := parameters["accesskeysecret"]
	if !ok {
		return nil, fmt.Errorf("No accesskeysecret parameter provided")
	}

	regionName, ok := parameters["region"]
	if !ok || fmt.Sprint(regionName) == "" {
		return nil, fmt.Errorf("No region parameter provided")
	}

	bucket, ok := parameters["bucket"]
	if !ok || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
	}

	secureBool := true
	secure, ok := parameters["secure"]
	if ok {
		secureBool, ok = secure.(bool)
		if !ok {
			return nil, fmt.Errorf("The secure parameter should be a boolean")
		}
	}

	chunkSize := int64(defaultChunkSize)
	chunkSizeParam, ok := parameters["chunksize"]
	if ok {
		switch v := chunkSizeParam.(type) {
		case string:
			vv, err := strconv.ParseInt(v, 0, 64)
			if err != nil {
				return nil, fmt.Errorf("chunksize parameter must be an integer, %v invalid", chunkSizeParam)
			}
			chunkSize = vv
		case int64:
			chunkSize = v
		case int, uint, int32, uint32, uint64:
			chunkSize = reflect.ValueOf(v).Convert(reflect.TypeOf(chunkSize)).Int()
		default:
			return nil, fmt.Errorf("invalid valud for chunksize: %#v", chunkSizeParam)
		}

		if chunkSize < minChunkSize {
			return nil, fmt.Errorf("The chunksize %#v parameter should be a number that is larger than or equal to %d", chunkSize, minChunkSize)
		}
	}

	rootDirectory, ok := parameters["rootdirectory"]
	if !ok {
		rootDirectory = ""
	}

	endpoint, ok := parameters["endpoint"]
	if !ok {
		endpoint = ""
	}

	debugBool := false
	debug, ok := parameters["debug"]
	if ok {
		debugBool, ok = debug.(bool)
		if !ok {
			return nil, fmt.Errorf("The debug parameter should be a boolean")
		}
	}

	params := DriverParameters{
		AccessKeyID:     fmt.Sprint(accessKey),
		AccessKeySecret: fmt.Sprint(secretKey),
		Bucket:          fmt.Sprint(bucket),
		Region:          fmt.Sprint(regionName),
		ChunkSize:       chunkSize,
		Endpoint:        fmt.Sprint(endpoint),
		RootDirectory:   fmt.Sprint(rootDirectory),
		Secure:          secureBool,
		Debug:           debugBool,
	}

	return New(params)
}

// New constructs a new Driver with the given Baidubce credentials, region, encryption flag, and
// bucketName
func New(params DriverParameters) (*Driver, error) {
	credentials := bce.NewCredentials(params.AccessKeyID, params.AccessKeySecret)
	config := &bce.Config{
		Credentials: credentials,
		Region:      params.Region,
		Endpoint:    params.Endpoint,
		Checksum:    true,
		UserAgent:   strings.Join([]string{bce.DefaultUserAgent, "docker-registry"}, "/"),
	}

	if params.Secure {
		config.Protocol = "https"
	}

	client := bos.NewClient(bos.NewConfig(config))

	if params.Debug {
		client.SetDebug(true)
	}

	listObjectsRequest := bos.ListObjectsRequest{
		BucketName: params.Bucket,
		Prefix:     params.RootDirectory,
	}

	// Validate that the given credentials have at least read permissions in the
	// given bucket scope.
	if _, err := client.ListObjectsFromRequest(listObjectsRequest, nil); err != nil {
		return nil, err
	}

	d := &driver{
		Client:        client,
		Bucket:        params.Bucket,
		ChunkSize:     params.ChunkSize,
		RootDirectory: params.RootDirectory,
	}

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}, nil
}

// Implement the storagedriver.StorageDriver interface

func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, path string) ([]byte, error) {
	reader, err := d.Reader(ctx, path, 0)

	if err != nil {
		return nil, err
	}

	return ioutil.ReadAll(reader)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	_, err := d.Client.PutObject(d.Bucket, d.bcePath(path), contents, nil, nil)

	return parseError(path, err)
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	getObjectRequest := bos.GetObjectRequest{
		BucketName: d.Bucket,
		ObjectKey:  d.bcePath(path),
		Range:      strconv.FormatInt(offset, 10) + "-",
	}

	object, err := d.Client.GetObjectFromRequest(getObjectRequest, nil)

	if err != nil {
		if bceError, ok := err.(*bce.Error); ok && bceError.Code == "InvalidRange" {
			return ioutil.NopCloser(bytes.NewReader(nil)), nil
		}

		return nil, parseError(path, err)
	}

	return object.ObjectContent, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	key := d.bcePath(path)

	if !append {
		request := bos.InitiateMultipartUploadRequest{
			BucketName: d.Bucket,
			ObjectKey:  key,
		}

		// TODO (brianbland): cancel other uploads at this path
		initiateMultipartUploadResponse, err := d.Client.InitiateMultipartUpload(request, nil)

		if err != nil {
			return nil, err
		}

		return d.newWriter(key, initiateMultipartUploadResponse.UploadId, nil), nil
	}

	listMultipartUploadsResponse, err := d.Client.ListMultipartUploadsFromRequest(bos.ListMultipartUploadsRequest{
		BucketName: d.Bucket,
		Prefix:     key,
	}, nil)

	if err != nil {
		return nil, parseError(path, err)
	}

	for _, multi := range listMultipartUploadsResponse.Uploads {
		if key != multi.Key {
			continue
		}

		listPartsResponse, err := d.Client.ListParts(d.Bucket, key, multi.UploadId, nil)

		if err != nil {
			return nil, parseError(path, err)
		}

		var multiSize int64

		for _, part := range listPartsResponse.Parts {
			multiSize += part.Size
		}

		return d.newWriter(key, multi.UploadId, listPartsResponse.Parts), nil
	}

	return nil, storagedriver.PathNotFoundError{Path: path}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	key := d.bcePath(path)

	listObjectsRequest := bos.ListObjectsRequest{
		BucketName: d.Bucket,
		//Delimiter:  "/",
		Prefix:  key,
		MaxKeys: 1,
	}

	listObjectsResponse, err := d.Client.ListObjectsFromRequest(listObjectsRequest, nil)

	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(listObjectsResponse.Contents) == 1 {
		if listObjectsResponse.Contents[0].Key != key {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = listObjectsResponse.Contents[0].Size

			//fi.ModTime = listObjectsResponse.Contents[0].LastModified
			timestamp, err := time.Parse(time.RFC3339Nano, listObjectsResponse.Contents[0].LastModified)

			if err != nil {
				return nil, err
			}

			fi.ModTime = timestamp
		}
	} else if len(listObjectsResponse.CommonPrefixes) == 1 {
		fi.IsDir = true
	} else {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, opath string) ([]string, error) {
	path := opath
	if path != "/" && opath[len(path)-1] != '/' {
		path = path + "/"
	}

	// This is to cover for the cases when the rootDirectory of the driver is either "" or "/".
	// In those cases, there is no root prefix to replace and we must actually add a "/" to all
	// results in order to keep them as valid paths as recognized by storagedriver.PathRegexp
	prefix := ""
	if d.bcePath("") == "" {
		prefix = "/"
	}

	listObjectsRequest := bos.ListObjectsRequest{
		BucketName: d.Bucket,
		Delimiter:  "/",
		Prefix:     d.bcePath(path),
		MaxKeys:    listMax,
	}

	listObjectsResponse, err := d.Client.ListObjectsFromRequest(listObjectsRequest, nil)

	if err != nil {
		return nil, parseError(opath, err)
	}

	files := []string{}
	directories := []string{}

	for {
		for _, objectSummary := range listObjectsResponse.Contents {
			files = append(files, strings.Replace(objectSummary.Key, d.bcePath(""), prefix, 1))
		}

		for _, commonPrefix := range listObjectsResponse.GetCommonPrefixes() {
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.bcePath(""), prefix, 1))
		}

		if listObjectsResponse.IsTruncated {
			listObjectsRequest = bos.ListObjectsRequest{
				BucketName: d.Bucket,
				Delimiter:  "/",
				Prefix:     d.bcePath(path),
				Marker:     listObjectsResponse.NextMarker,
				MaxKeys:    listMax,
			}

			listObjectsResponse, err = d.Client.ListObjectsFromRequest(listObjectsRequest, nil)

			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}

	if opath != "/" {
		if len(files) == 0 && len(directories) == 0 {
			// Treat empty response as missing directory, since we don't actually
			// have directories in BOS.
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	srcKey := d.bcePath(sourcePath)
	destKey := d.bcePath(destPath)

	logrus.Infof("Move from %s to %s", srcKey, destKey)

	_, err := d.Client.CopyObject(d.Bucket, srcKey, d.Bucket, destKey, nil)

	if err != nil {
		logrus.Errorf("Failed for move from %s to %s: %v", srcKey, destKey, err)

		return parseError(sourcePath, err)
	}

	return d.Delete(ctx, sourcePath)
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	keys := make([]string, 0, listMax)
	listObjectsRequest := bos.ListObjectsRequest{
		BucketName: d.Bucket,
		Prefix:     d.bcePath(path),
		MaxKeys:    listMax,
	}

	for {
		// list all the objects
		resp, err := d.Client.ListObjectsFromRequest(listObjectsRequest, nil)

		// resp.Contents can only be empty on the first call
		// if there were no more results to return after the first call, resp.IsTruncated would have been false
		// and the loop would be exited without recalling ListObjects
		if err != nil || len(resp.Contents) == 0 {
			return storagedriver.PathNotFoundError{Path: path}
		}

		for _, objectSummary := range resp.Contents {
			keys = append(keys, objectSummary.Key)
		}

		// resp.Contents must have at least one element or we would have returned not found
		listObjectsRequest.Marker = resp.Contents[len(resp.Contents)-1].Key

		// from the s3 api docs, IsTruncated "specifies whether (true) or not (false) all of the results were returned"
		// if everything has been returned, break
		if !resp.IsTruncated {
			break
		}
	}

	// need to chunk objects into groups of 1000 per s3 restrictions
	total := len(keys)

	for i := 0; i < total; i += 1000 {
		_, err := d.Client.DeleteMultipleObjects(d.Bucket, keys[i:min(i+1000, total)], nil)

		if err != nil {
			return err
		}
	}

	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	methodString := "GET"
	method, ok := options["method"]

	if ok {
		methodString, ok = method.(string)

		if !ok || (methodString != "GET") {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	now := time.Now()
	expiresTime := now.Add(20 * time.Minute)
	expires, ok := options["expiry"]

	if ok {
		et, ok := expires.(time.Time)

		if ok {
			expiresTime = et
		}
	}

	logrus.Infof("methodString: %s, expiresTime: %v", methodString, expiresTime)

	signOption := &bce.SignOption{
		ExpirationPeriodInSeconds: int(expiresTime.Sub(now).Seconds()),
	}
	signedURL, err := d.Client.GeneratePresignedUrl(d.Bucket, d.bcePath(path), signOption)

	if err != nil {
		return "", err
	}

	logrus.Infof("signed URL: %s", signedURL)

	return signedURL, nil
}

func (d *driver) bcePath(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

func parseError(path string, err error) error {
	//if bceError, ok := err.(*bce.Error); ok && bceError.StatusCode == http.StatusNotFound && (bceError.Code == "NoSuchKey" || bceError.Code == "") {
	if bceError, ok := err.(*bce.Error); ok && bceError.Code == "NoSuchKey" {
		return storagedriver.PathNotFoundError{Path: path}
	}

	return err
}

// writer attempts to upload parts to BOS in a buffered fashion where the last
// part is at least as large as the chunksize, so the multipart upload could be
// cleanly resumed in the future. This is violated if Close is called after less
// than a full chunk is written.
type writer struct {
	driver      *driver
	key         string
	uploadId    string
	parts       []bos.PartSummary
	size        int64
	readyPart   []byte
	pendingPart []byte
	closed      bool
	committed   bool
	cancelled   bool
}

func (d *driver) newWriter(key, uploadId string, parts []bos.PartSummary) storagedriver.FileWriter {
	var size int64

	for _, part := range parts {
		size += part.Size
	}

	return &writer{
		driver:   d,
		key:      key,
		uploadId: uploadId,
		parts:    parts,
		size:     size,
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

	var err error

	// If the last written part is smaller than minChunkSize, we need to make a
	// new multipart upload :sadface:
	if len(w.parts) > 0 && int(w.parts[len(w.parts)-1].Size) < minChunkSize {
		_, err = w.driver.Client.CompleteMultipartUpload(bos.CompleteMultipartUploadRequest{
			BucketName: w.driver.Bucket,
			ObjectKey:  w.key,
			UploadId:   w.uploadId,
			Parts:      w.parts,
		}, nil)

		if err != nil {
			w.driver.Client.AbortMultipartUpload(bos.AbortMultipartUploadRequest{
				BucketName: w.driver.Bucket,
				ObjectKey:  w.key,
				UploadId:   w.uploadId,
			}, nil)

			return 0, err
		}

		resp, err := w.driver.Client.InitiateMultipartUpload(bos.InitiateMultipartUploadRequest{
			BucketName: w.driver.Bucket,
			ObjectKey:  w.key,
		}, nil)

		if err != nil {
			return 0, err
		}

		w.uploadId = resp.UploadId
		object, err := w.driver.Client.GetObject(w.driver.Bucket, w.key, nil)
		defer object.ObjectContent.Close()

		if err != nil {
			return 0, err
		}

		// If the entire written file is smaller than minChunkSize, we need to make
		// a new part from scratch :double sad face:
		if w.size < minChunkSize {
			w.parts = nil
			w.readyPart, err = ioutil.ReadAll(object.ObjectContent)

			if err != nil {
				return 0, err
			}
		} else {
			// Otherwise we can use the old file as the new first part
			uploadPartResponse, err := w.driver.Client.UploadPart(bos.UploadPartRequest{
				BucketName: w.driver.Bucket,
				ObjectKey:  w.key,
				UploadId:   resp.UploadId,
				PartSize:   object.ObjectMetadata.ContentLength,
				PartNumber: 1,
				PartData:   object.ObjectContent,
			}, nil)

			if err != nil {
				return 0, err
			}

			w.parts = []bos.PartSummary{
				bos.PartSummary{
					ETag:       uploadPartResponse.GetETag(),
					PartNumber: 1,
				},
			}
		}
	}

	var n int

	for len(p) > 0 {
		// If no parts are ready to write, fill up the first part
		if neededBytes := int(w.driver.ChunkSize) - len(w.readyPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.readyPart = append(w.readyPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]
			} else {
				w.readyPart = append(w.readyPart, p...)
				n += len(p)
				p = nil
			}
		}

		if neededBytes := int(w.driver.ChunkSize) - len(w.pendingPart); neededBytes > 0 {
			if len(p) >= neededBytes {
				w.pendingPart = append(w.pendingPart, p[:neededBytes]...)
				n += neededBytes
				p = p[neededBytes:]

				err = w.flushPart()

				if err != nil {
					w.size += int64(n)
					return n, err
				}
			} else {
				w.pendingPart = append(w.pendingPart, p...)
				n += len(p)
				p = nil
			}
		}
	}

	w.size += int64(n)

	return n, nil
}

func (w *writer) Size() int64 {
	return w.size
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}

	w.closed = true

	return w.flushPart()
}

func (w *writer) Cancel() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}

	w.cancelled = true

	request := bos.AbortMultipartUploadRequest{
		BucketName: w.driver.Bucket,
		ObjectKey:  w.key,
		UploadId:   w.uploadId,
	}

	err := w.driver.Client.AbortMultipartUpload(request, nil)

	return err
}

func (w *writer) Commit() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	} else if w.cancelled {
		return fmt.Errorf("already cancelled")
	}

	err := w.flushPart()

	if err != nil {
		return err
	}

	w.committed = true

	request := bos.CompleteMultipartUploadRequest{
		BucketName: w.driver.Bucket,
		ObjectKey:  w.key,
		UploadId:   w.uploadId,
		Parts:      w.parts,
	}

	_, err = w.driver.Client.CompleteMultipartUpload(request, nil)

	if err != nil {
		abortRequest := bos.AbortMultipartUploadRequest{
			BucketName: w.driver.Bucket,
			ObjectKey:  w.key,
			UploadId:   w.uploadId,
		}

		w.driver.Client.AbortMultipartUpload(abortRequest, nil)

		return err
	}

	return nil
}

// flushPart flushes buffers to write a part to BOS.
// Only called by Write (with both buffers full) and Close/Commit (always)
func (w *writer) flushPart() error {
	if len(w.readyPart) == 0 && len(w.pendingPart) == 0 {
		// nothing to write
		return nil
	}

	if len(w.pendingPart) < int(w.driver.ChunkSize) {
		// closing with a small pending part
		// combine ready and pending to avoid writing a small part
		w.readyPart = append(w.readyPart, w.pendingPart...)
		w.pendingPart = nil
	}

	partNumber := len(w.parts) + 1
	partSize := int64(len(w.readyPart))

	uploadPartRequest := bos.UploadPartRequest{
		BucketName: w.driver.Bucket,
		ObjectKey:  w.key,
		UploadId:   w.uploadId,
		PartSize:   partSize,
		PartNumber: partNumber,
		PartData:   bytes.NewReader(w.readyPart),
	}

	uploadPartResponse, err := w.driver.Client.UploadPart(uploadPartRequest, nil)

	if err != nil {
		return err
	}

	w.parts = append(w.parts, bos.PartSummary{
		PartNumber: partNumber,
		Size:       partSize,
		ETag:       uploadPartResponse.GetETag(),
	})
	w.readyPart = w.pendingPart
	w.pendingPart = nil

	return nil
}

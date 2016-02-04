// Package gcs provides a storagedriver.StorageDriver implementation to
// store blobs in Google cloud storage.
//
// This package leverages the google.golang.org/cloud/storage client library
//for interfacing with gcs.
//
// Because gcs is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
//
// Note that the contents of incomplete uploads are not accessible even though
// Stat returns their length
//
// +build include_gcs

package gcs

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/googleapi"
	"google.golang.org/cloud"
	"google.golang.org/cloud/storage"

	ctx "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const (
	driverName     = "gcs"
	dummyProjectID = "<unknown>"

	uploadSessionContentType = "application/x-docker-upload-session"
	minChunkSize             = 256 * 1024
	maxChunkSize             = 20 * minChunkSize
)

var rangeHeader = regexp.MustCompile(`^bytes=([0-9])+-([0-9]+)$`)

type uploadSession struct {
	sessionURI   string // URI of the upload session, which can be used to append chunks of data with PUT operations
	uploadedSize int64  // Number of bytes successfully uploaded to the session URI
	buffer       []byte // PUT operations on the session URI must happen in multiples of 256KB, this buffer
	// collects the remaining (< 256KB) bytes that need to be uploaded at a later point
}

// driverParameters is a struct that encapsulates all of the driver parameters after all values have been set
type driverParameters struct {
	bucket        string
	config        *jwt.Config
	email         string
	privateKey    []byte
	client        *http.Client
	rootDirectory string
}

func init() {
	factory.Register(driverName, &gcsDriverFactory{})
}

// gcsDriverFactory implements the factory.StorageDriverFactory interface
type gcsDriverFactory struct{}

// Create StorageDriver from parameters
func (factory *gcsDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

// driver is a storagedriver.StorageDriver implementation backed by GCS
// Objects are stored at absolute keys in the provided bucket.
type driver struct {
	client        *http.Client
	bucket        string
	email         string
	privateKey    []byte
	rootDirectory string
}

// FromParameters constructs a new Driver with a given parameters map
// Required parameters:
// - bucket
func FromParameters(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	bucket, ok := parameters["bucket"]
	if !ok || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
	}

	rootDirectory, ok := parameters["rootdirectory"]
	if !ok {
		rootDirectory = ""
	}

	var ts oauth2.TokenSource
	jwtConf := new(jwt.Config)
	if keyfile, ok := parameters["keyfile"]; ok {
		jsonKey, err := ioutil.ReadFile(fmt.Sprint(keyfile))
		if err != nil {
			return nil, err
		}
		jwtConf, err = google.JWTConfigFromJSON(jsonKey, storage.ScopeFullControl)
		if err != nil {
			return nil, err
		}
		ts = jwtConf.TokenSource(context.Background())
	} else {
		var err error
		ts, err = google.DefaultTokenSource(context.Background(), storage.ScopeFullControl)
		if err != nil {
			return nil, err
		}

	}

	params := driverParameters{
		bucket:        fmt.Sprint(bucket),
		rootDirectory: fmt.Sprint(rootDirectory),
		email:         jwtConf.Email,
		privateKey:    jwtConf.PrivateKey,
		client:        oauth2.NewClient(context.Background(), ts),
	}

	return New(params)
}

// New constructs a new driver
func New(params driverParameters) (storagedriver.StorageDriver, error) {
	rootDirectory := strings.Trim(params.rootDirectory, "/")
	if rootDirectory != "" {
		rootDirectory += "/"
	}
	d := &driver{
		bucket:        params.bucket,
		rootDirectory: rootDirectory,
		email:         params.email,
		privateKey:    params.privateKey,
		client:        params.client,
	}

	return &base.Base{
		StorageDriver: d,
	}, nil
}

// Implement the storagedriver.StorageDriver interface

func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
// This should primarily be used for small objects.
func (d *driver) GetContent(context ctx.Context, path string) ([]byte, error) {
	rc, err := d.ReadStream(context, path, 0)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	p, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// PutContent stores the []byte content at a location designated by "path".
// This should primarily be used for small objects.
func (d *driver) PutContent(context ctx.Context, path string, contents []byte) error {
	wc := storage.NewWriter(d.context(context), d.bucket, d.pathToKey(path))
	wc.ContentType = "application/octet-stream"
	defer wc.Close()
	_, err := wc.Write(contents)
	return err
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path"
// with a given byte offset.
// May be used to resume reading a stream by providing a nonzero offset.
func (d *driver) ReadStream(context ctx.Context, path string, offset int64) (io.ReadCloser, error) {
	res, err := d.getObject(context, path, offset)
	if err != nil {
		if res != nil && res.StatusCode == http.StatusRequestedRangeNotSatisfiable {
			res.Body.Close()
			obj, err := storageStatObject(d.context(context), d.bucket, d.pathToKey(path))
			if err != nil {
				return nil, err
			}
			if offset == int64(obj.Size) {
				return ioutil.NopCloser(bytes.NewReader([]byte{})), nil
			}
			return nil, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
		}
		return nil, err
	}
	if res.Header.Get("Content-Type") == uploadSessionContentType {
		defer res.Body.Close()
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return res.Body, nil
}

func (d *driver) getObject(context ctx.Context, path string, offset int64) (*http.Response, error) {
	// copied from google.golang.org/cloud/storage#NewReader :
	// to set the additional "Range" header
	u := &url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   fmt.Sprintf("/%s/%s", d.bucket, d.pathToKey(path)),
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%v-", offset))
	}
	var res *http.Response
	err = retry(5, func() error {
		var err error
		res, err = d.client.Do(req)
		return err
	})
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return res, googleapi.CheckMediaResponse(res)
}

// WriteStream stores the contents of the provided io.ReadCloser at a
// location designated by the given path.
// May be used to resume writing a stream by providing a nonzero offset.
// The offset must be no larger than the CurrentSize for this path.
func (d *driver) WriteStream(context ctx.Context, path string, offset int64, reader io.Reader) (totalWritten int64, err error) {
	if offset < 0 {
		return 0, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	var s *uploadSession
	// start new upload session of offset 0
	if offset == 0 {
		s = &uploadSession{}
		// retrieve existing upload session for resuming
	} else {
		s, err = d.getUploadSession(context, path)
		if err != nil {
			return 0, err
		}
	}

	// offset should not be greater than the size of the file to avoid creating 'gaps'
	if offset > s.length() {
		return 0, storagedriver.InvalidOffsetError{Path: path, Offset: offset}
	}

	// in case offset is less than the current file size, their difference must be skipped
	// the skipped bytes must be reported to the caller as if they were written
	skipped := s.length() - offset
	if skipped > 0 {
		nn, err := io.Copy(ioutil.Discard, io.LimitReader(reader, skipped))
		if err != nil || nn < skipped {
			return nn, err
		}
	}

	// write buffer
	buffer := make([]byte, maxChunkSize)
	// current size of the write buffer
	var buffSize int
	// the number of bytes written by this call to WriteStream
	var written int64

	// copy bytes from the upload session into the buffer
	copy(buffer, s.buffer)
	buffSize += len(s.buffer)

	for more := true; more; {
		// fill the buffer by reading bytes from the reader
		n, err := reader.Read(buffer[buffSize:])
		if err == io.EOF {
			err = nil
			more = false
		}
		buffSize += n

		// amount of bytes that got succesfully written
		var chunkWritten int
		// chunks can be uploaded only in multiples of minChunkSize
		// chunkSize is a multiple of minChunkSize less than or equal to buffSize
		if chunkSize := buffSize - (buffSize % minChunkSize); chunkSize > 0 {
			var err2 error
			// if their is no sessionURI yet, obtain one by starting the session
			if s.sessionURI == "" {
				s.sessionURI, err2 = startSession(d.client, d.bucket, d.pathToKey(path))
			}
			if err2 != nil {
				chunkWritten = 0
			} else {
				var nn int64
				nn, err2 = putChunk(d.client, s.sessionURI, buffer[0:chunkSize], s.uploadedSize, -1)
				chunkWritten = int(nn)
				s.uploadedSize += nn
				written += nn
			}
			// set err to the latest error, if there was no earlier one
			if err2 != nil && err == nil {
				err = err2
			}
		}
		// shift the remaining bytes to the start of the buffer
		copy(buffer, buffer[chunkWritten:buffSize])
		buffSize -= chunkWritten

		// do not continue looping if there was an error while reading/writing
		if err != nil {
			more = false
		}
	}

	previouslyWritten := len(s.buffer)

	// Copy the remaining bytes from the buffer to the upload session
	// Normally buffSize will be smaller than minChunkSize. However, in the
	// unlikely event that the upload session failed to start, this number could be higher.
	// In this case we can safely clip the remaining bytes to the minChunkSize
	if buffSize > minChunkSize {
		buffSize = minChunkSize
	}
	s.buffer = buffer[0:buffSize]
	written += int64(buffSize)

	// commit the writes by updating the upload session
	err2 := d.updateUploadSession(context, path, s)
	if err2 != nil {
		// failed to update the upload session, inform the caller that nothing has been written
		return skipped, err2
	}
	// calculate total bytes written: subtract 'previouslyWritten' to avoid double counting,
	// and pretend that skipped bytes were really written by this WriteStream call
	return written + skipped - int64(previouslyWritten), err
}

func (d *driver) CloseWriteStream(context ctx.Context, path string) error {
	s, err := d.getUploadSession(context, path)
	if err != nil {
		return err
	}
	// no session started yet just perform a simple upload
	if s.sessionURI == "" {
		return d.PutContent(context, path, s.buffer)
	}
	_, err = putChunk(d.client, s.sessionURI, s.buffer, s.uploadedSize, s.length())
	return err
}

type request func() error

func retry(maxTries int, req request) error {
	backoff := time.Second
	var err error
	for i := 0; i < maxTries; i++ {
		err = req()
		if err == nil {
			return nil
		}

		status, ok := err.(*googleapi.Error)
		if !ok || (status.Code != 429 && status.Code < http.StatusInternalServerError) {
			return err
		}

		time.Sleep(backoff - time.Second + (time.Duration(rand.Int31n(1000)) * time.Millisecond))
		if i <= 4 {
			backoff = backoff * 2
		}
	}
	return err
}

func (d *driver) getUploadSession(context ctx.Context, path string) (*uploadSession, error) {
	res, err := d.getObject(context, path, 0)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.Header.Get("Content-Type") != uploadSessionContentType {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	offset, err := strconv.ParseInt(res.Header.Get("X-Goog-Meta-Offset"), 10, 64)
	if err != nil {
		return nil, err
	}
	buffer, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return &uploadSession{
		sessionURI:   res.Header.Get("X-Goog-Meta-Session-URI"),
		buffer:       buffer,
		uploadedSize: offset,
	}, nil
}

func (d *driver) updateUploadSession(context ctx.Context, path string, s *uploadSession) error {
	wc := storage.NewWriter(d.context(context), d.bucket, d.pathToKey(path))
	wc.ContentType = uploadSessionContentType
	wc.Metadata = map[string]string{
		"Session-URI": s.sessionURI,
		"Offset":      strconv.FormatInt(s.uploadedSize, 10),
	}
	defer wc.Close()
	_, err := wc.Write(s.buffer)
	return err
}

// Stat retrieves the FileInfo for the given path, including the current
// size in bytes and the creation time.
func (d *driver) Stat(context ctx.Context, path string) (storagedriver.FileInfo, error) {

	var fi storagedriver.FileInfoFields
	//try to get as file
	gcsContext := d.context(context)
	obj, err := storageStatObject(gcsContext, d.bucket, d.pathToKey(path))
	if err == nil {
		size := obj.Size
		if obj.ContentType == uploadSessionContentType {
			offset, err := strconv.ParseInt(obj.Metadata["Offset"], 0, 64)
			if err != nil {
				return nil, err
			}
			size = size + offset
		}
		fi = storagedriver.FileInfoFields{
			Path:    path,
			Size:    size,
			ModTime: obj.Updated,
			IsDir:   false,
		}
		return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
	}
	//try to get as folder
	dirpath := d.pathToDirKey(path)

	var query *storage.Query
	query = &storage.Query{}
	query.Prefix = dirpath
	query.MaxResults = 1

	objects, err := storageListObjects(gcsContext, d.bucket, query)
	if err != nil {
		return nil, err
	}
	if len(objects.Results) < 1 {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	fi = storagedriver.FileInfoFields{
		Path:  path,
		IsDir: true,
	}
	obj = objects.Results[0]
	if obj.Name == dirpath {
		fi.Size = obj.Size
		fi.ModTime = obj.Updated
	}
	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the
//given path.
func (d *driver) List(context ctx.Context, path string) ([]string, error) {
	var query *storage.Query
	query = &storage.Query{}
	query.Delimiter = "/"
	query.Prefix = d.pathToDirKey(path)
	list := make([]string, 0, 64)
	for {
		objects, err := storageListObjects(d.context(context), d.bucket, query)
		if err != nil {
			return nil, err
		}
		for _, object := range objects.Results {
			// GCS does not guarantee strong consistency between
			// DELETE and LIST operationsCheck that the object is not deleted,
			// so filter out any objects with a non-zero time-deleted
			if object.Deleted.IsZero() {
				list = append(list, d.keyToPath(object.Name))
			}
		}
		for _, subpath := range objects.Prefixes {
			subpath = d.keyToPath(subpath)
			list = append(list, subpath)
		}
		query = objects.Next
		if query == nil {
			break
		}
	}
	if path != "/" && len(list) == 0 {
		// Treat empty response as missing directory, since we don't actually
		// have directories in Google Cloud Storage.
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return list, nil
}

// Move moves an object stored at sourcePath to destPath, removing the
// original object.
func (d *driver) Move(context ctx.Context, sourcePath string, destPath string) error {
	prefix := d.pathToDirKey(sourcePath)
	gcsContext := d.context(context)
	keys, err := d.listAll(gcsContext, prefix)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		destPrefix := d.pathToDirKey(destPath)
		copies := make([]string, 0, len(keys))
		sort.Strings(keys)
		var err error
		for _, key := range keys {
			dest := destPrefix + key[len(prefix):]
			_, err = storageCopyObject(gcsContext, d.bucket, key, d.bucket, dest, nil)
			if err == nil {
				copies = append(copies, dest)
			} else {
				break
			}
		}
		// if an error occurred, attempt to cleanup the copies made
		if err != nil {
			for i := len(copies) - 1; i >= 0; i-- {
				_ = storageDeleteObject(gcsContext, d.bucket, copies[i])
			}
			return err
		}
		// delete originals
		for i := len(keys) - 1; i >= 0; i-- {
			err2 := storageDeleteObject(gcsContext, d.bucket, keys[i])
			if err2 != nil {
				err = err2
			}
		}
		return err
	}
	_, err = storageCopyObject(gcsContext, d.bucket, d.pathToKey(sourcePath), d.bucket, d.pathToKey(destPath), nil)
	if err != nil {
		if status, ok := err.(*googleapi.Error); ok {
			if status.Code == http.StatusNotFound {
				return storagedriver.PathNotFoundError{Path: sourcePath}
			}
		}
		return err
	}
	return storageDeleteObject(gcsContext, d.bucket, d.pathToKey(sourcePath))
}

// listAll recursively lists all names of objects stored at "prefix" and its subpaths.
func (d *driver) listAll(context context.Context, prefix string) ([]string, error) {
	list := make([]string, 0, 64)
	query := &storage.Query{}
	query.Prefix = prefix
	query.Versions = false
	for {
		objects, err := storageListObjects(d.context(context), d.bucket, query)
		if err != nil {
			return nil, err
		}
		for _, obj := range objects.Results {
			// GCS does not guarantee strong consistency between
			// DELETE and LIST operationsCheck that the object is not deleted,
			// so filter out any objects with a non-zero time-deleted
			if obj.Deleted.IsZero() {
				list = append(list, obj.Name)
			}
		}
		query = objects.Next
		if query == nil {
			break
		}
	}
	return list, nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(context ctx.Context, path string) error {
	prefix := d.pathToDirKey(path)
	gcsContext := d.context(context)
	keys, err := d.listAll(gcsContext, prefix)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		sort.Sort(sort.Reverse(sort.StringSlice(keys)))
		for _, key := range keys {
			err := storageDeleteObject(gcsContext, d.bucket, key)
			// GCS only guarantees eventual consistency, so listAll might return
			// paths that no longer exist. If this happens, just ignore any not
			// found error
			if status, ok := err.(*googleapi.Error); ok {
				if status.Code == http.StatusNotFound {
					err = nil
				}
			}
			if err != nil {
				return err
			}
		}
		return nil
	}
	err = storageDeleteObject(gcsContext, d.bucket, d.pathToKey(path))
	if err != nil {
		if status, ok := err.(*googleapi.Error); ok {
			if status.Code == http.StatusNotFound {
				return storagedriver.PathNotFoundError{Path: path}
			}
		}
	}
	return err
}

func storageDeleteObject(context context.Context, bucket string, name string) error {
	return retry(5, func() error {
		return storage.DeleteObject(context, bucket, name)
	})
}

func storageStatObject(context context.Context, bucket string, name string) (*storage.Object, error) {
	var obj *storage.Object
	err := retry(5, func() error {
		var err error
		obj, err = storage.StatObject(context, bucket, name)
		return err
	})
	return obj, err
}

func storageListObjects(context context.Context, bucket string, q *storage.Query) (*storage.Objects, error) {
	var objs *storage.Objects
	err := retry(5, func() error {
		var err error
		objs, err = storage.ListObjects(context, bucket, q)
		return err
	})
	return objs, err
}

func storageCopyObject(context context.Context, srcBucket, srcName string, destBucket, destName string, attrs *storage.ObjectAttrs) (*storage.Object, error) {
	var obj *storage.Object
	err := retry(5, func() error {
		var err error
		obj, err = storage.CopyObject(context, srcBucket, srcName, destBucket, destName, attrs)
		return err
	})
	return obj, err
}

// URLFor returns a URL which may be used to retrieve the content stored at
// the given path, possibly using the given options.
// Returns ErrUnsupportedMethod if this driver has no privateKey
func (d *driver) URLFor(context ctx.Context, path string, options map[string]interface{}) (string, error) {
	if d.privateKey == nil {
		return "", storagedriver.ErrUnsupportedMethod{}
	}

	name := d.pathToKey(path)
	methodString := "GET"
	method, ok := options["method"]
	if ok {
		methodString, ok = method.(string)
		if !ok || (methodString != "GET" && methodString != "HEAD") {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	expiresTime := time.Now().Add(20 * time.Minute)
	expires, ok := options["expiry"]
	if ok {
		et, ok := expires.(time.Time)
		if ok {
			expiresTime = et
		}
	}

	opts := &storage.SignedURLOptions{
		GoogleAccessID: d.email,
		PrivateKey:     d.privateKey,
		Method:         methodString,
		Expires:        expiresTime,
	}
	return storage.SignedURL(d.bucket, name, opts)
}

func (s *uploadSession) length() int64 {
	return s.uploadedSize + int64(len(s.buffer))
}

func startSession(client *http.Client, bucket string, name string) (uri string, err error) {
	u := &url.URL{
		Scheme:   "https",
		Host:     "www.googleapis.com",
		Path:     fmt.Sprintf("/upload/storage/v1/b/%v/o", bucket),
		RawQuery: fmt.Sprintf("uploadType=resumable&name=%v", name),
	}
	err = retry(5, func() error {
		req, err := http.NewRequest("POST", u.String(), nil)
		if err != nil {
			return err
		}
		req.Header.Set("X-Upload-Content-Type", "application/octet-stream")
		req.Header.Set("Content-Length", "0")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		err = googleapi.CheckMediaResponse(resp)
		if err != nil {
			return err
		}
		uri = resp.Header.Get("Location")
		return nil
	})
	return uri, err
}

func putChunk(client *http.Client, sessionURI string, chunk []byte, from int64, totalSize int64) (int64, error) {
	bytesPut := int64(0)
	err := retry(5, func() error {
		req, err := http.NewRequest("PUT", sessionURI, bytes.NewReader(chunk))
		if err != nil {
			return err
		}
		length := int64(len(chunk))
		to := from + length - 1
		size := "*"
		if totalSize >= 0 {
			size = strconv.FormatInt(totalSize, 10)
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		if from == to+1 {
			req.Header.Set("Content-Range", fmt.Sprintf("bytes */%v", size))
		} else {
			req.Header.Set("Content-Range", fmt.Sprintf("bytes %v-%v/%v", from, to, size))
		}
		req.Header.Set("Content-Length", strconv.FormatInt(length, 10))

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if totalSize < 0 && resp.StatusCode == 308 {
			groups := rangeHeader.FindStringSubmatch(resp.Header.Get("Range"))
			end, err := strconv.ParseInt(groups[2], 10, 64)
			if err != nil {
				return err
			}
			bytesPut = end - from + 1
			return nil
		}
		err = googleapi.CheckMediaResponse(resp)
		if err != nil {
			return err
		}
		bytesPut = to - from + 1
		return nil
	})
	return bytesPut, err
}

func (d *driver) context(context ctx.Context) context.Context {
	return cloud.WithContext(context, dummyProjectID, d.client)
}

func (d *driver) pathToKey(path string) string {
	return strings.TrimRight(d.rootDirectory+strings.TrimLeft(path, "/"), "/")
}

func (d *driver) pathToDirKey(path string) string {
	return d.pathToKey(path) + "/"
}

func (d *driver) keyToPath(key string) string {
	return "/" + strings.Trim(strings.TrimPrefix(key, d.rootDirectory), "/")
}

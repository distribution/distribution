// Package ks3 provides a storagedriver.StorageDriver implementation to
// store blobs in Kingsoft Cloud KS3 storage.
//
// This package leverages the softlns/ks3 client library for interfacing with
// KS3.
//
// Because KS3 is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
//
// +build include_ks3

package ks3

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/softlns/ks3-sdk-go/ks3"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const driverName = "ks3"

// minChunkSize defines the minimum multipart upload chunk size
// KS3 API requires multipart upload chunks to be at least 5MB
const minChunkSize = 5 << 20

const defaultChunkSize = 2 * minChunkSize

// listMax is the largest amount of objects you can request from KS3 in a list call
const listMax = 1000

//DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	AccessKey      string
	SecretKey      string
	Bucket         string
	RegionName     string
	Internal       bool
	Encrypt        bool
	Secure         bool
	ChunkSize      int64
	RootDirectory  string
	RegionEndpoint string
}

func init() {
	factory.Register(driverName, &ks3DriverFactory{})
}

// ks3DriverFactory implements the factory.StorageDriverFactory interface
type ks3DriverFactory struct{}

func (factory *ks3DriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	KS3           *ks3.KS3
	Bucket        *ks3.Bucket
	ChunkSize     int64
	Encrypt       bool
	RootDirectory string
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Kingsoft Cloud KS3
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
// - encrypt
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	// Providing no values for these is valid in case the user is authenticating

	accessKey := parameters["accesskey"]
	if accessKey == nil {
		accessKey = ""
	}

	secretKey := parameters["secretkey"]
	if secretKey == nil {
		secretKey = ""
	}

	regionName, ok := parameters["region"]
	if !ok || fmt.Sprint(regionName) == "" {
		return nil, fmt.Errorf("No region parameter provided")
	}

	bucket, ok := parameters["bucket"]
	if !ok || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
	}

	internalBool := false
	internal, ok := parameters["internal"]
	if ok {
		internalBool, ok = internal.(bool)
		if !ok {
			return nil, fmt.Errorf("The internal parameter should be a boolean")
		}
	}

	encryptBool := false
	encrypt, ok := parameters["encrypt"]
	if ok {
		encryptBool, ok = encrypt.(bool)
		if !ok {
			return nil, fmt.Errorf("The encrypt parameter should be a boolean")
		}
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
	chunkSizeParam := parameters["chunksize"]
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
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("invalid value for chunksize: %#v", chunkSizeParam)
	}

	if chunkSize < minChunkSize {
		return nil, fmt.Errorf("The chunksize %#v parameter should be a number that is larger than or equal to %d", chunkSize, minChunkSize)
	}

	rootDirectory := parameters["rootdirectory"]
	if rootDirectory == nil {
		rootDirectory = ""
	}

	regionEndpoint := parameters["regionendpoint"]
	if regionEndpoint == nil {
		regionEndpoint = ""
	}

	params := DriverParameters{
		fmt.Sprint(accessKey),
		fmt.Sprint(secretKey),
		fmt.Sprint(bucket),
		fmt.Sprint(regionName),
		internalBool,
		encryptBool,
		secureBool,
		chunkSize,
		fmt.Sprint(rootDirectory),
		fmt.Sprint(regionEndpoint),
	}

	return New(params)
}

// New constructs a new Driver with the given Kingsoft Cloud credentials, region, encryption flag, and
// bucketName
func New(params DriverParameters) (*Driver, error) {

	ks3obj, err := ks3.New(params.AccessKey, params.SecretKey, params.RegionName, params.Secure,
		params.Internal, params.RegionEndpoint)
	if err != nil {
		return nil, err
	}
	ks3.SetDebug(false)

	bucket := ks3obj.Bucket(params.Bucket)

	// TODO(softlns): Currently multipart uploads have no timestamps, so this would be unwise
	// if you initiated a new ks3driver while another one is running on the same bucket.

	d := &driver{
		KS3:           ks3obj,
		Bucket:        bucket,
		ChunkSize:     params.ChunkSize,
		Encrypt:       params.Encrypt,
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
	content, err := d.Bucket.Get(d.ks3Path(path))
	if err != nil {
		return nil, parseError(path, err)
	}
	return content, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	return parseError(path, d.Bucket.Put(d.ks3Path(path), contents, d.getContentType(), getPermissions(), d.getOptions()))
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	headers := make(http.Header)
	headers.Add("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")

	resp, err := d.Bucket.GetResponseWithHeaders(d.ks3Path(path), headers)
	if err != nil {
		if ks3Err, ok := err.(*ks3.Error); ok && ks3Err.Code == "InvalidRange" {
			return ioutil.NopCloser(bytes.NewReader(nil)), nil
		}

		return nil, parseError(path, err)
	}
	return resp.Body, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	key := d.ks3Path(path)

	if !append {
		// TODO (softlns): cancel other uploads at this path
		multi, err := d.Bucket.InitMulti(key, d.getContentType(), getPermissions(), d.getOptions())
		if err != nil {
			return nil, err
		}
		return d.newWriter(key, multi, nil), nil
	}
	multis, _, err := d.Bucket.ListMulti(key, "")
	if err != nil {
		return nil, parseError(path, err)
	}
	for _, multi := range multis {
		if key != multi.Key {
			continue
		}
		parts, err := multi.ListParts()
		if err != nil {
			return nil, parseError(path, err)
		}
		var multiSize int64
		for _, part := range parts {
			multiSize += part.Size
		}
		return d.newWriter(key, multi, parts), nil
	}
	return nil, storagedriver.PathNotFoundError{Path: path}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	listResponse, err := d.Bucket.List(d.ks3Path(path), "", "", 1)
	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(listResponse.Contents) == 1 {
		if listResponse.Contents[0].Key != d.ks3Path(path) {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = listResponse.Contents[0].Size

			timestamp, err := time.Parse(time.RFC3339Nano, listResponse.Contents[0].LastModified)
			if err != nil {
				return nil, err
			}
			fi.ModTime = timestamp
		}
	} else if len(listResponse.CommonPrefixes) == 1 {
		fi.IsDir = true
	} else {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, opath string) ([]string, error) {
	path := opath
	if path != "/" && path[len(path)-1] != '/' {
		path = path + "/"
	}

	// This is to cover for the cases when the rootDirectory of the driver is either "" or "/".
	// In those cases, there is no root prefix to replace and we must actually add a "/" to all
	// results in order to keep them as valid paths as recognized by storagedriver.PathRegexp
	prefix := ""
	if d.ks3Path("") == "" {
		prefix = "/"
	}

	listResponse, err := d.Bucket.List(d.ks3Path(path), "/", "", listMax)
	if err != nil {
		return nil, parseError(opath, err)
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range listResponse.Contents {
			files = append(files, strings.Replace(key.Key, d.ks3Path(""), prefix, 1))
		}

		for _, commonPrefix := range listResponse.CommonPrefixes {
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.ks3Path(""), prefix, 1))
		}

		if listResponse.IsTruncated {
			listResponse, err = d.Bucket.List(d.ks3Path(path), "/", listResponse.NextMarker, listMax)
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
			// have directories in ks3.
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	_, err := d.Bucket.PutCopy(d.ks3Path(destPath), getPermissions(),
		ks3.CopyOptions{Options: d.getOptions(), ContentType: d.getContentType()}, d.Bucket.Name+"/"+d.ks3Path(sourcePath))
	if err != nil {
		return parseError(sourcePath, err)
	}

	return d.Delete(ctx, sourcePath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	listResponse, err := d.Bucket.List(d.ks3Path(path), "", "", listMax)
	if err != nil || len(listResponse.Contents) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}

	ks3Objects := make([]ks3.Object, listMax)

	for len(listResponse.Contents) > 0 {
		for index, key := range listResponse.Contents {
			ks3Objects[index].Key = key.Key
		}

		err := d.Bucket.DelMulti(ks3.Delete{Quiet: false, Objects: ks3Objects[0:len(listResponse.Contents)]})
		if err != nil {
			return nil
		}

		listResponse, err = d.Bucket.List(d.ks3Path(path), "", "", listMax)
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

	return d.Bucket.SignedURLWithMethod(methodString, d.ks3Path(path), expiresTime, nil, nil), nil
}

func (d *driver) ks3Path(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

// KS3BucketKey returns the ks3 bucket key for the given storage driver path.
func (d *Driver) KS3BucketKey(path string) string {
	return d.StorageDriver.(*driver).ks3Path(path)
}

func parseError(path string, err error) error {
	if ks3Err, ok := err.(*ks3.Error); ok && ks3Err.Code == "NoSuchKey" {
		return storagedriver.PathNotFoundError{Path: path}
	}

	return err
}

func hasCode(err error, code string) bool {
	ks3err, ok := err.(*ks3.Error)
	return ok && ks3err.Code == code
}

func (d *driver) getOptions() ks3.Options {
	return ks3.Options{
		SSE: d.Encrypt,
	}
}

func getPermissions() ks3.ACL {
	return ks3.Private
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

// writer attempts to upload parts to KS3 in a buffered fashion where the last
// part is at least as large as the chunksize, so the multipart upload could be
// cleanly resumed in the future. This is violated if Close is called after less
// than a full chunk is written.
type writer struct {
	driver      *driver
	key         string
	multi       *ks3.Multi
	parts       []ks3.Part
	size        int64
	readyPart   []byte
	pendingPart []byte
	closed      bool
	committed   bool
	cancelled   bool
}

func (d *driver) newWriter(key string, multi *ks3.Multi, parts []ks3.Part) storagedriver.FileWriter {
	var size int64
	for _, part := range parts {
		size += part.Size
	}
	return &writer{
		driver: d,
		key:    key,
		multi:  multi,
		parts:  parts,
		size:   size,
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

	// If the last written part is smaller than minChunkSize, we need to make a
	// new multipart upload :sadface:
	if len(w.parts) > 0 && int(w.parts[len(w.parts)-1].Size) < minChunkSize {
		err := w.multi.Complete(w.parts)
		if err != nil {
			w.multi.Abort()
			return 0, err
		}

		multi, err := w.driver.Bucket.InitMulti(w.key, w.driver.getContentType(), getPermissions(), w.driver.getOptions())
		if err != nil {
			return 0, err
		}
		w.multi = multi

		// If the entire written file is smaller than minChunkSize, we need to make
		// a new part from scratch :double sad face:
		if w.size < minChunkSize {
			contents, err := w.driver.Bucket.Get(w.key)
			if err != nil {
				return 0, err
			}
			w.parts = nil
			w.readyPart = contents
		} else {
			// Otherwise we can use the old file as the new first part
			_, part, err := multi.PutPartCopy(1, ks3.CopyOptions{}, w.driver.Bucket.Name+"/"+w.key)
			if err != nil {
				return 0, err
			}
			w.parts = []ks3.Part{part}
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
				err := w.flushPart()
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
	err := w.multi.Abort()
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
	err = w.multi.Complete(w.parts)
	if err != nil {
		w.multi.Abort()
		return err
	}
	return nil
}

// flushPart flushes buffers to write a part to KS3.
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

	part, err := w.multi.PutPart(len(w.parts)+1, bytes.NewReader(w.readyPart))
	if err != nil {
		return err
	}
	w.parts = append(w.parts, part)
	w.readyPart = w.pendingPart
	w.pendingPart = nil
	return nil
}

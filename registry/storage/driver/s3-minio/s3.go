// Package s3 provides a storagedriver.StorageDriver implementation to
// store blobs in Amazon S3 cloud storage.
//
// Keep in mind that S3 guarantees only read-after-write consistency for new
// objects, but no read-after-update or list-after-write consistency.
package s3

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

	"github.com/minio/minio-go"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const driverName = "s3minio"

// minChunkSize defines the minimum multipart upload chunk size.
// S3 API requires multipart upload chunks to be at least 5MB.
const minChunkSize = 5 << 20

const defaultChunkSize = minChunkSize

// listMax is the largest amount of objects you can request from S3 in a list call.
const listMax = 1000

func parseError(path string, err error) error {
	if e, ok := err.(minio.ErrorResponse); ok && e.Code == "NoSuchKey" {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return err
}

// DriverParameters is a struct that encapsulates all of the driver parameters after all values have been set.
type DriverParameters struct {
	AccessKey     string
	SecretKey     string
	Bucket        string
	Endpoint      string
	Secure        bool
	ChunkSize     int64
	RootDirectory string
}

type driver struct {
	S3            *minio.Core
	Bucket        string
	ChunkSize     int64
	RootDirectory string
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Amazon S3
// Objects are stored at absolute keys in the provided bucket.
type Driver struct {
	baseEmbed
}

// New constructs a new Driver with the given parameters.
func New(params DriverParameters) (*Driver, error) {
	s3obj, err := minio.NewCore(params.Endpoint, params.AccessKey, params.SecretKey, params.Secure)
	if err != nil {
		return nil, err
	}

	d := &driver{
		S3:            s3obj,
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

// FromParameters constructs a new Driver with a given parameters map.
// Required parameters: accesskey, secretkey, endpoint, bucket.
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	getString := func(name string) string {
		v := parameters[name]
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}

	getInt64 := func(name string, defaultValue int64) (int64, error) {
		v, ok := parameters[name]
		if !ok {
			return defaultValue, nil
		}
		switch v := v.(type) {
		case int64:
			return v, nil
		case int, uint, int32, uint32, uint64:
			return reflect.ValueOf(v).Convert(reflect.TypeOf(int64(0))).Int(), nil
		case string:
			n, err := strconv.ParseInt(v, 0, 64)
			if err == nil {
				return n, nil
			}
		}
		return defaultValue, fmt.Errorf("The %s parameter must be an integer, %v is invalid", name, v)
	}

	getBool := func(name string, defaultValue bool) (bool, error) {
		v, ok := parameters[name]
		if !ok {
			return defaultValue, nil
		}
		switch v := v.(type) {
		case bool:
			return v, nil
		case string:
			b, err := strconv.ParseBool(v)
			if err == nil {
				return b, nil
			}
		}
		return defaultValue, fmt.Errorf("The %s parameter should be a boolean, %v is invalid", name, v)
	}

	bucket := getString("bucket")
	if bucket == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
	}

	secureBool, err := getBool("secure", true)
	if err != nil {
		return nil, err
	}

	chunkSize, err := getInt64("chunksize", defaultChunkSize)
	if err != nil {
		return nil, err
	}
	if chunkSize < minChunkSize {
		return nil, fmt.Errorf("The chunksize %#v parameter should be a number that is larger than or equal to %d", chunkSize, minChunkSize)
	}

	params := DriverParameters{
		AccessKey:     getString("accesskey"),
		SecretKey:     getString("secretkey"),
		Bucket:        bucket,
		Endpoint:      getString("endpoint"),
		Secure:        secureBool,
		ChunkSize:     chunkSize,
		RootDirectory: getString("rootdirectory"),
	}
	return New(params)
}

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
	_, err := d.S3.PutObject(d.Bucket, d.s3Path(path), int64(len(contents)), bytes.NewReader(contents), nil, nil, map[string][]string{
		"Content-Type": {d.getContentType()},
		"x-amz-acl":    {d.getACL()},
	})
	return err
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	reader, _, err := d.S3.GetObject(d.Bucket, d.s3Path(path), minio.RequestHeaders{
		Header: http.Header{
			"Range": {"bytes=" + strconv.FormatInt(offset, 10) + "-"},
		},
	})
	return reader, parseError(path, err)
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	if append {
		return resumeWriter(ctx, d, path)
	}
	return createWriter(d, path)
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	resp, err := d.S3.ListObjects(d.Bucket, d.s3Path(path), "", "", 1)
	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(resp.Contents) == 1 {
		if resp.Contents[0].Key != d.s3Path(path) {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = resp.Contents[0].Size
			fi.ModTime = resp.Contents[0].LastModified
		}
	} else if len(resp.CommonPrefixes) == 1 {
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
	if d.s3Path("") == "" {
		prefix = "/"
	}

	resp, err := d.S3.ListObjects(d.Bucket, d.s3Path(path), "", "/", listMax)
	if err != nil {
		return nil, err
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range resp.Contents {
			files = append(files, strings.Replace(key.Key, d.s3Path(""), prefix, 1))
		}

		for _, commonPrefix := range resp.CommonPrefixes {
			commonPrefix := commonPrefix.Prefix
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.s3Path(""), prefix, 1))
		}

		if resp.IsTruncated {
			resp, err = d.S3.ListObjects(d.Bucket, d.s3Path(path), resp.NextMarker, "/", listMax)
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
			// have directories in s3.
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	/* This is terrible, but aws doesn't have an actual move. */
	dst, err := minio.NewDestinationInfo(d.Bucket, d.s3Path(destPath), nil, nil)
	if err != nil {
		return err
	}

	src := minio.NewSourceInfo(d.Bucket, d.s3Path(sourcePath), nil)

	err = d.S3.CopyObject(dst, src)
	if err != nil {
		return err
	}

	return d.Delete(ctx, sourcePath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	objectsCh := make(chan string)
	errorCh := d.S3.RemoveObjects(d.Bucket, objectsCh)

	var removeErr error
	done := make(chan struct{})
	go func() {
		removeObjectError, ok := <-errorCh
		if ok {
			removeErr = removeObjectError.Err
		}
		close(done)
		for range errorCh {
		}
	}()

Loop:
	for {
		resp, err := d.S3.ListObjects(d.Bucket, d.s3Path(path), "", "", 0)
		if err != nil {
			close(objectsCh)
			return err
		}
		if len(resp.Contents) == 0 {
			break
		}
		for _, key := range resp.Contents {
			select {
			case <-ctx.Done():
				break Loop
			case <-done:
				break Loop
			case objectsCh <- key.Key:
			}
		}
	}
	close(objectsCh)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	return removeErr
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	method, ok := options["method"]
	if ok {
		methodString, ok := method.(string)
		if !ok || methodString != "GET" {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	expiresIn := 20 * time.Minute
	expires, ok := options["expiry"]
	if ok {
		et, ok := expires.(time.Time)
		if ok {
			expiresIn = et.Sub(time.Now())
		}
	}

	u, err := d.S3.PresignedGetObject(d.Bucket, d.s3Path(path), expiresIn, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

func (d *driver) s3Path(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

func (d *driver) getACL() string {
	return "private"
}

func init() {
	factory.Register(driverName, &s3DriverFactory{})
}

type s3DriverFactory struct{}

var _ factory.StorageDriverFactory = &s3DriverFactory{}

func (factory *s3DriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

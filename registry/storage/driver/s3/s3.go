// Package s3 provides a storagedriver.StorageDriver implementation to
// store blobs in Amazon S3 cloud storage.
//
// This package leverages the AdRoll/goamz client library for interfacing with
// s3.
//
// Because s3 is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
//
// Keep in mind that s3 guarantees only eventual consistency, so do not assume
// that a successful write will mean immediate access to the data written (although
// in most regions a new object put has guaranteed read after write). The only true
// guarantee is that once you call Stat and receive a certain file size, that much of
// the file is already accessible.
package s3

import (
	"fmt"
	"io"
	"reflect"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/s3"
  s3muxer "github.com/rlmcpherson/s3gof3r"
	//"github.com/Sirupsen/logrus"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const driverName = "s3"

// minChunkSize defines the minimum multipart upload chunk size
// S3 API requires multipart upload chunks to be at least 5MB
const minChunkSize = 5 << 20

const defaultChunkSize = 2 * minChunkSize

// listMax is the largest amount of objects you can request from S3 in a list call
const listMax = 1000

//DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	AccessKey     string
	SecretKey     string
	Bucket        string
	Region        aws.Region
	Encrypt       bool
	Secure        bool
	V4Auth        bool
	ChunkSize     int64
	RootDirectory string
}

func init() {
	factory.Register(driverName, &s3DriverFactory{})
}

// s3DriverFactory implements the factory.StorageDriverFactory interface
type s3DriverFactory struct{}

func (factory *s3DriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	S3            *s3.S3
	Bucket        *s3.Bucket
  Muxer         *s3muxer.Bucket
  MuxerConf     *s3muxer.Config
	ChunkSize     int64
	Encrypt       bool
	RootDirectory string

	pool  sync.Pool // pool []byte buffers used for WriteStream
	zeros []byte    // shared, zero-valued buffer used for WriteStream
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Amazon S3
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
	// with an IAM on an ec2 instance (in which case the instance credentials will
	// be summoned when GetAuth is called)
	accessKey, ok := parameters["accesskey"]
	if !ok {
		accessKey = ""
	}
	secretKey, ok := parameters["secretkey"]
	if !ok {
		secretKey = ""
	}

	regionName, ok := parameters["region"]
	if !ok || fmt.Sprint(regionName) == "" {
		return nil, fmt.Errorf("No region parameter provided")
	}
	region := aws.GetRegion(fmt.Sprint(regionName))
	if region.Name == "" {
		return nil, fmt.Errorf("Invalid region provided: %v", region)
	}

	bucket, ok := parameters["bucket"]
	if !ok || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
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

	v4AuthBool := false
	v4Auth, ok := parameters["v4auth"]
	if ok {
		v4AuthBool, ok = v4Auth.(bool)
		if !ok {
			return nil, fmt.Errorf("The v4auth parameter should be a boolean")
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

	params := DriverParameters{
		fmt.Sprint(accessKey),
		fmt.Sprint(secretKey),
		fmt.Sprint(bucket),
		region,
		encryptBool,
		secureBool,
		v4AuthBool,
		chunkSize,
		fmt.Sprint(rootDirectory),
	}

	return New(params)
}

// New constructs a new Driver with the given AWS credentials, region, encryption flag, and
// bucketName
func New(params DriverParameters) (*Driver, error) {
	auth, err := aws.GetAuth(params.AccessKey, params.SecretKey, "", time.Time{})
	if err != nil {
		return nil, fmt.Errorf("unable to resolve aws credentials, please ensure that 'accesskey' and 'secretkey' are properly set or the credentials are available in $HOME/.aws/credentials: %v", err)
	}

	if !params.Secure {
		params.Region.S3Endpoint = strings.Replace(params.Region.S3Endpoint, "https", "http", 1)
	}

	s3obj := s3.New(auth, params.Region)
	bucket := s3obj.Bucket(params.Bucket)

	if params.V4Auth {
		s3obj.Signature = aws.V4Signature
	} else {
		if params.Region.Name == "eu-central-1" {
			return nil, fmt.Errorf("The eu-central-1 region only works with v4 authentication")
		}
	}

	// Validate that the given credentials have at least read permissions in the
	// given bucket scope.
	if _, err := bucket.List(strings.TrimRight(params.RootDirectory, "/"), "", "", 1); err != nil {
		return nil, err
	}

  // Construct the appropriate S3 endpoint from the region
  // For the muxer
  endpoint_slug := ""
  if params.Region.Name != "us-east-1" {
   endpoint_slug = fmt.Sprint("-",params.Region.Name)
  }
  endpoint := fmt.Sprint("s3", endpoint_slug, "amazonaws.com")

  // Build the correct key structure for the muxer
  muxkeys := s3muxer.Keys{
    AccessKey: params.AccessKey,
    SecretKey: params.SecretKey,
  }

  mux_conf := new(s3muxer.Config)
 *mux_conf = *s3muxer.DefaultConfig

  // create the muxer
  s3mux := s3muxer.New(endpoint, muxkeys)
  muxer := s3mux.Bucket(params.Bucket)

	d := &driver{
		S3:            s3obj,
    Muxer:         muxer,
    MuxerConf:     mux_conf,
		Bucket:        bucket,
		ChunkSize:     params.ChunkSize,
		Encrypt:       params.Encrypt,
		RootDirectory: params.RootDirectory,
		zeros:         make([]byte, params.ChunkSize),
	}

	d.pool.New = func() interface{} {
		return make([]byte, d.ChunkSize)
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
  content, err := d.Bucket.Get(d.s3Path(path))
  if err != nil {
    return nil, parseError(path, err)
  }
  return content, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
  return parseError(path, d.Bucket.Put(d.s3Path(path), contents, d.getContentType(), getPermissions(), d.getOptions()))
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) ReadStream(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
  r, _, err := d.Muxer.GetReader(path, d.MuxerConf)
  return r, err
}

// WriteStream stores the contents of the provided io.Reader at a
// location designated by the given path. The driver will know it has
// received the full contents when the reader returns io.EOF. The number
// of successfully READ bytes will be returned, even if an error is
// returned. May be used to resume writing a stream by providing a nonzero
// offset. Offsets past the current size will write from the position
// beyond the end of the file.
func (d *driver) WriteStream(ctx context.Context, path string, offset int64, reader io.Reader) (totalRead int64, err error) {
  writer, err := d.Muxer.PutWriter(path, make(http.Header), d.MuxerConf)
  return io.Copy(writer, reader)
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	listResponse, err := d.Bucket.List(d.s3Path(path), "", "", 1)
	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(listResponse.Contents) == 1 {
		if listResponse.Contents[0].Key != d.s3Path(path) {
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
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
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

	listResponse, err := d.Bucket.List(d.s3Path(path), "/", "", listMax)
	if err != nil {
		return nil, err
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range listResponse.Contents {
			files = append(files, strings.Replace(key.Key, d.s3Path(""), prefix, 1))
		}

		for _, commonPrefix := range listResponse.CommonPrefixes {
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.s3Path(""), prefix, 1))
		}

		if listResponse.IsTruncated {
			listResponse, err = d.Bucket.List(d.s3Path(path), "/", listResponse.NextMarker, listMax)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	/* This is terrible, but aws doesn't have an actual move. */
	_, err := d.Bucket.PutCopy(d.s3Path(destPath), getPermissions(),
		s3.CopyOptions{Options: d.getOptions(), ContentType: d.getContentType()}, d.Bucket.Name+"/"+d.s3Path(sourcePath))
	if err != nil {
		return parseError(sourcePath, err)
	}

	return d.Delete(ctx, sourcePath)
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	listResponse, err := d.Bucket.List(d.s3Path(path), "", "", listMax)
	if err != nil || len(listResponse.Contents) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}

	s3Objects := make([]s3.Object, listMax)

	for len(listResponse.Contents) > 0 {
		for index, key := range listResponse.Contents {
			s3Objects[index].Key = key.Key
		}

		err := d.Bucket.DelMulti(s3.Delete{Quiet: false, Objects: s3Objects[0:len(listResponse.Contents)]})
		if err != nil {
			return nil
		}

		listResponse, err = d.Bucket.List(d.s3Path(path), "", "", listMax)
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
			return "", storagedriver.ErrUnsupportedMethod
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

	return d.Bucket.SignedURLWithMethod(methodString, d.s3Path(path), expiresTime, nil, nil), nil
}

func (d *driver) s3Path(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

// S3BucketKey returns the s3 bucket key for the given storage driver path.
func (d *Driver) S3BucketKey(path string) string {
	return d.StorageDriver.(*driver).s3Path(path)
}

func parseError(path string, err error) error {
	if s3Err, ok := err.(*s3.Error); ok && s3Err.Code == "NoSuchKey" {
		return storagedriver.PathNotFoundError{Path: path}
	}

	return err
}

func hasCode(err error, code string) bool {
	s3err, ok := err.(*aws.Error)
	return ok && s3err.Code == code
}

func (d *driver) getOptions() s3.Options {
	return s3.Options{SSE: d.Encrypt}
}

func getPermissions() s3.ACL {
	return s3.Private
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

// getbuf returns a buffer from the driver's pool with length d.ChunkSize.
func (d *driver) getbuf() []byte {
	return d.pool.Get().([]byte)
}

func (d *driver) putbuf(p []byte) {
	copy(p, d.zeros)
	d.pool.Put(p)
}

// Package bos provides a storagedriver.StorageDriver implementation to
// store blobs in Baidu Object Storage.
//
// This package leverages the official BOS client library for interfacing with
// BOS.
//
// Because BOS is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
// +build include_bos

package bos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/baidubce/bce-sdk-go/bce"
	"github.com/baidubce/bce-sdk-go/services/bos"
	"github.com/baidubce/bce-sdk-go/services/bos/api"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
	"github.com/sirupsen/logrus"
)

const driverName = "bos"

// minChunkSize defines the minimum multipart upload chunk size
// BOS API requires multipart upload chunks to be at least 5MB
const minChunkSize = 5 << 20

// maxChunkSize defines the maximum multipart upload chunk size allowed by BOS.
const maxChunkSize = 5 << 30

// defaultChunkSize define default chunk size
const defaultChunkSize = 2 * minChunkSize

// listMax is the largest amount of objects you can request from BOS in a list call
const listMax = 1000

//DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	Endpoint        string
	Encrypt         bool
	Secure          bool
	RootDirectory   string
	StorageClass    string
	ChunkSize       int64
}

func init() {
	factory.Register(driverName, &bosDriverFactory{})
}

// BosDriverFactory implements the factory.StorageDriverFactory interface
// should use "bos" but not "Bos" because it used by other packages
type bosDriverFactory struct{}

func (factory *bosDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	Client        *bos.Client
	Bucket        string
	ChunkSize     int64
	Encrypt       bool
	RootDirectory string
	StorageClass  string
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by BOS
// Objects are stored at absolute keys in the provided bucket.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
// it can't used by os.Getenv which return string("") when key not found
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	// it's public auth when ak and sk is nil
	accessKeyID := parameters["accesskeyid"]
	secretKey := parameters["secretaccesskey"]

	bucket, ok := parameters["bucket"]
	if !ok || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
	}

	region, ok := parameters["region"]
	if !ok || fmt.Sprint(region) == "" {
		return nil, fmt.Errorf("No region parameter provided")
	}

	endpoint, ok := parameters["endpoint"]
	if !ok {
		endpoint = ""
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

	rootDirectory, ok := parameters["rootdirectory"]
	if !ok {
		rootDirectory = ""
	}

	storageClass := api.STORAGE_CLASS_STANDARD
	storageClassParam, ok := parameters["storageclass"]
	if ok {
		storageClassString, ok := storageClassParam.(string)
		if !ok {
			return nil, fmt.Errorf("The storageclass parameter must be a string")
		}
		storageClassString = strings.ToUpper(storageClassString)
		validStorageClass := false
		storageClasses := []string{api.STORAGE_CLASS_STANDARD,
			api.STORAGE_CLASS_STANDARD_IA, api.STORAGE_CLASS_COLD}
		for _, class := range storageClasses {
			if storageClassString == class {
				validStorageClass = true
				break
			}
		}
		if !validStorageClass {
			return nil, fmt.Errorf("The storageclass parameter must be one of %v, %v invalid",
				storageClasses, storageClassParam)
		}
	}

	chunkSize, err := getParameterAsInt64(parameters, "chunksize", defaultChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	// should not to log all params, avoid sensitive information like SecretAccessKey
	logrus.WithFields(logrus.Fields{
		"accesskeyid":   accessKeyID,
		"bucket":        bucket,
		"region":        region,
		"endpoint":      endpoint,
		"rootdirectory": rootDirectory,
		"storageclass":  storageClass,
		"chunksize":     chunkSize,
	}).Debugf("FromParameters")

	params := DriverParameters{
		AccessKeyID:     fmt.Sprint(accessKeyID),
		SecretAccessKey: fmt.Sprint(secretKey),
		Bucket:          fmt.Sprint(bucket),
		Region:          fmt.Sprint(region),
		Encrypt:         encryptBool,
		Secure:          secureBool,
		Endpoint:        fmt.Sprint(endpoint),
		RootDirectory:   fmt.Sprint(rootDirectory),
		StorageClass:    storageClass,
		ChunkSize:       chunkSize,
	}

	return New(params)
}

// getParameterAsInt64 converts paramaters[name] to an int64 value (using
// defaultt if nil), verifies it is no smaller than min, and returns it.
func getParameterAsInt64(parameters map[string]interface{}, name string, defaultt int64, min int64, max int64) (int64, error) {
	rv := defaultt
	param := parameters[name]
	switch v := param.(type) {
	case string:
		vv, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			return 0, fmt.Errorf("%s parameter must be an integer, %v invalid", name, param)
		}
		rv = vv
	case int64:
		rv = v
	case int, uint, int32, uint32, uint64:
		rv = reflect.ValueOf(v).Convert(reflect.TypeOf(rv)).Int()
	case nil:
		// do nothing
	default:
		return 0, fmt.Errorf("invalid value for %s: %#v", name, param)
	}

	if rv < min || rv > max {
		return 0, fmt.Errorf("The %s %#v parameter should be a number between %d and %d (inclusive)", name, rv, min, max)
	}

	return rv, nil
}

// New constructs a new Driver
func New(params DriverParameters) (*Driver, error) {
	client, err := bos.NewClient(params.AccessKeyID, params.SecretAccessKey, params.Endpoint)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err.Error(),
		}).Errorf("New client fail")
		return nil, err
	}

	d := &driver{
		Client:        client,
		Bucket:        params.Bucket,
		ChunkSize:     params.ChunkSize,
		Encrypt:       params.Encrypt,
		RootDirectory: params.RootDirectory,
		StorageClass:  params.StorageClass,
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
		logrus.WithFields(logrus.Fields{
			"cause":  "Reader",
			"bucket": d.Bucket,
			"path":   path,
			"err":    err,
		}).Errorf("GetContent fail")
		return nil, err
	}

	content, err := ioutil.ReadAll(reader)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause":  "ReadAll",
			"bucket": d.Bucket,
			"path":   path,
			"err":    err,
		}).Errorf("GetContent fail")
		return nil, err
	}

	return content, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	_, err := d.Client.PutObjectFromBytes(d.Bucket, d.bosPath(path), contents,
		&api.PutObjectArgs{
			StorageClass: d.getStorageClass(),
		})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause":  "PutObjectFromBytes",
			"bucket": d.Bucket,
			"path":   path,
			"error":  err,
		}).Errorf("GetContent fail")
		return err
	}

	return parseError(path, nil)
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	getObjectResult, err := d.Client.GetObject(d.Bucket, d.bosPath(path), nil, offset)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause":  "GetObject",
			"bucket": d.Bucket,
			"path":   path,
			"offset": offset,
			"error":  err,
		}).Errorf("Reader fail")
		bosErr, ok := err.(*bce.BceServiceError)
		if ok && bosErr.Code == "InvalidRange" {
			return ioutil.NopCloser(bytes.NewReader(nil)), nil
		}

		return nil, parseError(path, err)
	}

	return getObjectResult.Body, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	key := d.bosPath(path)
	if !append {
		initiateMultipartUploadResult, err := d.Client.InitiateMultipartUpload(
			d.Bucket, key, d.getContentType(),
			&api.InitiateMultipartUploadArgs{
				StorageClass: d.getStorageClass(),
			})
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"cause":  "InitiateMultipartUpload",
				"bucket": d.Bucket,
				"path":   path,
				"append": append,
				"error":  err,
			}).Errorf("Writer fail")
			return nil, err
		}
		return d.newWriter(key, initiateMultipartUploadResult.UploadId, nil), nil
	}

	listMultipartUploadsResult, err := d.Client.ListMultipartUploads(d.Bucket, &api.ListMultipartUploadsArgs{
		Prefix: key,
	})
	if err != nil {
		return nil, parseError(path, err)
	}

	for _, multi := range listMultipartUploadsResult.Uploads {
		if key != multi.Key {
			continue
		}
		listPartsResult, err := d.Client.ListParts(d.Bucket, key, multi.UploadId, &api.ListPartsArgs{})
		if err != nil {
			return nil, parseError(path, err)
		}
		var multiSize int64
		for _, part := range listPartsResult.Parts {
			multiSize += int64(part.Size)
		}
		return d.newWriter(key, multi.UploadId, listPartsResult.Parts), nil
	}

	return nil, storagedriver.PathNotFoundError{Path: path}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	bosPath := d.bosPath(path)

	listObjectsResult, err := d.Client.ListObjects(d.Bucket, &api.ListObjectsArgs{
		Prefix:  bosPath,
		MaxKeys: 1,
	})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause":   "ListObjects",
			"bucket":  d.Bucket,
			"path":    path,
			"bospath": bosPath,
			"error":   err,
		}).Errorf("Stat fail")
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(listObjectsResult.Contents) == 1 {
		if listObjectsResult.Contents[0].Key != bosPath {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = int64(listObjectsResult.Contents[0].Size)
			timestamp, err := time.Parse(time.RFC3339Nano,
				listObjectsResult.Contents[0].LastModified)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"cause":        "Parse",
					"lastmodified": listObjectsResult.Contents[0].LastModified,
					"error":        err,
				}).Errorf("Stat fail")
				return nil, err
			}
			fi.ModTime = timestamp
		}
	} else if len(listObjectsResult.CommonPrefixes) == 1 {
		fi.IsDir = true
	} else {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}

	logrus.WithFields(logrus.Fields{
		"path":    fi.Path,
		"isdir":   fi.IsDir,
		"size":    fi.Size,
		"modtime": fi.ModTime,
	}).Debugf("Stat succ")

	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, opath string) ([]string, error) {
	path := opath
	if path != "/" && opath[len(path)-1] != '/' {
		path = path + "/"
	}

	prefix := ""
	if d.bosPath("") == "" {
		prefix = "/"
	}

	listObjectsResult, err := d.Client.ListObjects(d.Bucket, &api.ListObjectsArgs{
		Prefix:    d.bosPath(path),
		MaxKeys:   listMax,
		Delimiter: "/",
	})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause": "ListObjects",
			"path":  path,
			"error": err,
		}).Errorf("List fail")
		return nil, parseError(opath, err)
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range listObjectsResult.Contents {
			files = append(files, strings.Replace(key.Key, d.bosPath(""), prefix, 1))
			logrus.WithFields(logrus.Fields{
				"path":  path,
				"file":  files[len(files)-1:][0],
				"error": err,
			}).Debugf("List")
		}

		for _, commonPrefix := range listObjectsResult.CommonPrefixes {
			commonPrefix := commonPrefix.Prefix
			directories = append(directories,
				strings.Replace(commonPrefix[0:len(commonPrefix)-1],
					d.bosPath(""), prefix, 1))
			logrus.WithFields(logrus.Fields{
				"path":  path,
				"dir":   directories[len(directories)-1:][0],
				"error": err,
			}).Debugf("List")
		}

		if listObjectsResult.IsTruncated {
			listObjectsResult, err = d.Client.ListObjects(d.Bucket, &api.ListObjectsArgs{
				Prefix:    d.bosPath(path),
				MaxKeys:   listMax,
				Marker:    listObjectsResult.NextMarker,
				Delimiter: "/",
			})
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"cause": "ListObjects",
					"path":  path,
					"error": err,
				}).Errorf("List fail")
				return nil, err
			}
		} else {
			break
		}
	}

	if opath != "/" {
		if len(files) == 0 && len(directories) == 0 {
			logrus.WithFields(logrus.Fields{
				"cause": "empty files and dirs",
				"path":  path,
				"error": err,
			}).Debugf("List fail")
			// Treat empty response as missing directory, since we don't actually
			// have directories.
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	logrus.WithFields(logrus.Fields{
		"path":         path,
		"files":        files,
		"directories:": directories,
		"error":        err,
	}).Debugf("List succ")

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	// This is terrible, but bos doesn't have an actual move.
	if err := d.copy(ctx, sourcePath, destPath); err != nil {
		return err
	}
	return d.Delete(ctx, sourcePath)
}

// copy copies an object stored at sourcePath to destPath.
func (d *driver) copy(ctx context.Context, sourcePath string, destPath string) error {
	_, err := d.Client.CopyObject(d.Bucket, d.bosPath(destPath),
		d.Bucket, d.bosPath(sourcePath), &api.CopyObjectArgs{})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause":      "CopyObject",
			"sourcePath": sourcePath,
			"destPath":   destPath,
			"error":      err,
		}).Errorf("copy fail")
		return nil
	}

	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
// We must be careful since BOS does not guarantee read after delete consistency
func (d *driver) Delete(ctx context.Context, path string) error {
	bosPath := d.bosPath(path)

	listObjectsArgs := &api.ListObjectsArgs{
		Prefix:  bosPath,
		MaxKeys: listMax,
	}

	objects := make([]string, listMax)

	listCount := 0

	for {
		listObjectsResult, err := d.Client.ListObjects(d.Bucket, listObjectsArgs)
		if err != nil || len(listObjectsResult.Contents) == 0 {
			if listCount == 0 {
				return storagedriver.PathNotFoundError{Path: path}
			}
			break
		}
		listCount++

		numObjects := len(listObjectsResult.Contents)
		for index, content := range listObjectsResult.Contents {
			// Stop if we encounter a key that is not a subpath (so that deleting "/a" does not delete "/ab").
			if len(content.Key) > len(bosPath) && (content.Key)[len(bosPath)] != '/' {
				numObjects = index
				break
			}
			objects[index] = content.Key
		}

		_, err = d.Client.DeleteMultipleObjectsFromKeyList(d.Bucket, objects[0:numObjects])
		// TODO(hanchen): it's sdk's bug, always return EOF when delete all succ
		if err != nil && err.Error() != "EOF" {
			logrus.WithFields(logrus.Fields{
				"cause":   "DeleteMultipleObjectsFromKeyList",
				"path":    path,
				"bospath": bosPath,
				"num":     numObjects,
				"objects": objects[0],
				"error":   err,
			}).Errorf("Delete fail")
			return err
		}

		if numObjects < len(listObjectsResult.Contents) {
			return nil
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

	expiresIn := 20 * time.Minute
	expires, ok := options["expiry"]
	if ok {
		et, ok := expires.(time.Time)
		if ok {
			expiresIn = et.Sub(time.Now())
		}
	}

	signedURL := d.Client.GeneratePresignedUrl(d.Bucket, d.bosPath(path),
		int(expiresIn.Seconds()), methodString, nil, nil)

	logrus.WithFields(logrus.Fields{
		"path":      path,
		"signdur":   signedURL,
		"method":    methodString,
		"expires":   expires,
		"expiresin": expiresIn,
	}).Debugf("URLFor succ")

	return signedURL, nil
}

func (d *driver) bosPath(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

func parseError(path string, err error) error {
	bosErr, ok := err.(*bce.BceServiceError)
	if ok && (bosErr.Code == "NoSuchKey" || bosErr.Code == "") {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return err
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

func (d *driver) getStorageClass() string {
	return d.StorageClass
}

// writer attempts to upload parts to BOS in a buffered fashion where the last
// part is at least as large as the chunksize, so the multipart upload could be
// cleanly resumed in the future. This is violated if Close is called after less
// than a full chunk is written.
type writer struct {
	driver      *driver
	key         string
	uploadID    string
	parts       []api.ListPartType
	size        int64
	readyPart   []byte
	pendingPart []byte
	closed      bool
	committed   bool
	cancelled   bool
}

func (d *driver) newWriter(key, uploadID string, parts []api.ListPartType) storagedriver.FileWriter {
	var size int64
	for _, part := range parts {
		size += int64(part.Size)
	}
	return &writer{
		driver:   d,
		key:      key,
		uploadID: uploadID,
		parts:    parts,
		size:     size,
	}
}

type completedParts []api.UploadInfoType

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return a[i].PartNumber < a[j].PartNumber }

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
		var completedUploadedParts completedParts
		for _, part := range w.parts {
			completedUploadedParts = append(completedUploadedParts,
				api.UploadInfoType{
					ETag:       part.ETag,
					PartNumber: part.PartNumber,
				})
		}

		sort.Sort(completedUploadedParts)

		_, err := w.driver.Client.CompleteMultipartUploadFromStruct(w.driver.Bucket,
			w.key, w.uploadID,
			&api.CompleteMultipartUploadArgs{
				Parts: completedUploadedParts,
			}, nil)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"cause": "CompleteMultipartUploadFromStruct",
				"error": err,
			}).Errorf("Write fail")
			w.driver.Client.AbortMultipartUpload(w.driver.Bucket, w.key, w.uploadID)
			return 0, err
		}

		initiateMultipartUploadResult, err := w.driver.Client.InitiateMultipartUpload(
			w.driver.Bucket, w.key, w.driver.getContentType(),
			&api.InitiateMultipartUploadArgs{
				StorageClass: w.driver.getStorageClass(),
			})
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"cause": "InitiateMultipartUpload",
				"error": err,
			}).Errorf("Write fail")
			return 0, err
		}
		w.uploadID = initiateMultipartUploadResult.UploadId

		// If the entire written file is smaller than minChunkSize, we need to make
		// a new part from scratch :double sad face:
		if w.size < minChunkSize {
			getObjectResult, err := w.driver.Client.GetObject(w.driver.Bucket, w.key, nil)
			defer getObjectResult.Body.Close()
			if err != nil {
				return 0, err
			}
			w.parts = nil
			w.readyPart, err = ioutil.ReadAll(getObjectResult.Body)
			if err != nil {
				return 0, err
			}
		} else {
			copyObjectResult, err := w.driver.Client.UploadPartCopy(w.driver.Bucket, w.key,
				w.driver.Bucket, w.driver.Bucket+"/"+w.key,
				initiateMultipartUploadResult.UploadId, 1, &api.UploadPartCopyArgs{})
			if err != nil {
				return 0, err
			}
			w.parts = []api.ListPartType{
				{
					ETag:       copyObjectResult.ETag,
					PartNumber: 1,
					Size:       int(w.size),
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
	err := w.driver.Client.AbortMultipartUpload(w.driver.Bucket, w.key, w.uploadID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause": "AbortMultipartUpload",
			"error": err,
		}).Errorf("Cancel fail")
		return err
	}
	return nil
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

	var completedUploadedParts completedParts
	for _, part := range w.parts {
		completedUploadedParts = append(completedUploadedParts, api.UploadInfoType{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}

	sort.Sort(completedUploadedParts)

	_, err = w.driver.Client.CompleteMultipartUploadFromStruct(w.driver.Bucket, w.key, w.uploadID,
		&api.CompleteMultipartUploadArgs{
			Parts: completedUploadedParts,
		}, nil)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause": "CompleteMultipartUploadFromStruct",
			"error": err,
		}).Errorf("Commit fail")
		// not need to handler error
		w.driver.Client.AbortMultipartUpload(w.driver.Bucket, w.key, w.uploadID)
		return err
	}

	return nil
}

// flushPart flushes buffers to write a part to BOS.
// Only called by Write (with both buffers full) and Close/Commit (always)
func (w *writer) flushPart() error {
	if len(w.readyPart) == 0 && len(w.pendingPart) == 0 {
		// nothing to write
		logrus.Debugf("Nothing to write")
		return nil
	}
	if len(w.pendingPart) < int(w.driver.ChunkSize) {
		// closing with a small pending part
		// combine ready and pending to avoid writing a small part
		w.readyPart = append(w.readyPart, w.pendingPart...)
		w.pendingPart = nil
	}

	partNumber := int(len(w.parts) + 1)
	content, err := bce.NewBodyFromBytes(w.readyPart)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause":      "NewBodyFromBytes",
			"error":      err,
			"partnumber": partNumber,
		}).Errorf("flushPart fail")
		return err
	}
	etag, err := w.driver.Client.UploadPart(w.driver.Bucket, w.key, w.uploadID,
		partNumber, content, &api.UploadPartArgs{})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"cause":      "UploadPart",
			"error":      err,
			"partnumber": partNumber,
		}).Errorf("flushPart fail")
		return err
	}

	w.parts = append(w.parts, api.ListPartType{
		ETag:       etag,
		PartNumber: partNumber,
		Size:       len(w.readyPart),
	})
	w.readyPart = w.pendingPart
	w.pendingPart = nil

	return nil
}

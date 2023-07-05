// Package obs provides a storagedriver.StorageDriver implementation to
// store blobs in HuaweiCloud storage.
//
// This package leverages the huaweicloud/huaweicloud-sdk-go-obs client library
// for interfacing with obs.
//
// Because obs is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
//
// Note that the contents of incomplete uploads are not accessible even though
// Stat returns their length
//

package obs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
)

// noStorageClass defines the value to be used if storage class is not supported by the OBS endpoint
const noStorageClass obs.StorageClassType = "NONE"

// OBSStorageClasses lists all compatible (instant retrieval) OBS storage classes
var OBSStorageClasses = []obs.StorageClassType{
	noStorageClass,
	obs.StorageClassStandard,
	obs.StorageClassWarm,
	obs.StorageClassCold,
}

// validStorageClasses contains known OBS StorageClass
var validStorageClasses = map[obs.StorageClassType]struct{}{}

var OBSAcls = []obs.AclType{
	obs.AclPrivate,
	obs.AclPublicRead,
	obs.AclPublicReadWrite,
	obs.AclAuthenticatedRead,
	obs.AclBucketOwnerRead,
	obs.AclBucketOwnerFullControl,
	obs.AclLogDeliveryWrite,
	obs.AclPublicReadDelivery,
	obs.AclPublicReadWriteDelivery,
}

// validObjectACLs contains known OBS object Acls
var validObjectACLs = map[obs.AclType]struct{}{}

const (
	driverName     = "obs"
	dummyProjectID = "<unknown>"

	uploadSessionContentType           = "application/x-docker-upload-session"
	minChunkSize                       = 5 << 20
	maxChunkSize                       = 5 << 30
	defaultChunkSize                   = 2 * minChunkSize
	defaultMaxConcurrency              = 50
	minConcurrency                     = 25
	listMax                            = 1000
	maxTries                           = 5
	defaultMultipartCopyChunkSize      = 32 << 20
	defaultMultipartCopyMaxConcurrency = 100
	defaultMultipartCopyThresholdSize  = 32 << 20
)

// DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	AccessKey                   string
	SecretKey                   string
	Bucket                      string
	Endpoint                    string
	Encrypt                     bool
	ChunkSize                   int64
	RootDirectory               string
	EncryptionKeyID             string
	MultipartCopyChunkSize      int64
	MultipartCopyMaxConcurrency int64
	MultipartCopyThresholdSize  int64
	MultipartCombineSmallPart   bool
	StorageClass                obs.StorageClassType
	ObjectACL                   obs.AclType
}

func init() {
	for _, storageClass := range OBSStorageClasses {
		validStorageClasses[storageClass] = struct{}{}
	}
	for _, acl := range OBSAcls {
		validObjectACLs[acl] = struct{}{}
	}
	factory.Register(driverName, &obsDriverFactory{})
}

// obsDriverFactory implements the factory.StorageDriverFactory interface
type obsDriverFactory struct{}

func (factory *obsDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	Client                      *obs.ObsClient
	Bucket                      string
	ChunkSize                   int64
	Encrypt                     bool
	RootDirectory               string
	EncryptionKeyID             string
	MultipartCopyChunkSize      int64
	MultipartCopyMaxConcurrency int64
	MultipartCopyThresholdSize  int64
	StorageClass                obs.StorageClassType
	ObjectACL                   obs.AclType
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by OBS
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
	accessKey, ok := parameters["accesskey"]
	if !ok {
		return nil, fmt.Errorf("No accesskeyid parameter provided")
	}
	secretKey, ok := parameters["secretkey"]
	if !ok {
		return nil, fmt.Errorf("No accesskeysecret parameter provided")
	}

	endpoint, ok := parameters["endpoint"]
	if !ok || fmt.Sprint(endpoint) == "" {
		return nil, fmt.Errorf("No region endpoint parameter provided")
	}

	bucket, ok := parameters["bucket"]
	if !ok || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("No bucket parameter provided")
	}

	storageClass := obs.StorageClassStandard
	storageClassParam, ok := parameters["storageclass"]
	if ok {
		storageClassString, ok := storageClassParam.(obs.StorageClassType)
		if !ok {
			return nil, fmt.Errorf(
				"the storageclass parameter must be one of %v, %v invalid",
				OBSStorageClasses,
				storageClassParam,
			)
		}
		if _, ok = validStorageClasses[storageClassString]; !ok {
			return nil, fmt.Errorf(
				"the storageclass parameter must be one of %v, %v invalid",
				OBSStorageClasses,
				storageClassParam,
			)
		}
		storageClass = storageClassString
	}

	objectACL := obs.AclPrivate
	objectACLParam, ok := parameters["objectacl"]
	if ok {
		objectACLString, ok := objectACLParam.(obs.AclType)
		if !ok {
			return nil, fmt.Errorf(
				"the objectacl parameter must be one of %v, %v invalid",
				OBSAcls,
				objectACLParam,
			)
		}

		if _, ok = validObjectACLs[objectACLString]; !ok {
			return nil, fmt.Errorf(
				"the objectacl parameter must be one of %v, %v invalid",
				OBSAcls,
				objectACLParam,
			)
		}
		objectACL = objectACLString
	}

	encryptBool := false
	encrypt, ok := parameters["encrypt"]
	if ok {
		encryptBool, ok = encrypt.(bool)
		if !ok {
			return nil, fmt.Errorf("The encrypt parameter should be a boolean")
		}
	}

	encryptionKeyID, ok := parameters["encryptionkeyid"]
	if !ok {
		encryptionKeyID = ""
	}

	chunkSize, err := getParameterAsInt64(parameters, "chunksize", defaultChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	multipartCopyChunkSize, err := getParameterAsInt64(parameters, "multipartcopychunksize", defaultMultipartCopyChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	multipartCopyMaxConcurrency, err := getParameterAsInt64(parameters, "multipartcopymaxconcurrency", defaultMultipartCopyMaxConcurrency, 1, math.MaxInt64)
	if err != nil {
		return nil, err
	}

	multipartCopyThresholdSize, err := getParameterAsInt64(parameters, "multipartcopythresholdsize", defaultMultipartCopyThresholdSize, 0, maxChunkSize)
	if err != nil {
		return nil, err
	}

	rootDirectory, ok := parameters["rootdirectory"]
	if !ok {
		rootDirectory = ""
	}

	params := DriverParameters{
		AccessKey:                   fmt.Sprint(accessKey),
		SecretKey:                   fmt.Sprint(secretKey),
		Bucket:                      fmt.Sprint(bucket),
		Endpoint:                    fmt.Sprint(endpoint),
		ChunkSize:                   chunkSize,
		RootDirectory:               fmt.Sprint(rootDirectory),
		Encrypt:                     encryptBool,
		EncryptionKeyID:             fmt.Sprint(encryptionKeyID),
		MultipartCopyChunkSize:      multipartCopyChunkSize,
		MultipartCopyMaxConcurrency: multipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  multipartCopyThresholdSize,
		StorageClass:                storageClass,
		ObjectACL:                   objectACL,
	}

	return New(params)
}

// getParameterAsInt64 converts parameters[name] to an int64 value (using
// defaultt if nil), verifies it is no smaller than min, and returns it.
func getParameterAsInt64(parameters map[string]interface{}, name string, defaultt int64, min int64, max int64) (int64, error) {
	rv := defaultt
	param, ok := parameters[name]
	if ok {
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
	}

	if rv < min || rv > max {
		return 0, fmt.Errorf("the %s %#v parameter should be a number between %d and %d (inclusive)", name, rv, min, max)
	}

	return rv, nil
}

// New constructs a new Driver with the given Aliyun credentials, region, encryption flag, and
// bucketName
func New(params DriverParameters) (*Driver, error) {
	client, err := obs.New(params.AccessKey, params.SecretKey, params.Endpoint)
	if err != nil {
		return nil, err
	}

	d := &driver{
		Client:                      client,
		Bucket:                      params.Bucket,
		ChunkSize:                   params.ChunkSize,
		Encrypt:                     params.Encrypt,
		RootDirectory:               params.RootDirectory,
		EncryptionKeyID:             params.EncryptionKeyID,
		MultipartCopyChunkSize:      params.MultipartCopyChunkSize,
		MultipartCopyMaxConcurrency: params.MultipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  params.MultipartCopyThresholdSize,
		ObjectACL:                   params.ObjectACL,
		StorageClass:                params.StorageClass,
	}

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: d,
			},
		},
	}, nil
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
	return io.ReadAll(reader)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	input := &obs.PutObjectInput{}
	input.Bucket = d.Bucket
	input.Key = d.OBSPath(path)
	input.Body = bytes.NewReader(contents)
	input.SseHeader = d.getEncryptionMode()
	_, err := d.Client.PutObject(input)
	return d.parseError(path, err)
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	input := obs.GetObjectInput{}
	input.Bucket = d.Bucket
	input.Key = d.OBSPath(path)
	input.RangeStart = offset
	input.RangeEnd = math.MaxInt64
	output, err := d.Client.GetObject(&input)
	if err != nil {
		if obsErr, ok := err.(obs.ObsError); ok {
			if obsErr.Code == "InvalidRange" {
				return io.NopCloser(bytes.NewReader(nil)), nil
			}
		}
		return nil, d.parseError(path, err)
	}
	return output.Body, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, appendParam bool) (storagedriver.FileWriter, error) {
	key := d.OBSPath(path)
	if !appendParam {
		// TODO (brianbland): cancel other uploads at this path
		initInput := &obs.InitiateMultipartUploadInput{}
		initInput.Bucket = d.Bucket
		initInput.Key = key
		initInput.ContentType = d.getContentType()
		initInput.ACL = d.getACL()
		initInput.StorageClass = d.getStorageClass()
		initInput.SseHeader = d.getEncryptionMode()
		initOutput, err := d.Client.InitiateMultipartUpload(initInput)
		if err != nil {
			return nil, err
		}
		return d.newWriter(key, initOutput.UploadId, nil), nil
	}

	listMultipartUploadsInput := &obs.ListMultipartUploadsInput{
		Bucket: d.Bucket,
		Prefix: key,
	}
	for {
		output, err := d.Client.ListMultipartUploads(listMultipartUploadsInput)
		if err != nil {
			return nil, d.parseError(path, err)
		}

		// output.Uploads can only be empty on the first call
		// if there were no more results to return after the first call, output.IsTruncated would have been false
		// and the loop would be exited without recalling ListMultipartUploads
		if len(output.Uploads) == 0 {
			break
		}

		var allParts []obs.Part
		for _, multi := range output.Uploads {
			if key != multi.Key {
				continue
			}

			partsList, err := d.Client.ListParts(&obs.ListPartsInput{
				Bucket:   d.Bucket,
				Key:      key,
				UploadId: multi.UploadId,
			})
			if err != nil {
				return nil, d.parseError(path, err)
			}
			allParts = append(allParts, partsList.Parts...)
			for partsList.IsTruncated {
				partsList, err = d.Client.ListParts(&obs.ListPartsInput{
					Bucket:           d.Bucket,
					Key:              key,
					UploadId:         multi.UploadId,
					PartNumberMarker: partsList.NextPartNumberMarker,
				})
				if err != nil {
					return nil, d.parseError(path, err)
				}
				allParts = append(allParts, partsList.Parts...)
			}
			return d.newWriter(key, multi.UploadId, allParts), nil
		}

		// output.NextUploadIdMarker must have at least one element or we would have returned not found
		listMultipartUploadsInput.UploadIdMarker = output.NextUploadIdMarker

		// IsTruncated "specifies whether (true) or not (false) all of the results were returned"
		// if everything has been returned, break
		if !output.IsTruncated {
			break
		}
	}
	return nil, storagedriver.PathNotFoundError{Path: path}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	input := &obs.ListObjectsInput{}
	input.Bucket = d.Bucket
	input.Prefix = d.OBSPath(path)
	input.MaxKeys = 1
	output, err := d.Client.ListObjects(input)
	if err != nil {
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: path,
	}

	if len(output.Contents) == 1 {
		if output.Contents[0].Key != d.OBSPath(path) {
			fi.IsDir = true
		} else {
			fi.IsDir = false
			fi.Size = output.Contents[0].Size
			fi.ModTime = output.Contents[0].LastModified
		}
	} else if len(output.CommonPrefixes) == 1 {
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
	if d.OBSPath("") == "" {
		prefix = "/"
	}
	input := &obs.ListObjectsInput{}
	input.Bucket = d.Bucket
	input.Prefix = d.OBSPath(path)
	input.MaxKeys = listMax
	input.Delimiter = "/"
	output, err := d.Client.ListObjects(input)
	if err != nil {
		return nil, d.parseError(opath, err)
	}

	files := []string{}
	directories := []string{}

	for {
		for _, content := range output.Contents {
			files = append(files, strings.Replace(content.Key, d.OBSPath(""), prefix, 1))
		}

		for _, commonPrefix := range output.CommonPrefixes {
			commonPrefix := commonPrefix
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.OBSPath(""), prefix, 1))
		}

		if output.IsTruncated {
			input.Marker = output.NextMarker
			output, err = d.Client.ListObjects(input)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}

	// This is to cover for the cases when the first key equal to obsPath.
	if len(files) > 0 && files[0] == strings.Replace(d.OBSPath(path), d.OBSPath(""), prefix, 1) {
		files = files[1:]
	}

	if opath != "/" {
		if len(files) == 0 && len(directories) == 0 {
			// Treat empty response as missing directory, since we don't actually
			// have directories in OBS.
			return nil, storagedriver.PathNotFoundError{Path: opath}
		}
	}

	return append(files, directories...), nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	/* This is terrible, but aws doesn't have an actual move. */
	if err := d.copy(ctx, sourcePath, destPath); err != nil {
		return err
	}
	return d.Delete(ctx, sourcePath)
}

// copy copies an object stored at sourcePath to destPath.
func (d *driver) copy(ctx context.Context, sourcePath string, destPath string) error {
	// OBS can copy objects up to 5 GB in size with a single PUT Object - Copy
	// operation. For larger objects, the multipart upload API must be used.
	//
	// Empirically, multipart copy is fastest with 32 MB parts and is faster
	// than PUT Object - Copy for objects larger than 32 MB.

	fileInfo, err := d.Stat(ctx, sourcePath)
	if err != nil {
		return d.parseError(sourcePath, err)
	}

	if fileInfo.Size() <= d.MultipartCopyThresholdSize {
		input := &obs.CopyObjectInput{}
		input.Bucket = d.Bucket
		input.Key = d.OBSPath(destPath)
		input.ContentType = d.getContentType()
		input.ACL = d.getACL()
		input.SseHeader = d.getEncryptionMode()
		input.StorageClass = d.getStorageClass()
		input.CopySourceBucket = d.Bucket
		input.CopySourceKey = d.OBSPath(sourcePath)
		_, err := d.Client.CopyObject(input)
		if err != nil {
			return d.parseError(sourcePath, err)
		}
		return nil
	}
	initInput := &obs.InitiateMultipartUploadInput{}
	initInput.Bucket = d.Bucket
	initInput.Key = d.OBSPath(destPath)
	initInput.ContentType = d.getContentType()
	initInput.ACL = d.getACL()
	initInput.StorageClass = d.getStorageClass()
	initInput.SseHeader = d.getEncryptionMode()
	initOutput, err := d.Client.InitiateMultipartUpload(initInput)
	if err != nil {
		return err
	}

	numParts := (fileInfo.Size() + d.MultipartCopyChunkSize - 1) / d.MultipartCopyChunkSize
	completedParts := make([]obs.Part, numParts)
	errChan := make(chan error, numParts)
	limiter := make(chan struct{}, d.MultipartCopyMaxConcurrency)

	for i := range completedParts {
		i := int64(i)
		go func() {
			limiter <- struct{}{}
			firstByte := i * d.MultipartCopyChunkSize
			lastByte := firstByte + d.MultipartCopyChunkSize - 1
			if lastByte >= fileInfo.Size() {
				lastByte = fileInfo.Size() - 1
			}
			copyPartOutput, err := d.Client.CopyPart(&obs.CopyPartInput{
				Bucket:               d.Bucket,
				Key:                  d.OBSPath(destPath),
				CopySourceBucket:     d.Bucket,
				CopySourceKey:        d.OBSPath(sourcePath),
				PartNumber:           int(i + 1),
				UploadId:             initOutput.UploadId,
				CopySourceRangeStart: firstByte,
				CopySourceRangeEnd:   lastByte,
			})
			if err == nil {
				completedParts[i] = obs.Part{
					ETag:       copyPartOutput.ETag,
					PartNumber: int(i + 1),
				}
			}
			errChan <- err
			<-limiter
		}()
	}

	for range completedParts {
		err := <-errChan
		if err != nil {
			return err
		}
	}
	_, err = d.Client.CompleteMultipartUpload(&obs.CompleteMultipartUploadInput{
		Bucket:   d.Bucket,
		Key:      d.OBSPath(destPath),
		UploadId: initOutput.UploadId,
		Parts:    completedParts,
	})
	return err
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
// We must be careful since obs does not guarantee read after delete consistency
func (d *driver) Delete(ctx context.Context, path string) error {
	objects := make([]obs.ObjectToDelete, 0, listMax)
	obsPath := d.OBSPath(path)
	listObjectsInput := &obs.ListObjectsInput{}
	listObjectsInput.Bucket = d.Bucket
	listObjectsInput.Prefix = obsPath

	for {
		// list all the objects
		output, err := d.Client.ListObjects(listObjectsInput)

		// output.Contents can only be empty on the first call
		// if there were no more results to return after the first call, resp.IsTruncated would have been false
		// and the loop would exit without recalling ListObjects
		if err != nil || len(output.Contents) == 0 {
			return storagedriver.PathNotFoundError{Path: path}
		}

		for _, content := range output.Contents {
			// Skip if we encounter a key that is not a subpath (so that deleting "/a" does not delete "/ab").
			if len(content.Key) > len(obsPath) && (content.Key)[len(obsPath)] != '/' {
				continue
			}
			objects = append(objects, obs.ObjectToDelete{
				Key: content.Key,
			})
		}

		// Delete objects only if the list is not empty, otherwise obs API returns a cryptic error
		if len(objects) > 0 {
			output, err := d.Client.DeleteObjects(&obs.DeleteObjectsInput{
				Bucket:  d.Bucket,
				Objects: objects,
				Quiet:   false,
			})
			if err != nil {
				return err
			}

			if len(output.Errors) > 0 {
				errs := make([]error, 0, len(output.Errors))
				for _, err := range output.Errors {
					errs = append(errs, errors.New(err.Message))
				}
				return storagedriver.Errors{
					DriverName: driverName,
					Errs:       errs,
				}
			}
		}
		// NOTE: we don't want to reallocate
		// the slice so we simply "reset" it
		objects = objects[:0]

		listObjectsInput.Marker = output.NextMarker

		// IsTruncated "specifies whether (true) or not (false) all of the results were returned"
		// if everything has been returned, break
		if !output.IsTruncated {
			break
		}
	}
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	methodString := http.MethodGet
	method, ok := options["method"]
	if ok {
		methodString, ok = method.(string)
		if !ok || (methodString != http.MethodGet && methodString != http.MethodHead) {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	expiresIn := 20 * time.Minute.Seconds()
	expires, ok := options["expiry"]
	if ok {
		et, ok := expires.(time.Time)
		if ok {
			expiresIn = time.Until(et).Seconds()
		}
	}
	output, err := d.Client.CreateSignedUrl(&obs.CreateSignedUrlInput{
		Bucket:  d.Bucket,
		Key:     d.OBSPath(path),
		Method:  obs.HttpMethodType(methodString),
		Expires: int(expiresIn),
	})
	if err != nil {
		return "", err
	}
	return output.SignedUrl, nil
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
}

func (d *driver) OBSPath(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

func (d *driver) parseError(path string, err error) error {
	if obsErr, ok := err.(obs.ObsError); ok {
		if obsErr.Code == "NoSuchKey" {
			return storagedriver.PathNotFoundError{Path: path}
		}
	}
	return err
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

func (d *driver) getACL() obs.AclType {
	return d.ObjectACL
}

func (d *driver) getStorageClass() obs.StorageClassType {
	if d.StorageClass == noStorageClass {
		return ""
	}
	return d.StorageClass
}

func (d *driver) getEncryptionMode() obs.ISseHeader {
	if !d.Encrypt {
		return nil
	}
	if d.EncryptionKeyID == "" {
		return obs.SseKmsHeader{}
	}
	return obs.SseCHeader{Key: d.EncryptionKeyID}
}

func (d *driver) getSSEKMSKeyID() string {
	if d.EncryptionKeyID != "" {
		return d.EncryptionKeyID
	}
	return ""
}

// writer attempts to upload parts to OBS in a buffered fashion where the last
// part is at least as large as the chunksize, so the multipart upload could be
// cleanly resumed in the future. This is violated if Close is called after less
// than a full chunk is written.
type writer struct {
	driver      *driver
	key         string
	uploadID    string
	parts       []obs.Part
	size        int64
	readyPart   []byte
	pendingPart []byte
	closed      bool
	committed   bool
	cancelled   bool
}

func (d *driver) newWriter(key, uploadID string, parts []obs.Part) storagedriver.FileWriter {
	var size int64
	for _, part := range parts {
		size += part.Size
	}
	return &writer{
		driver:   d,
		key:      key,
		uploadID: uploadID,
		parts:    parts,
		size:     size,
	}
}

type completedParts []obs.Part

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return a[i].PartNumber < a[j].PartNumber }

func (w *writer) Write(p []byte) (n int, err error) {
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
			completedUploadedParts = append(completedUploadedParts, obs.Part{
				ETag:       part.ETag,
				PartNumber: part.PartNumber,
			})
		}

		sort.Sort(completedUploadedParts)

		_, err := w.driver.Client.CompleteMultipartUpload(&obs.CompleteMultipartUploadInput{
			Bucket:   w.driver.Bucket,
			Key:      w.key,
			UploadId: w.uploadID,
			Parts:    completedUploadedParts,
		})
		if err != nil {
			w.driver.Client.AbortMultipartUpload(&obs.AbortMultipartUploadInput{
				Bucket:   w.driver.Bucket,
				Key:      w.key,
				UploadId: w.uploadID,
			})
			return 0, err
		}
		initInput := &obs.InitiateMultipartUploadInput{}
		initInput.Bucket = w.driver.Bucket
		initInput.Key = w.key
		initInput.ContentType = w.driver.getContentType()
		initInput.ACL = w.driver.getACL()
		initInput.StorageClass = w.driver.getStorageClass()
		initInput.SseHeader = w.driver.getEncryptionMode()
		initOutput, err := w.driver.Client.InitiateMultipartUpload(initInput)
		if err != nil {
			return 0, err
		}
		w.uploadID = initOutput.UploadId

		// If the entire written file is smaller than minChunkSize, we need to make
		// a new part from scratch :double sad face:
		if w.size < minChunkSize {
			getObjectInput := &obs.GetObjectInput{}
			getObjectInput.Bucket = w.driver.Bucket
			getObjectInput.Key = w.key
			getObjectOutput, err := w.driver.Client.GetObject(getObjectInput)
			if err != nil {
				return 0, err
			}
			defer getObjectOutput.Body.Close()
			w.parts = nil
			w.readyPart, err = io.ReadAll(getObjectOutput.Body)
			if err != nil {
				return 0, err
			}
		} else {
			// Otherwise we can use the old file as the new first part
			copyPartOutput, err := w.driver.Client.CopyPart(&obs.CopyPartInput{
				Bucket:           w.driver.Bucket,
				Key:              w.key,
				CopySourceBucket: w.driver.Bucket,
				CopySourceKey:    w.key,
				PartNumber:       1,
				UploadId:         w.uploadID,
			})
			if err != nil {
				return 0, err
			}
			w.parts = []obs.Part{
				{
					ETag:       copyPartOutput.ETag,
					PartNumber: 1,
					Size:       w.size,
				},
			}
		}
	}
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

// flushPart flushes buffers to write a part to OBS.
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
	ouput, err := w.driver.Client.UploadPart(&obs.UploadPartInput{
		Bucket:     w.driver.Bucket,
		Key:        w.key,
		PartNumber: len(w.parts) + 1,
		UploadId:   w.uploadID,
		Body:       bytes.NewReader(w.readyPart),
	})
	if err != nil {
		return err
	}
	w.parts = append(w.parts, obs.Part{
		ETag:       ouput.ETag,
		PartNumber: len(w.parts) + 1,
		Size:       int64(len(w.readyPart)),
	})
	w.readyPart = w.pendingPart
	w.pendingPart = nil
	return nil
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}
	w.closed = true
	return w.flushPart()
}

func (w *writer) Size() int64 {
	return w.size
}

func (w *writer) Cancel(ctx context.Context) error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true
	_, err := w.driver.Client.AbortMultipartUpload(&obs.AbortMultipartUploadInput{
		Bucket:   w.driver.Bucket,
		Key:      w.key,
		UploadId: w.uploadID,
	})
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

	var completedUploadedParts completedParts
	for _, part := range w.parts {
		completedUploadedParts = append(completedUploadedParts, obs.Part{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		})
	}

	sort.Sort(completedUploadedParts)

	_, err = w.driver.Client.CompleteMultipartUpload(&obs.CompleteMultipartUploadInput{
		Bucket:   w.driver.Bucket,
		Key:      w.key,
		UploadId: w.uploadID,
		Parts:    completedUploadedParts,
	})
	if err != nil {
		w.driver.Client.AbortMultipartUpload(&obs.AbortMultipartUploadInput{
			Bucket:   w.driver.Bucket,
			Key:      w.key,
			UploadId: w.uploadID,
		})
		return err
	}
	return nil
}

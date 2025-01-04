// Package s3 provides a storagedriver.StorageDriver implementation to
// store blobs in Amazon S3 cloud storage.
//
// This package leverages the official aws client library for interfacing with
// S3.
//
// Because S3 is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
//
// Keep in mind that S3 guarantees only read-after-write consistency for new
// objects, but no read-after-update or list-after-write consistency.
package s3

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/distribution/distribution/v3/internal/dcontext"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
)

const driverName = "s3aws"

// minChunkSize defines the minimum multipart upload chunk size
// S3 API requires multipart upload chunks to be at least 5MB
const minChunkSize = 5 * 1024 * 1024

const defaultChunkSize = 2 * minChunkSize

const (
	// defaultMultipartCopyChunkSize defines the default chunk size for all
	// but the last Upload Part - Copy operation of a multipart copy.
	// Empirically, 32 MB is optimal.
	defaultMultipartCopyChunkSize = 32 * 1024 * 1024

	// defaultMultipartCopyMaxConcurrency defines the default maximum number
	// of concurrent Upload Part - Copy operations for a multipart copy.
	defaultMultipartCopyMaxConcurrency = 100

	// defaultMultipartCopyThresholdSize defines the default object size
	// above which multipart copy will be used. (PUT Object - Copy is used
	// for objects at or below this size.)  Empirically, 32 MB is optimal.
	defaultMultipartCopyThresholdSize = 32 * 1024 * 1024
)

// listMax is the largest amount of objects you can request from S3 in a list call
const listMax = 1000

// noStorageClass defines the value to be used if storage class is not supported by the S3 endpoint
const noStorageClass = "NONE"

// s3StorageClasses lists all compatible (instant retrieval) S3 storage classes
var s3StorageClasses = []string{
	noStorageClass,
	s3.StorageClassStandard,
	s3.StorageClassReducedRedundancy,
	s3.StorageClassStandardIa,
	s3.StorageClassOnezoneIa,
	s3.StorageClassIntelligentTiering,
	s3.StorageClassOutposts,
	s3.StorageClassGlacierIr,
}

// validRegions maps known s3 region identifiers to region descriptors
var validRegions = map[string]struct{}{}

// validObjectACLs contains known s3 object Acls
var validObjectACLs = map[string]struct{}{}

// DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	AccessKey                   string
	SecretKey                   string
	Bucket                      string
	Region                      string
	RegionEndpoint              string
	ForcePathStyle              bool
	Encrypt                     bool
	KeyID                       string
	Secure                      bool
	SkipVerify                  bool
	V4Auth                      bool
	ChunkSize                   int
	MultipartCopyChunkSize      int64
	MultipartCopyMaxConcurrency int64
	MultipartCopyThresholdSize  int64
	RootDirectory               string
	StorageClass                string
	UserAgent                   string
	ObjectACL                   string
	SessionToken                string
	UseDualStack                bool
	Accelerate                  bool
	LogLevel                    aws.LogLevelType
}

func init() {
	partitions := endpoints.DefaultPartitions()
	for _, p := range partitions {
		for region := range p.Regions() {
			validRegions[region] = struct{}{}
		}
	}

	for _, objectACL := range []string{
		s3.ObjectCannedACLPrivate,
		s3.ObjectCannedACLPublicRead,
		s3.ObjectCannedACLPublicReadWrite,
		s3.ObjectCannedACLAuthenticatedRead,
		s3.ObjectCannedACLAwsExecRead,
		s3.ObjectCannedACLBucketOwnerRead,
		s3.ObjectCannedACLBucketOwnerFullControl,
	} {
		validObjectACLs[objectACL] = struct{}{}
	}

	// Register this as the default s3 driver in addition to s3aws
	factory.Register("s3", &s3DriverFactory{})
	factory.Register(driverName, &s3DriverFactory{})
}

// s3DriverFactory implements the factory.StorageDriverFactory interface
type s3DriverFactory struct{}

func (factory *s3DriverFactory) Create(ctx context.Context, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(ctx, parameters)
}

var _ storagedriver.StorageDriver = &driver{}

type driver struct {
	S3                          *s3.S3
	Bucket                      string
	ChunkSize                   int
	Encrypt                     bool
	KeyID                       string
	MultipartCopyChunkSize      int64
	MultipartCopyMaxConcurrency int64
	MultipartCopyThresholdSize  int64
	RootDirectory               string
	StorageClass                string
	ObjectACL                   string
	pool                        *sync.Pool
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
func FromParameters(ctx context.Context, parameters map[string]interface{}) (*Driver, error) {
	// Providing no values for these is valid in case the user is authenticating
	// with an IAM on an ec2 instance (in which case the instance credentials will
	// be summoned when GetAuth is called)
	accessKey := parameters["accesskey"]
	if accessKey == nil {
		accessKey = ""
	}
	secretKey := parameters["secretkey"]
	if secretKey == nil {
		secretKey = ""
	}

	regionEndpoint := parameters["regionendpoint"]
	if regionEndpoint == nil {
		regionEndpoint = ""
	}

	forcePathStyleBool := false
	forcePathStyle := parameters["forcepathstyle"]
	switch forcePathStyle := forcePathStyle.(type) {
	case string:
		b, err := strconv.ParseBool(forcePathStyle)
		if err != nil {
			return nil, fmt.Errorf("the forcePathStyle parameter should be a boolean")
		}
		forcePathStyleBool = b
	case bool:
		forcePathStyleBool = forcePathStyle
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the forcePathStyle parameter should be a boolean")
	}

	regionName := parameters["region"]
	region := fmt.Sprint(regionName)

	// Don't check the region value if a custom endpoint is provided.
	if regionEndpoint == "" {
		if regionName == nil || region == "" {
			return nil, fmt.Errorf("no region parameter provided")
		}
		if _, ok := validRegions[region]; !ok {
			return nil, fmt.Errorf("invalid region provided: %v", region)
		}
	}

	bucket := parameters["bucket"]
	if bucket == nil || fmt.Sprint(bucket) == "" {
		return nil, fmt.Errorf("no bucket parameter provided")
	}

	encryptBool := false
	encrypt := parameters["encrypt"]
	switch encrypt := encrypt.(type) {
	case string:
		b, err := strconv.ParseBool(encrypt)
		if err != nil {
			return nil, fmt.Errorf("the encrypt parameter should be a boolean")
		}
		encryptBool = b
	case bool:
		encryptBool = encrypt
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the encrypt parameter should be a boolean")
	}

	secureBool := true
	secure := parameters["secure"]
	switch secure := secure.(type) {
	case string:
		b, err := strconv.ParseBool(secure)
		if err != nil {
			return nil, fmt.Errorf("the secure parameter should be a boolean")
		}
		secureBool = b
	case bool:
		secureBool = secure
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the secure parameter should be a boolean")
	}

	skipVerifyBool := false
	skipVerify := parameters["skipverify"]
	switch skipVerify := skipVerify.(type) {
	case string:
		b, err := strconv.ParseBool(skipVerify)
		if err != nil {
			return nil, fmt.Errorf("the skipVerify parameter should be a boolean")
		}
		skipVerifyBool = b
	case bool:
		skipVerifyBool = skipVerify
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the skipVerify parameter should be a boolean")
	}

	v4Bool := true
	v4auth := parameters["v4auth"]
	switch v4auth := v4auth.(type) {
	case string:
		b, err := strconv.ParseBool(v4auth)
		if err != nil {
			return nil, fmt.Errorf("the v4auth parameter should be a boolean")
		}
		v4Bool = b
	case bool:
		v4Bool = v4auth
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the v4auth parameter should be a boolean")
	}

	keyID := parameters["keyid"]
	if keyID == nil {
		keyID = ""
	}

	chunkSize, err := getParameterAsInteger(parameters, "chunksize", defaultChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	multipartCopyChunkSize, err := getParameterAsInteger[int64](parameters, "multipartcopychunksize", defaultMultipartCopyChunkSize, minChunkSize, maxChunkSize)
	if err != nil {
		return nil, err
	}

	multipartCopyMaxConcurrency, err := getParameterAsInteger[int64](parameters, "multipartcopymaxconcurrency", defaultMultipartCopyMaxConcurrency, 1, math.MaxInt64)
	if err != nil {
		return nil, err
	}

	multipartCopyThresholdSize, err := getParameterAsInteger[int64](parameters, "multipartcopythresholdsize", defaultMultipartCopyThresholdSize, 0, maxChunkSize)
	if err != nil {
		return nil, err
	}

	rootDirectory := parameters["rootdirectory"]
	if rootDirectory == nil {
		rootDirectory = ""
	}

	storageClass := s3.StorageClassStandard
	storageClassParam := parameters["storageclass"]
	if storageClassParam != nil {
		storageClassString, ok := storageClassParam.(string)
		if !ok {
			return nil, fmt.Errorf(
				"the storageclass parameter must be one of %v, %v invalid",
				s3StorageClasses,
				storageClassParam,
			)
		}
		// All valid storage class parameters are UPPERCASE, so be a bit more flexible here
		storageClassString = strings.ToUpper(storageClassString)
		if storageClassString != noStorageClass &&
			storageClassString != s3.StorageClassStandard &&
			storageClassString != s3.StorageClassReducedRedundancy &&
			storageClassString != s3.StorageClassStandardIa &&
			storageClassString != s3.StorageClassOnezoneIa &&
			storageClassString != s3.StorageClassIntelligentTiering &&
			storageClassString != s3.StorageClassOutposts &&
			storageClassString != s3.StorageClassGlacierIr {
			return nil, fmt.Errorf(
				"the storageclass parameter must be one of %v, %v invalid",
				s3StorageClasses,
				storageClassParam,
			)
		}
		storageClass = storageClassString
	}

	userAgent := parameters["useragent"]
	if userAgent == nil {
		userAgent = ""
	}

	objectACL := s3.ObjectCannedACLPrivate
	objectACLParam := parameters["objectacl"]
	if objectACLParam != nil {
		objectACLString, ok := objectACLParam.(string)
		if !ok {
			return nil, fmt.Errorf("invalid value for objectacl parameter: %v", objectACLParam)
		}

		if _, ok = validObjectACLs[objectACLString]; !ok {
			return nil, fmt.Errorf("invalid value for objectacl parameter: %v", objectACLParam)
		}
		objectACL = objectACLString
	}

	useDualStackBool := false
	useDualStack := parameters["usedualstack"]
	switch useDualStack := useDualStack.(type) {
	case string:
		b, err := strconv.ParseBool(useDualStack)
		if err != nil {
			return nil, fmt.Errorf("the useDualStack parameter should be a boolean")
		}
		useDualStackBool = b
	case bool:
		useDualStackBool = useDualStack
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the useDualStack parameter should be a boolean")
	}

	sessionToken := ""

	accelerateBool := false
	accelerate := parameters["accelerate"]
	switch accelerate := accelerate.(type) {
	case string:
		b, err := strconv.ParseBool(accelerate)
		if err != nil {
			return nil, fmt.Errorf("the accelerate parameter should be a boolean")
		}
		accelerateBool = b
	case bool:
		accelerateBool = accelerate
	case nil:
		// do nothing
	default:
		return nil, fmt.Errorf("the accelerate parameter should be a boolean")
	}

	params := DriverParameters{
		AccessKey:                   fmt.Sprint(accessKey),
		SecretKey:                   fmt.Sprint(secretKey),
		Bucket:                      fmt.Sprint(bucket),
		Region:                      region,
		RegionEndpoint:              fmt.Sprint(regionEndpoint),
		ForcePathStyle:              forcePathStyleBool,
		Encrypt:                     encryptBool,
		KeyID:                       fmt.Sprint(keyID),
		Secure:                      secureBool,
		SkipVerify:                  skipVerifyBool,
		V4Auth:                      v4Bool,
		ChunkSize:                   chunkSize,
		MultipartCopyChunkSize:      multipartCopyChunkSize,
		MultipartCopyMaxConcurrency: multipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  multipartCopyThresholdSize,
		RootDirectory:               fmt.Sprint(rootDirectory),
		StorageClass:                storageClass,
		UserAgent:                   fmt.Sprint(userAgent),
		ObjectACL:                   objectACL,
		SessionToken:                fmt.Sprint(sessionToken),
		UseDualStack:                useDualStackBool,
		Accelerate:                  accelerateBool,
		LogLevel:                    getS3LogLevelFromParam(parameters["loglevel"]),
	}

	return New(ctx, params)
}

func getS3LogLevelFromParam(param interface{}) aws.LogLevelType {
	if param == nil {
		return aws.LogOff
	}
	logLevelParam := param.(string)
	var logLevel aws.LogLevelType
	switch strings.ToLower(logLevelParam) {
	case "off":
		logLevel = aws.LogOff
	case "debug":
		logLevel = aws.LogDebug
	case "debugwithsigning":
		logLevel = aws.LogDebugWithSigning
	case "debugwithhttpbody":
		logLevel = aws.LogDebugWithHTTPBody
	case "debugwithrequestretries":
		logLevel = aws.LogDebugWithRequestRetries
	case "debugwithrequesterrors":
		logLevel = aws.LogDebugWithRequestErrors
	case "debugwitheventstreambody":
		logLevel = aws.LogDebugWithEventStreamBody
	default:
		logLevel = aws.LogOff
	}
	return logLevel
}

type integer interface{ signed | unsigned }

type signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// getParameterAsInteger converts parameters[name] to T (using defaultValue if
// nil) and ensures it is in the range of min and max.
func getParameterAsInteger[T integer](parameters map[string]any, name string, defaultValue, min, max T) (T, error) {
	v := defaultValue
	if p := parameters[name]; p != nil {
		if _, err := fmt.Sscanf(fmt.Sprint(p), "%d", &v); err != nil {
			return 0, fmt.Errorf("%s parameter must be an integer, %v invalid", name, p)
		}
	}
	if v < min || v > max {
		return 0, fmt.Errorf("the %s %#v parameter should be a number between %d and %d (inclusive)", name, v, min, max)
	}
	return v, nil
}

// New constructs a new Driver with the given AWS credentials, region, encryption flag, and
// bucketName
func New(ctx context.Context, params DriverParameters) (*Driver, error) {
	if !params.V4Auth &&
		(params.RegionEndpoint == "" ||
			strings.Contains(params.RegionEndpoint, "s3.amazonaws.com")) {
		return nil, fmt.Errorf("on Amazon S3 this storage driver can only be used with v4 authentication")
	}

	awsConfig := aws.NewConfig().WithLogLevel(params.LogLevel)

	if params.AccessKey != "" && params.SecretKey != "" {
		creds := credentials.NewStaticCredentials(
			params.AccessKey,
			params.SecretKey,
			params.SessionToken,
		)
		awsConfig.WithCredentials(creds)
	}

	if params.RegionEndpoint != "" {
		awsConfig.WithEndpoint(params.RegionEndpoint)
	}

	awsConfig.WithS3ForcePathStyle(params.ForcePathStyle)
	awsConfig.WithS3UseAccelerate(params.Accelerate)
	awsConfig.WithRegion(params.Region)
	awsConfig.WithDisableSSL(!params.Secure)
	if params.UseDualStack {
		awsConfig.UseDualStackEndpoint = endpoints.DualStackEndpointStateEnabled
	}

	if params.SkipVerify {
		httpTransport := http.DefaultTransport.(*http.Transport).Clone()
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		awsConfig.WithHTTPClient(&http.Client{
			Transport: httpTransport,
		})
	}

	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create new session with aws config: %v", err)
	}

	if params.UserAgent != "" {
		sess.Handlers.Build.PushBack(request.MakeAddToUserAgentFreeFormHandler(params.UserAgent))
	}

	s3obj := s3.New(sess)

	// enable S3 compatible signature v2 signing instead
	if !params.V4Auth {
		setv2Handlers(s3obj)
	}

	// TODO Currently multipart uploads have no timestamps, so this would be unwise
	// if you initiated a new s3driver while another one is running on the same bucket.
	// multis, _, err := bucket.ListMulti("", "")
	// if err != nil {
	// 	return nil, err
	// }

	// for _, multi := range multis {
	// 	err := multi.Abort()
	// 	//TODO appropriate to do this error checking?
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	d := &driver{
		S3:                          s3obj,
		Bucket:                      params.Bucket,
		ChunkSize:                   params.ChunkSize,
		Encrypt:                     params.Encrypt,
		KeyID:                       params.KeyID,
		MultipartCopyChunkSize:      params.MultipartCopyChunkSize,
		MultipartCopyMaxConcurrency: params.MultipartCopyMaxConcurrency,
		MultipartCopyThresholdSize:  params.MultipartCopyThresholdSize,
		RootDirectory:               params.RootDirectory,
		StorageClass:                params.StorageClass,
		ObjectACL:                   params.ObjectACL,
		pool: &sync.Pool{
			New: func() any { return &bytes.Buffer{} },
		},
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
	return io.ReadAll(reader)
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	_, err := d.S3.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(d.Bucket),
		Key:                  aws.String(d.s3Path(path)),
		ContentType:          d.getContentType(),
		ACL:                  d.getACL(),
		ServerSideEncryption: d.getEncryptionMode(),
		SSEKMSKeyId:          d.getSSEKMSKeyID(),
		StorageClass:         d.getStorageClass(),
		Body:                 bytes.NewReader(contents),
	})
	return parseError(path, err)
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	resp, err := d.S3.GetObjectWithContext(ctx, &s3.GetObjectInput{
		Bucket: aws.String(d.Bucket),
		Key:    aws.String(d.s3Path(path)),
		Range:  aws.String("bytes=" + strconv.FormatInt(offset, 10) + "-"),
	})
	if err != nil {
		if s3Err, ok := err.(awserr.Error); ok && s3Err.Code() == "InvalidRange" {
			return io.NopCloser(bytes.NewReader(nil)), nil
		}

		return nil, parseError(path, err)
	}
	return resp.Body, nil
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
// It only allows appending to paths with zero size committed content,
// in which the existing content is overridden with the new content.
// It returns storagedriver.Error when appending to paths
// with non-zero committed content.
func (d *driver) Writer(ctx context.Context, path string, appendMode bool) (storagedriver.FileWriter, error) {
	key := d.s3Path(path)
	if !appendMode {
		// TODO (brianbland): cancel other uploads at this path
		resp, err := d.S3.CreateMultipartUploadWithContext(ctx, &s3.CreateMultipartUploadInput{
			Bucket:               aws.String(d.Bucket),
			Key:                  aws.String(key),
			ContentType:          d.getContentType(),
			ACL:                  d.getACL(),
			ServerSideEncryption: d.getEncryptionMode(),
			SSEKMSKeyId:          d.getSSEKMSKeyID(),
			StorageClass:         d.getStorageClass(),
		})
		if err != nil {
			return nil, err
		}
		return d.newWriter(ctx, key, *resp.UploadId, nil), nil
	}

	listMultipartUploadsInput := &s3.ListMultipartUploadsInput{
		Bucket: aws.String(d.Bucket),
		Prefix: aws.String(key),
	}
	for {
		resp, err := d.S3.ListMultipartUploadsWithContext(ctx, listMultipartUploadsInput)
		if err != nil {
			return nil, parseError(path, err)
		}

		// resp.Uploads can only be empty on the first call
		// if there were no more results to return after the first call, resp.IsTruncated would have been false
		// and the loop would be exited without recalling ListMultipartUploads
		if len(resp.Uploads) == 0 {
			fi, err := d.Stat(ctx, path)
			if err != nil {
				return nil, parseError(path, err)
			}

			if fi.Size() == 0 {
				resp, err := d.S3.CreateMultipartUploadWithContext(ctx, &s3.CreateMultipartUploadInput{
					Bucket:               aws.String(d.Bucket),
					Key:                  aws.String(key),
					ContentType:          d.getContentType(),
					ACL:                  d.getACL(),
					ServerSideEncryption: d.getEncryptionMode(),
					SSEKMSKeyId:          d.getSSEKMSKeyID(),
					StorageClass:         d.getStorageClass(),
				})
				if err != nil {
					return nil, err
				}
				return d.newWriter(ctx, key, *resp.UploadId, nil), nil
			}
			return nil, storagedriver.Error{
				DriverName: driverName,
				Detail:     fmt.Errorf("append to zero-size path %s unsupported", path),
			}
		}

		var allParts []*s3.Part
		for _, multi := range resp.Uploads {
			if key != *multi.Key {
				continue
			}

			partsList, err := d.S3.ListPartsWithContext(ctx, &s3.ListPartsInput{
				Bucket:   aws.String(d.Bucket),
				Key:      aws.String(key),
				UploadId: multi.UploadId,
			})
			if err != nil {
				return nil, parseError(path, err)
			}
			allParts = append(allParts, partsList.Parts...)
			for *partsList.IsTruncated {
				partsList, err = d.S3.ListPartsWithContext(ctx, &s3.ListPartsInput{
					Bucket:           aws.String(d.Bucket),
					Key:              aws.String(key),
					UploadId:         multi.UploadId,
					PartNumberMarker: partsList.NextPartNumberMarker,
				})
				if err != nil {
					return nil, parseError(path, err)
				}
				allParts = append(allParts, partsList.Parts...)
			}
			return d.newWriter(ctx, key, *multi.UploadId, allParts), nil
		}

		// resp.NextUploadIdMarker must have at least one element or we would have returned not found
		listMultipartUploadsInput.UploadIdMarker = resp.NextUploadIdMarker

		// from the s3 api docs, IsTruncated "specifies whether (true) or not (false) all of the results were returned"
		// if everything has been returned, break
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
	}
	return nil, storagedriver.PathNotFoundError{Path: path}
}

func (d *driver) statHead(ctx context.Context, path string) (*storagedriver.FileInfoFields, error) {
	resp, err := d.S3.HeadObjectWithContext(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(d.Bucket),
		Key:    aws.String(d.s3Path(path)),
	})
	if err != nil {
		return nil, err
	}
	return &storagedriver.FileInfoFields{
		Path:    path,
		IsDir:   false,
		Size:    *resp.ContentLength,
		ModTime: *resp.LastModified,
	}, nil
}

func (d *driver) statList(ctx context.Context, path string) (*storagedriver.FileInfoFields, error) {
	s3Path := d.s3Path(path)
	resp, err := d.S3.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(d.Bucket),
		Prefix:  aws.String(s3Path),
		MaxKeys: aws.Int64(1),
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Contents) == 1 {
		if *resp.Contents[0].Key != s3Path {
			return &storagedriver.FileInfoFields{
				Path:  path,
				IsDir: true,
			}, nil
		}
		return &storagedriver.FileInfoFields{
			Path:    path,
			Size:    *resp.Contents[0].Size,
			ModTime: *resp.Contents[0].LastModified,
		}, nil
	}
	if len(resp.CommonPrefixes) == 1 {
		return &storagedriver.FileInfoFields{
			Path:  path,
			IsDir: true,
		}, nil
	}
	return nil, storagedriver.PathNotFoundError{Path: path}
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	fi, err := d.statHead(ctx, path)
	if err != nil {
		// For AWS errors, we fail over to ListObjects:
		// Though the official docs https://docs.aws.amazon.com/AmazonS3/latest/API/API_HeadObject.html#API_HeadObject_Errors
		// are slightly outdated, the HeadObject actually returns NotFound error
		// if querying a key which doesn't exist or a key which has nested keys
		// and Forbidden if IAM/ACL permissions do not allow Head but allow List.
		var awsErr awserr.Error
		if errors.As(err, &awsErr) {
			fi, err := d.statList(ctx, path)
			if err != nil {
				return nil, parseError(path, err)
			}
			return storagedriver.FileInfoInternal{FileInfoFields: *fi}, nil
		}
		// For non-AWS errors, return the error directly
		return nil, err
	}
	return storagedriver.FileInfoInternal{FileInfoFields: *fi}, nil
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

	resp, err := d.S3.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(d.Bucket),
		Prefix:    aws.String(d.s3Path(path)),
		Delimiter: aws.String("/"),
		MaxKeys:   aws.Int64(listMax),
	})
	if err != nil {
		return nil, parseError(opath, err)
	}

	files := []string{}
	directories := []string{}

	for {
		for _, key := range resp.Contents {
			files = append(files, strings.Replace(*key.Key, d.s3Path(""), prefix, 1))
		}

		for _, commonPrefix := range resp.CommonPrefixes {
			commonPrefix := *commonPrefix.Prefix
			directories = append(directories, strings.Replace(commonPrefix[0:len(commonPrefix)-1], d.s3Path(""), prefix, 1))
		}

		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}

		resp, err = d.S3.ListObjectsV2WithContext(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(d.Bucket),
			Prefix:            aws.String(d.s3Path(path)),
			Delimiter:         aws.String("/"),
			MaxKeys:           aws.Int64(listMax),
			ContinuationToken: resp.NextContinuationToken,
		})
		if err != nil {
			return nil, err
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
func (d *driver) Move(ctx context.Context, sourcePath, destPath string) error {
	/* This is terrible, but aws doesn't have an actual move. */
	if err := d.copy(ctx, sourcePath, destPath); err != nil {
		return err
	}
	return d.Delete(ctx, sourcePath)
}

// copy copies an object stored at sourcePath to destPath.
func (d *driver) copy(ctx context.Context, sourcePath, destPath string) error {
	// S3 can copy objects up to 5 GB in size with a single PUT Object - Copy
	// operation. For larger objects, the multipart upload API must be used.
	//
	// Empirically, multipart copy is fastest with 32 MB parts and is faster
	// than PUT Object - Copy for objects larger than 32 MB.

	fileInfo, err := d.Stat(ctx, sourcePath)
	if err != nil {
		return parseError(sourcePath, err)
	}

	if fileInfo.Size() <= d.MultipartCopyThresholdSize {
		_, err := d.S3.CopyObjectWithContext(ctx, &s3.CopyObjectInput{
			Bucket:               aws.String(d.Bucket),
			Key:                  aws.String(d.s3Path(destPath)),
			ContentType:          d.getContentType(),
			ACL:                  d.getACL(),
			ServerSideEncryption: d.getEncryptionMode(),
			SSEKMSKeyId:          d.getSSEKMSKeyID(),
			StorageClass:         d.getStorageClass(),
			CopySource:           aws.String(d.Bucket + "/" + d.s3Path(sourcePath)),
		})
		if err != nil {
			return parseError(sourcePath, err)
		}
		return nil
	}

	createResp, err := d.S3.CreateMultipartUploadWithContext(ctx, &s3.CreateMultipartUploadInput{
		Bucket:               aws.String(d.Bucket),
		Key:                  aws.String(d.s3Path(destPath)),
		ContentType:          d.getContentType(),
		ACL:                  d.getACL(),
		SSEKMSKeyId:          d.getSSEKMSKeyID(),
		ServerSideEncryption: d.getEncryptionMode(),
		StorageClass:         d.getStorageClass(),
	})
	if err != nil {
		return err
	}

	numParts := (fileInfo.Size() + d.MultipartCopyChunkSize - 1) / d.MultipartCopyChunkSize
	completedParts := make([]*s3.CompletedPart, numParts)
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
			uploadResp, err := d.S3.UploadPartCopyWithContext(ctx, &s3.UploadPartCopyInput{
				Bucket:          aws.String(d.Bucket),
				CopySource:      aws.String(d.Bucket + "/" + d.s3Path(sourcePath)),
				Key:             aws.String(d.s3Path(destPath)),
				PartNumber:      aws.Int64(i + 1),
				UploadId:        createResp.UploadId,
				CopySourceRange: aws.String(fmt.Sprintf("bytes=%d-%d", firstByte, lastByte)),
			})
			if err == nil {
				completedParts[i] = &s3.CompletedPart{
					ETag:       uploadResp.CopyPartResult.ETag,
					PartNumber: aws.Int64(i + 1),
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

	_, err = d.S3.CompleteMultipartUploadWithContext(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(d.Bucket),
		Key:             aws.String(d.s3Path(destPath)),
		UploadId:        createResp.UploadId,
		MultipartUpload: &s3.CompletedMultipartUpload{Parts: completedParts},
	})
	return err
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
// We must be careful since S3 does not guarantee read after delete consistency
func (d *driver) Delete(ctx context.Context, path string) error {
	s3Objects := make([]*s3.ObjectIdentifier, 0, listMax)
	s3Path := d.s3Path(path)
	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(d.Bucket),
		Prefix: aws.String(s3Path),
	}

	for {
		// list all the objects
		resp, err := d.S3.ListObjectsV2WithContext(ctx, listObjectsInput)

		// resp.Contents can only be empty on the first call
		// if there were no more results to return after the first call, resp.IsTruncated would have been false
		// and the loop would exit without recalling ListObjects
		if err != nil || len(resp.Contents) == 0 {
			return storagedriver.PathNotFoundError{Path: path}
		}

		for _, key := range resp.Contents {
			// Skip if we encounter a key that is not a subpath (so that deleting "/a" does not delete "/ab").
			if len(*key.Key) > len(s3Path) && (*key.Key)[len(s3Path)] != '/' {
				continue
			}
			s3Objects = append(s3Objects, &s3.ObjectIdentifier{
				Key: key.Key,
			})
		}

		// Delete objects only if the list is not empty, otherwise S3 API returns a cryptic error
		if len(s3Objects) > 0 {
			// NOTE: according to AWS docs https://docs.aws.amazon.com/AmazonS3/latest/API/API_ListObjectsV2.html
			// by default the response returns up to 1,000 key names. The response _might_ contain fewer keys but it will never contain more.
			// 10000 keys is coincidentally (?) also the max number of keys that can be deleted in a single Delete operation, so we'll just smack
			// Delete here straight away and reset the object slice when successful.
			resp, err := d.S3.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(d.Bucket),
				Delete: &s3.Delete{
					Objects: s3Objects,
					Quiet:   aws.Bool(false),
				},
			})
			if err != nil {
				return err
			}

			if len(resp.Errors) > 0 {
				// NOTE: AWS SDK s3.Error does not implement error interface which
				// is pretty intensely sad, so we have to do away with this for now.
				errs := make([]error, 0, len(resp.Errors))
				for _, err := range resp.Errors {
					errs = append(errs, errors.New(err.String()))
				}
				return storagedriver.Errors{
					DriverName: driverName,
					Errs:       errs,
				}
			}
		}
		// NOTE: we don't want to reallocate
		// the slice so we simply "reset" it
		s3Objects = s3Objects[:0]

		// resp.Contents must have at least one element or we would have returned not found
		listObjectsInput.StartAfter = resp.Contents[len(resp.Contents)-1].Key

		// from the s3 api docs, IsTruncated "specifies whether (true) or not (false) all of the results were returned"
		// if everything has been returned, break
		if resp.IsTruncated == nil || !*resp.IsTruncated {
			break
		}
	}

	return nil
}

// RedirectURL returns a URL which may be used to retrieve the content stored at the given path.
func (d *driver) RedirectURL(r *http.Request, path string) (string, error) {
	expiresIn := 20 * time.Minute

	var req *request.Request

	switch r.Method {
	case http.MethodGet:
		req, _ = d.S3.GetObjectRequest(&s3.GetObjectInput{
			Bucket: aws.String(d.Bucket),
			Key:    aws.String(d.s3Path(path)),
		})
	case http.MethodHead:
		req, _ = d.S3.HeadObjectRequest(&s3.HeadObjectInput{
			Bucket: aws.String(d.Bucket),
			Key:    aws.String(d.s3Path(path)),
		})
	default:
		return "", nil
	}

	return req.Presign(expiresIn)
}

// Walk traverses a filesystem defined within driver, starting
// from the given path, calling f on each file
func (d *driver) Walk(ctx context.Context, from string, f storagedriver.WalkFn, options ...func(*storagedriver.WalkOptions)) error {
	walkOptions := &storagedriver.WalkOptions{}
	for _, o := range options {
		o(walkOptions)
	}

	var objectCount int64
	if err := d.doWalk(ctx, &objectCount, from, walkOptions.StartAfterHint, f); err != nil {
		return err
	}

	return nil
}

func (d *driver) doWalk(parentCtx context.Context, objectCount *int64, from, startAfter string, f storagedriver.WalkFn) error {
	var (
		retError error
		// the most recent directory walked for de-duping
		prevDir string
		// the most recent skip directory to avoid walking over undesirable files
		prevSkipDir string
	)
	prevDir = from

	path := from
	if !strings.HasSuffix(path, "/") {
		path = path + "/"
	}

	prefix := ""
	if d.s3Path("") == "" {
		prefix = "/"
	}

	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket:     aws.String(d.Bucket),
		Prefix:     aws.String(d.s3Path(path)),
		MaxKeys:    aws.Int64(listMax),
		StartAfter: aws.String(d.s3Path(startAfter)),
	}

	ctx, done := dcontext.WithTrace(parentCtx)
	defer done("s3aws.ListObjectsV2PagesWithContext(%s)", listObjectsInput)

	// When the "delimiter" argument is omitted, the S3 list API will list all objects in the bucket
	// recursively, omitting directory paths. Objects are listed in sorted, depth-first order so we
	// can infer all the directories by comparing each object path to the last one we saw.
	// See: https://docs.aws.amazon.com/AmazonS3/latest/userguide/ListingKeysUsingAPIs.html

	// With files returned in sorted depth-first order, directories are inferred in the same order.
	// ErrSkipDir is handled by explicitly skipping over any files under the skipped directory. This may be sub-optimal
	// for extreme edge cases but for the general use case in a registry, this is orders of magnitude
	// faster than a more explicit recursive implementation.
	listObjectErr := d.S3.ListObjectsV2PagesWithContext(ctx, listObjectsInput, func(objects *s3.ListObjectsV2Output, lastPage bool) bool {
		walkInfos := make([]storagedriver.FileInfoInternal, 0, len(objects.Contents))

		for _, file := range objects.Contents {
			filePath := strings.Replace(*file.Key, d.s3Path(""), prefix, 1)

			// get a list of all inferred directories between the previous directory and this file
			dirs := directoryDiff(prevDir, filePath)
			for _, dir := range dirs {
				walkInfos = append(walkInfos, storagedriver.FileInfoInternal{
					FileInfoFields: storagedriver.FileInfoFields{
						IsDir: true,
						Path:  dir,
					},
				})
				prevDir = dir
			}

			// in some cases the _uploads dir might be empty. when this happens, it would
			// be appended twice to the walkInfos slice, once as [...]/_uploads and
			// once more erroneously as [...]/_uploads/. the easiest way to avoid this is
			// to skip appending filePath to walkInfos if it ends in "/". the loop through
			// dirs will already have handled it in that case, so it's safe to continue this
			// loop.
			if strings.HasSuffix(filePath, "/") {
				continue
			}

			walkInfos = append(walkInfos, storagedriver.FileInfoInternal{
				FileInfoFields: storagedriver.FileInfoFields{
					IsDir:   false,
					Size:    *file.Size,
					ModTime: *file.LastModified,
					Path:    filePath,
				},
			})
		}

		for _, walkInfo := range walkInfos {
			// skip any results under the last skip directory
			if prevSkipDir != "" && strings.HasPrefix(walkInfo.Path(), prevSkipDir) {
				continue
			}

			err := f(walkInfo)
			*objectCount++

			if err != nil {
				if err == storagedriver.ErrSkipDir {
					prevSkipDir = walkInfo.Path()
					continue
				}
				if err == storagedriver.ErrFilledBuffer {
					return false
				}
				retError = err
				return false
			}
		}
		return true
	})

	if retError != nil {
		return retError
	}

	if listObjectErr != nil {
		return listObjectErr
	}

	return nil
}

// directoryDiff finds all directories that are not in common between
// the previous and current paths in sorted order.
//
// # Examples
//
//	directoryDiff("/path/to/folder", "/path/to/folder/folder/file")
//	// => [ "/path/to/folder/folder" ]
//
//	directoryDiff("/path/to/folder/folder1", "/path/to/folder/folder2/file")
//	// => [ "/path/to/folder/folder2" ]
//
//	directoryDiff("/path/to/folder/folder1/file", "/path/to/folder/folder2/file")
//	// => [ "/path/to/folder/folder2" ]
//
//	directoryDiff("/path/to/folder/folder1/file", "/path/to/folder/folder2/folder1/file")
//	// => [ "/path/to/folder/folder2", "/path/to/folder/folder2/folder1" ]
//
//	directoryDiff("/", "/path/to/folder/folder/file")
//	// => [ "/path", "/path/to", "/path/to/folder", "/path/to/folder/folder" ]
func directoryDiff(prev, current string) []string {
	var paths []string

	if prev == "" || current == "" {
		return paths
	}

	parent := current
	for {
		parent = filepath.Dir(parent)
		if parent == "/" || parent == prev || strings.HasPrefix(prev+"/", parent+"/") {
			break
		}
		paths = append(paths, parent)
	}
	slices.Reverse(paths)
	return paths
}

func (d *driver) s3Path(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.RootDirectory, "/")+path, "/")
}

// S3BucketKey returns the s3 bucket key for the given storage driver path.
func (d *Driver) S3BucketKey(path string) string {
	return d.StorageDriver.(*driver).s3Path(path)
}

func parseError(path string, err error) error {
	if s3Err, ok := err.(awserr.Error); ok && s3Err.Code() == "NoSuchKey" {
		return storagedriver.PathNotFoundError{Path: path}
	}

	return err
}

func (d *driver) getEncryptionMode() *string {
	if !d.Encrypt {
		return nil
	}
	if d.KeyID == "" {
		return aws.String("AES256")
	}
	return aws.String("aws:kms")
}

func (d *driver) getSSEKMSKeyID() *string {
	if d.KeyID != "" {
		return aws.String(d.KeyID)
	}
	return nil
}

func (d *driver) getContentType() *string {
	return aws.String("application/octet-stream")
}

func (d *driver) getACL() *string {
	return aws.String(d.ObjectACL)
}

func (d *driver) getStorageClass() *string {
	if d.StorageClass == noStorageClass {
		return nil
	}
	return aws.String(d.StorageClass)
}

// writer uploads parts to S3 in a buffered fashion where the length of each
// part is [writer.driver.ChunkSize], excluding the last part which may be
// smaller than the configured chunk size and never larger. This allows the
// multipart upload to be cleanly resumed in future. This is violated if
// [writer.Close] is called before at least one chunk is written.
type writer struct {
	ctx       context.Context
	driver    *driver
	key       string
	uploadID  string
	parts     []*s3.Part
	size      int64
	buf       *bytes.Buffer
	closed    bool
	committed bool
	cancelled bool
}

func (d *driver) newWriter(ctx context.Context, key, uploadID string, parts []*s3.Part) storagedriver.FileWriter {
	var size int64
	for _, part := range parts {
		size += *part.Size
	}
	return &writer{
		ctx:      ctx,
		driver:   d,
		key:      key,
		uploadID: uploadID,
		parts:    parts,
		size:     size,
		buf:      d.pool.Get().(*bytes.Buffer),
	}
}

type completedParts []*s3.CompletedPart

func (a completedParts) Len() int           { return len(a) }
func (a completedParts) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a completedParts) Less(i, j int) bool { return *a[i].PartNumber < *a[j].PartNumber }

func (w *writer) Write(p []byte) (int, error) {
	if err := w.done(); err != nil {
		return 0, err
	}

	// If the last written part is smaller than minChunkSize, we need to make a
	// new multipart upload :sadface:
	if len(w.parts) > 0 && int(*w.parts[len(w.parts)-1].Size) < minChunkSize {
		completedUploadedParts := make(completedParts, len(w.parts))
		for i, part := range w.parts {
			completedUploadedParts[i] = &s3.CompletedPart{
				ETag:       part.ETag,
				PartNumber: part.PartNumber,
			}
		}

		sort.Sort(completedUploadedParts)

		_, err := w.driver.S3.CompleteMultipartUploadWithContext(w.ctx, &s3.CompleteMultipartUploadInput{
			Bucket:   aws.String(w.driver.Bucket),
			Key:      aws.String(w.key),
			UploadId: aws.String(w.uploadID),
			MultipartUpload: &s3.CompletedMultipartUpload{
				Parts: completedUploadedParts,
			},
		})
		if err != nil {
			if _, aErr := w.driver.S3.AbortMultipartUploadWithContext(w.ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(w.driver.Bucket),
				Key:      aws.String(w.key),
				UploadId: aws.String(w.uploadID),
			}); aErr != nil {
				return 0, errors.Join(err, aErr)
			}
			return 0, err
		}

		resp, err := w.driver.S3.CreateMultipartUploadWithContext(w.ctx, &s3.CreateMultipartUploadInput{
			Bucket:               aws.String(w.driver.Bucket),
			Key:                  aws.String(w.key),
			ContentType:          w.driver.getContentType(),
			ACL:                  w.driver.getACL(),
			ServerSideEncryption: w.driver.getEncryptionMode(),
			StorageClass:         w.driver.getStorageClass(),
		})
		if err != nil {
			return 0, err
		}
		w.uploadID = *resp.UploadId

		// If the entire written file is smaller than minChunkSize, we need to make
		// a new part from scratch :double sad face:
		if w.size < minChunkSize {
			resp, err := w.driver.S3.GetObjectWithContext(w.ctx, &s3.GetObjectInput{
				Bucket: aws.String(w.driver.Bucket),
				Key:    aws.String(w.key),
			})
			if err != nil {
				return 0, err
			}
			defer resp.Body.Close()

			w.reset()

			if _, err := io.Copy(w.buf, resp.Body); err != nil {
				return 0, err
			}
		} else {
			// Otherwise we can use the old file as the new first part
			copyPartResp, err := w.driver.S3.UploadPartCopyWithContext(w.ctx, &s3.UploadPartCopyInput{
				Bucket:     aws.String(w.driver.Bucket),
				CopySource: aws.String(w.driver.Bucket + "/" + w.key),
				Key:        aws.String(w.key),
				PartNumber: aws.Int64(1),
				UploadId:   resp.UploadId,
			})
			if err != nil {
				return 0, err
			}
			w.parts = []*s3.Part{{
				ETag:       copyPartResp.CopyPartResult.ETag,
				PartNumber: aws.Int64(1),
				Size:       aws.Int64(w.size),
			}}
		}
	}

	n, _ := w.buf.Write(p)

	for w.buf.Len() >= w.driver.ChunkSize {
		if err := w.flush(); err != nil {
			return 0, fmt.Errorf("flush: %w", err)
		}
	}
	return n, nil
}

func (w *writer) Size() int64 {
	return w.size
}

// Close flushes any remaining data in the buffer and releases the buffer back
// to the pool.
func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}

	w.closed = true

	defer w.releaseBuffer()

	return w.flush()
}

func (w *writer) reset() {
	w.buf.Reset()
	w.parts = nil
	w.size = 0
}

// releaseBuffer resets the buffer and returns it to the pool.
func (w *writer) releaseBuffer() {
	w.buf.Reset()
	w.driver.pool.Put(w.buf)
}

// Cancel aborts the multipart upload and closes the writer.
func (w *writer) Cancel(ctx context.Context) error {
	if err := w.done(); err != nil {
		return err
	}

	w.cancelled = true
	_, err := w.driver.S3.AbortMultipartUploadWithContext(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.driver.Bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
	})
	return err
}

// Commit flushes any remaining data in the buffer and completes the multipart
// upload.
func (w *writer) Commit(ctx context.Context) error {
	if err := w.done(); err != nil {
		return err
	}

	if err := w.flush(); err != nil {
		return err
	}

	w.committed = true

	completedUploadedParts := make(completedParts, len(w.parts))
	for i, part := range w.parts {
		completedUploadedParts[i] = &s3.CompletedPart{
			ETag:       part.ETag,
			PartNumber: part.PartNumber,
		}
	}

	// This is an edge case when we are trying to upload an empty file as part of
	// the MultiPart upload. We get a PUT with Content-Length: 0 and sad things happen.
	// The result is we are trying to Complete MultipartUpload with an empty list of
	// completedUploadedParts which will always lead to 400 being returned from S3
	// See: https://docs.aws.amazon.com/sdk-for-go/api/service/s3/#CompletedMultipartUpload
	// Solution: we upload the empty i.e. 0 byte part as a single part and then append it
	// to the completedUploadedParts slice used to complete the Multipart upload.
	if len(w.parts) == 0 {
		resp, err := w.driver.S3.UploadPartWithContext(w.ctx, &s3.UploadPartInput{
			Bucket:     aws.String(w.driver.Bucket),
			Key:        aws.String(w.key),
			PartNumber: aws.Int64(1),
			UploadId:   aws.String(w.uploadID),
			Body:       bytes.NewReader(nil),
		})
		if err != nil {
			return err
		}

		completedUploadedParts = append(completedUploadedParts, &s3.CompletedPart{
			ETag:       resp.ETag,
			PartNumber: aws.Int64(1),
		})
	}

	sort.Sort(completedUploadedParts)

	if _, err := w.driver.S3.CompleteMultipartUploadWithContext(w.ctx, &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(w.driver.Bucket),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
		MultipartUpload: &s3.CompletedMultipartUpload{
			Parts: completedUploadedParts,
		},
	}); err != nil {
		if _, aErr := w.driver.S3.AbortMultipartUploadWithContext(w.ctx, &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(w.driver.Bucket),
			Key:      aws.String(w.key),
			UploadId: aws.String(w.uploadID),
		}); aErr != nil {
			return errors.Join(err, aErr)
		}
		return err
	}
	return nil
}

// flush writes at most [w.driver.ChunkSize] of the buffer to S3. flush is only
// called by [writer.Write] if the buffer is full, and always by [writer.Close]
// and [writer.Commit].
func (w *writer) flush() error {
	if w.buf.Len() == 0 {
		return nil
	}

	r := bytes.NewReader(w.buf.Next(w.driver.ChunkSize))

	partSize := r.Len()
	partNumber := aws.Int64(int64(len(w.parts)) + 1)

	resp, err := w.driver.S3.UploadPartWithContext(w.ctx, &s3.UploadPartInput{
		Bucket:     aws.String(w.driver.Bucket),
		Key:        aws.String(w.key),
		PartNumber: partNumber,
		UploadId:   aws.String(w.uploadID),
		Body:       r,
	})
	if err != nil {
		return fmt.Errorf("upload part: %w", err)
	}

	w.parts = append(w.parts, &s3.Part{
		ETag:       resp.ETag,
		PartNumber: partNumber,
		Size:       aws.Int64(int64(partSize)),
	})

	w.size += int64(partSize)

	return nil
}

// done returns an error if the writer is in an invalid state.
func (w *writer) done() error {
	switch {
	case w.closed:
		return fmt.Errorf("already closed")
	case w.committed:
		return fmt.Errorf("already committed")
	case w.cancelled:
		return fmt.Errorf("already cancelled")
	}
	return nil
}

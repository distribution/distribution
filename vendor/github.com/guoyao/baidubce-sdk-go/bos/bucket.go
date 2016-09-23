package bos

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guoyao/baidubce-sdk-go/bce"
	"github.com/guoyao/baidubce-sdk-go/util"
)

const MIN_PART_NUMBER int = 1
const MAX_PART_NUMBER int = 10000

var UserDefinedMetadataPrefix = "x-bce-meta-"

var CannedAccessControlList = map[string]string{
	"Private":         "private",
	"PublicRead":      "public-read",
	"PublicReadWrite": "public-read-write",
}

// Location is a struct for bucket location info.
type Location struct {
	LocationConstraint string
}

type BucketOwner struct {
	Id          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
}

// Bucket is a struct for bucket info.
type Bucket struct {
	Name, Location string
	CreationDate   time.Time
}

// BucketSummary is a struct for bucket summary.
type BucketSummary struct {
	Owner   BucketOwner
	Buckets []Bucket
}

type BucketAcl struct {
	Owner             BucketOwner `json:"owner"`
	AccessControlList []Grant     `json:"accessControlList"`
}

type Grant struct {
	Grantee    []BucketGrantee `json:"grantee"`
	Permission []string        `json:"permission"`
}

type BucketGrantee struct {
	Id string `json:"id"`
}

type ObjectMetadata struct {
	CacheControl       string
	ContentDisposition string
	ContentLength      int64
	ContentMD5         string
	ContentType        string
	Expires            string
	ContentSha256      string

	ContentRange string
	ETag         string
	UserMetadata map[string]string
}

func NewObjectMetadataFromHeader(h http.Header) *ObjectMetadata {
	objectMetadata := &ObjectMetadata{}

	for key, _ := range h {
		key = strings.ToLower(key)

		if key == "cache-control" {
			objectMetadata.CacheControl = h.Get(key)
		} else if key == "content-disposition" {
			objectMetadata.ContentDisposition = h.Get(key)
		} else if key == "content-length" {
			length, err := strconv.ParseInt(h.Get(key), 10, 64)

			if err == nil {
				objectMetadata.ContentLength = length
			}
		} else if key == "content-range" {
			objectMetadata.ContentRange = h.Get(key)
		} else if key == "content-type" {
			objectMetadata.ContentType = h.Get(key)
		} else if key == "expires" {
			objectMetadata.Expires = h.Get(key)
		} else if key == "etag" {
			objectMetadata.ETag = strings.Replace(h.Get(key), "\"", "", -1)
		} else if IsUserDefinedMetadata(key) {
			objectMetadata.UserMetadata[key] = h.Get(key)
		}
	}

	return objectMetadata
}

func (metadata *ObjectMetadata) AddUserMetadata(key, value string) {
	if metadata.UserMetadata == nil {
		metadata.UserMetadata = make(map[string]string)
	}

	metadata.UserMetadata[key] = value
}

func (metadata *ObjectMetadata) mergeToSignOption(option *bce.SignOption) {
	if metadata.CacheControl != "" {
		option.AddHeader("Cache-Control", metadata.CacheControl)
	}

	if metadata.ContentDisposition != "" {
		option.AddHeader("Content-Disposition", metadata.ContentDisposition)
	}

	if metadata.ContentLength != 0 {
		option.AddHeader("Content-Length", strconv.FormatInt(metadata.ContentLength, 10))
	}

	if metadata.ContentMD5 != "" {
		option.AddHeader("Content-MD5", metadata.ContentMD5)
	}

	if metadata.ContentType != "" {
		option.AddHeader("Content-Type", metadata.ContentType)
	}

	if metadata.Expires != "" {
		option.AddHeader("Expires", metadata.Expires)
	}

	if metadata.ContentSha256 != "" {
		option.AddHeader("x-bce-content-sha256", metadata.ContentSha256)
	}

	for key, value := range metadata.UserMetadata {
		option.AddHeader(ToUserDefinedMetadata(key), value)
	}
}

type PutObjectResponse http.Header

func NewPutObjectResponse(h http.Header) PutObjectResponse {
	return PutObjectResponse(h)
}

func (res PutObjectResponse) Get(key string) string {
	return http.Header(res).Get(key)
}

func (res PutObjectResponse) GetETag() string {
	return strings.Replace(res.Get("Etag"), "\"", "", -1)
}

type AppendObjectResponse http.Header

func NewAppendObjectResponse(h http.Header) AppendObjectResponse {
	return AppendObjectResponse(h)
}

func (res AppendObjectResponse) Get(key string) string {
	return http.Header(res).Get(key)
}

func (res AppendObjectResponse) GetETag() string {
	return strings.Replace(res.Get("Etag"), "\"", "", -1)
}

func (res AppendObjectResponse) GetMD5() string {
	return res.Get("Content-MD5")
}

func (res AppendObjectResponse) GetNextAppendOffset() string {
	return res.Get("x-bce-next-append-offset")
}

type ObjectSummary struct {
	Key          string
	LastModified string
	ETag         string
	Size         int64
	Owner        BucketOwner
}

type ListObjectsRequest struct {
	BucketName, Delimiter, Marker, Prefix string
	MaxKeys                               int
}

type ListObjectsResponse struct {
	Name           string
	Prefix         string
	Delimiter      string
	Marker         string
	NextMarker     string
	MaxKeys        uint
	IsTruncated    bool
	Contents       []ObjectSummary
	CommonPrefixes []map[string]string
}

func (listObjectsResponse *ListObjectsResponse) GetCommonPrefixes() []string {
	prefixes := make([]string, 0, len(listObjectsResponse.CommonPrefixes))

	for _, commonPrefix := range listObjectsResponse.CommonPrefixes {
		prefixes = append(prefixes, commonPrefix["prefix"])
	}

	return prefixes
}

type CopyObjectResponse struct {
	ETag         string
	LastModified time.Time
}

type CopyObjectRequest struct {
	SrcBucketName         string          `json:"-"`
	SrcKey                string          `json:"-"`
	DestBucketName        string          `json:"-"`
	DestKey               string          `json:"-"`
	ObjectMetadata        *ObjectMetadata `json:"-"`
	SourceMatch           string          `json:"x-bce-copy-source-if-match,omitempty"`
	SourceNoneMatch       string          `json:"x-bce-copy-source-if-none-match,omitempty"`
	SourceModifiedSince   string          `json:"x-bce-copy-source-if-modified-since,omitempty"`
	SourceUnmodifiedSince string          `json:"x-bce-copy-source-if-unmodified-since,omitempty"`
}

func (copyObjectRequest CopyObjectRequest) mergeToSignOption(option *bce.SignOption) {
	m, err := util.ToMap(copyObjectRequest)

	if err != nil {
		return
	}

	headerMap := make(map[string]string)

	for key, value := range m {
		if str, ok := value.(string); ok {
			headerMap[key] = str
		}
	}

	option.AddHeaders(headerMap)

	if copyObjectRequest.ObjectMetadata != nil {
		option.AddHeader("x-bce-metadata-directive", "replace")
		copyObjectRequest.ObjectMetadata.mergeToSignOption(option)
	} else {
		option.AddHeader("x-bce-metadata-directive", "copy")
	}
}

type Object struct {
	ObjectMetadata *ObjectMetadata
	ObjectContent  io.ReadCloser
}

type GetObjectRequest struct {
	BucketName string
	ObjectKey  string
	Range      string
}

func (getObjectRequest *GetObjectRequest) MergeToSignOption(option *bce.SignOption) {
	if getObjectRequest.Range != "" {
		option.AddHeader("Range", "bytes="+getObjectRequest.Range)
	}
}

func (getObjectRequest *GetObjectRequest) SetRange(start uint, end uint) {
	getObjectRequest.Range = fmt.Sprintf("%v-%v", start, end)
}

type DeleteMultipleObjectsResponse struct {
	Errors []DeleteMultipleObjectsError
}

type DeleteMultipleObjectsError struct {
	Key, Code, Message string
}

func (deleteMultipleObjectsError *DeleteMultipleObjectsError) Error() string {
	if deleteMultipleObjectsError.Message != "" {
		return deleteMultipleObjectsError.Message
	}

	return deleteMultipleObjectsError.Code
}

type InitiateMultipartUploadRequest struct {
	BucketName, ObjectKey string
	ObjectMetadata        *ObjectMetadata
}

type InitiateMultipartUploadResponse struct {
	Bucket, Key, UploadId string
}

type UploadPartRequest struct {
	BucketName, ObjectKey, UploadId string
	PartSize                        int64
	PartNumber                      int
	PartData                        io.Reader
}

type UploadPartResponse http.Header

func NewUploadPartResponse(h http.Header) UploadPartResponse {
	return UploadPartResponse(h)
}

func (res UploadPartResponse) Get(key string) string {
	return http.Header(res).Get(key)
}

func (res UploadPartResponse) GetETag() string {
	return strings.Replace(res.Get("Etag"), "\"", "", -1)
}

type PartSummarySlice []PartSummary

func (partSummarySlice PartSummarySlice) Len() int {
	return len(partSummarySlice)
}

func (partSummarySlice PartSummarySlice) Swap(i, j int) {
	partSummarySlice[i], partSummarySlice[j] = partSummarySlice[j], partSummarySlice[i]
}

func (partSummarySlice PartSummarySlice) Less(i, j int) bool {
	return partSummarySlice[i].PartNumber < partSummarySlice[j].PartNumber
}

type CompleteMultipartUploadRequest struct {
	BucketName, ObjectKey, UploadId string
	Parts                           []PartSummary `json:"parts"`
}

func (completeMultipartUploadRequest *CompleteMultipartUploadRequest) sort() {
	if len(completeMultipartUploadRequest.Parts) > 1 {
		sort.Sort(PartSummarySlice(completeMultipartUploadRequest.Parts))
	}
}

type CompleteMultipartUploadResponse struct {
	Location, Bucket, Key, ETag string
}

type AbortMultipartUploadRequest struct {
	BucketName, ObjectKey, UploadId string
}

type ListMultipartUploadsRequest struct {
	BucketName, Delimiter, KeyMarker, Prefix string
	MaxUploads                               int
}

type MultipartUploadSummary struct {
	Key, UploadId string
	Initiated     time.Time
	NextKeyMarker string
	Owner         BucketOwner
}

type ListMultipartUploadsResponse struct {
	Bucket         string
	Prefix         string
	Delimiter      string
	KeyMarker      string
	NextKeyMarker  string
	MaxUploads     int
	IsTruncated    bool
	Uploads        []MultipartUploadSummary
	CommonPrefixes []map[string]string
}

func (listMultipartUploadsResponse *ListMultipartUploadsResponse) GetCommonPrefixes() []string {
	prefixes := make([]string, 0, len(listMultipartUploadsResponse.CommonPrefixes))

	for _, commonPrefix := range listMultipartUploadsResponse.CommonPrefixes {
		prefixes = append(prefixes, commonPrefix["prefix"])
	}

	return prefixes
}

type ListPartsRequest struct {
	BucketName, ObjectKey, UploadId, PartNumberMarker string
	MaxParts                                          int
}

type PartSummary struct {
	PartNumber   int    `json:"partNumber"`
	ETag         string `json:"eTag"`
	LastModified time.Time
	Size         int64
}

type ListPartsResponse struct {
	Bucket               string
	Key                  string
	UploadId             string
	Initiated            time.Time
	PartNumberMarker     int
	NextPartNumberMarker int
	MaxParts             int
	IsTruncated          bool
	Owner                BucketOwner
	Parts                []PartSummary
}

type BucketCors struct {
	CorsConfiguration []BucketCorsItem `json:"corsConfiguration"`
}

type BucketCorsItem struct {
	AllowedOrigins       []string `json:"allowedOrigins"`
	AllowedMethods       []string `json:"allowedMethods"`
	AllowedHeaders       []string `json:"allowedHeaders"`
	AllowedExposeHeaders []string `json:"allowedExposeHeaders"`
	MaxAgeSeconds        int      `json:"maxAgeSeconds"`
}

type BucketLogging struct {
	Status       string `json:"status"`
	TargetBucket string `json:"targetBucket"`
	TargetPrefix string `json:"targetPrefix"`
}

func IsUserDefinedMetadata(metadata string) bool {
	return strings.Index(metadata, UserDefinedMetadataPrefix) == 0
}

func ToUserDefinedMetadata(metadata string) string {
	if IsUserDefinedMetadata(metadata) {
		return metadata
	}

	return UserDefinedMetadataPrefix + metadata
}

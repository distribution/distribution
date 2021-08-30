/**
 * Copyright (c) 2015 Guoyao Wu, All Rights Reserved
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * @file core.go
 * @author guoyao
 */

// Package bos defined a set of core data structure and functions for Baidu Cloud BOS API.
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

// MIN_PART_NUMBER is the min part number for multipart upload.
const MIN_PART_NUMBER int = 1

// MAX_PART_NUMBER is the max part number for multipart upload.
const MAX_PART_NUMBER int = 10000

// STORAGE_CLASS is the storage type of BOS object
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutObject.E6.8E.A5.E5.8F.A3
const STORAGE_CLASS_STANDARD = "STANDARD"
const STORAGE_CLASS_STANDARD_IA = "STANDARD_IA"
const STORAGE_CLASS_COLD = "COLD"

// UserDefinedMetadataPrefix is the prefix of custom metadata.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutObject.E6.8E.A5.E5.8F.A3
const UserDefinedMetadataPrefix = "x-bce-meta-"

// CannedAccessControlList contains all authority levels of BOS.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#.4F.FA.21.55.58.27.F8.31.85.2D.01.55.89.10.A7.16
var CannedAccessControlList = map[string]string{
	"Private":         "private",
	"PublicRead":      "public-read",
	"PublicReadWrite": "public-read-write",
}

// Location defined a struct for bucket location info.
type Location struct {
	LocationConstraint string
}

// BucketOwner defined a struct for bucket owner info.
type BucketOwner struct {
	Id          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
}

// Bucket defined a struct for bucket info.
type Bucket struct {
	Name, Location string
	CreationDate   time.Time
}

// BucketSummary defined a struct for bucket summary.
type BucketSummary struct {
	Owner   BucketOwner
	Buckets []Bucket
}

// BucketAcl defined a struct for authority info.
type BucketAcl struct {
	Owner             BucketOwner `json:"owner"`
	AccessControlList []Grant     `json:"accessControlList"`
}

// BucketAcl defined a struct for grantee and permission info.
type Grant struct {
	Grantee    []BucketGrantee `json:"grantee"`
	Permission []string        `json:"permission"`
}

// BucketGrantee defined a struct for grantee info.
type BucketGrantee struct {
	Id string `json:"id"`
}

// ObjectMetadata defined a struct for all metadata info of BOS Object.
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
	StorageClass string
}

// NewObjectMetadataFromHeader generates a bos.ObjectMetadata instance from a http.Header instance.
func NewObjectMetadataFromHeader(h http.Header) *ObjectMetadata {
	objectMetadata := &ObjectMetadata{}

	for key, _ := range h {
		if len(h[key]) > 0 {
			lowerKey := strings.ToLower(key)
			value := h[key][0]

			if lowerKey == "cache-control" {
				objectMetadata.CacheControl = value
			} else if lowerKey == "content-disposition" {
				objectMetadata.ContentDisposition = value
			} else if lowerKey == "content-length" {
				length, err := strconv.ParseInt(value, 10, 64)

				if err == nil {
					objectMetadata.ContentLength = length
				}
			} else if lowerKey == "content-range" {
				objectMetadata.ContentRange = value
			} else if lowerKey == "content-type" {
				objectMetadata.ContentType = value
			} else if lowerKey == "expires" {
				objectMetadata.Expires = value
			} else if lowerKey == "etag" {
				objectMetadata.ETag = strings.Replace(value, "\"", "", -1)
			} else if IsUserDefinedMetadata(lowerKey) {
				if objectMetadata.UserMetadata == nil {
					objectMetadata.UserMetadata = make(map[string]string, 0)
				}
				objectMetadata.UserMetadata[key] = h[key][0]
			} else if lowerKey == "x-bce-storage-class" {
				objectMetadata.StorageClass = value
			}
		}
	}

	return objectMetadata
}

// AddUserMetadata adds a custom metadata to bos.ObjectMetadata.
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

	if metadata.StorageClass == STORAGE_CLASS_STANDARD ||
		metadata.StorageClass == STORAGE_CLASS_STANDARD_IA ||
		metadata.StorageClass == STORAGE_CLASS_COLD {
		option.AddHeader("x-bce-storage-class", metadata.StorageClass)
	}

	for key, value := range metadata.UserMetadata {
		option.AddHeader(ToUserDefinedMetadata(key), value)
	}
}

type PutObjectResponse http.Header

func NewPutObjectResponse(h http.Header) PutObjectResponse {
	return PutObjectResponse(h)
}

// Get gets a header value by key from http header.
func (res PutObjectResponse) Get(key string) string {
	return http.Header(res).Get(key)
}

// GetETag gets Etag value from http header.
func (res PutObjectResponse) GetETag() string {
	return strings.Replace(res.Get("Etag"), "\"", "", -1)
}

type AppendObjectResponse http.Header

func NewAppendObjectResponse(h http.Header) AppendObjectResponse {
	return AppendObjectResponse(h)
}

// Get gets a header value by key from http header.
func (res AppendObjectResponse) Get(key string) string {
	return http.Header(res).Get(key)
}

// GetETag gets value of Etag field from http header.
func (res AppendObjectResponse) GetETag() string {
	return strings.Replace(res.Get("Etag"), "\"", "", -1)
}

// GetMD5 gets value of Content-MD5 field from http header.
func (res AppendObjectResponse) GetMD5() string {
	return res.Get("Content-MD5")
}

// GetNextAppendOffset gets value of x-bce-next-append-offset field from http header.
func (res AppendObjectResponse) GetNextAppendOffset() string {
	return res.Get("x-bce-next-append-offset")
}

// ObjectMetadata defined a struct for BOS Object summary info.
type ObjectSummary struct {
	Key          string
	LastModified string
	ETag         string
	Size         int64
	Owner        BucketOwner
}

// ListObjectsRequest contains all options for bos.ListObjectsFromRequest method.
type ListObjectsRequest struct {
	BucketName, Delimiter, Marker, Prefix string
	MaxKeys                               int
}

// ListObjectsResponse defined a struct for bos.ListObjectsFromRequest method's response.
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

// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#GetBucket.2FListObjects.E6.8E.A5.E5.8F.A3
func (listObjectsResponse *ListObjectsResponse) GetCommonPrefixes() []string {
	prefixes := make([]string, 0, len(listObjectsResponse.CommonPrefixes))

	for _, commonPrefix := range listObjectsResponse.CommonPrefixes {
		prefixes = append(prefixes, commonPrefix["prefix"])
	}

	return prefixes
}

// CopyObjectResponse defined a struct for bos.CopyObject method's response.
type CopyObjectResponse struct {
	ETag         string
	LastModified time.Time
}

// CopyObjectRequest contains all options for bos.ListObjectsFromRequest method.
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

// Object defined a struct for BOS Object.
type Object struct {
	ObjectMetadata *ObjectMetadata
	ObjectContent  io.ReadCloser
}

// GetObjectRequest contains all options for bos.GetObjectFromRequest method.
type GetObjectRequest struct {
	BucketName string
	ObjectKey  string
	Range      string
}

// MergeToSignOption merges bos.GetObjectRequest fields to bce.SignOption.
func (getObjectRequest *GetObjectRequest) MergeToSignOption(option *bce.SignOption) {
	if getObjectRequest.Range != "" {
		option.AddHeader("Range", "bytes="+getObjectRequest.Range)
	}
}

// SetRange sets the range field of bos.GetObjectRequest.
func (getObjectRequest *GetObjectRequest) SetRange(start uint, end uint) {
	getObjectRequest.Range = fmt.Sprintf("%v-%v", start, end)
}

// DeleteMultipleObjectsError defined a struct for bos.DeleteMultipleObjects method's response.
type DeleteMultipleObjectsResponse struct {
	Errors []DeleteMultipleObjectsError
}

// DeleteMultipleObjectsError defined a struct for error message of bos.DeleteMultipleObjects method.
type DeleteMultipleObjectsError struct {
	Key, Code, Message string
}

// Error returns the formatted error message of bos.DeleteMultipleObjects method.
func (deleteMultipleObjectsError *DeleteMultipleObjectsError) Error() string {
	if deleteMultipleObjectsError.Message != "" {
		return deleteMultipleObjectsError.Message
	}

	return deleteMultipleObjectsError.Code
}

// InitiateMultipartUploadRequest contains all options for bos.InitiateMultipartUpload method.
type InitiateMultipartUploadRequest struct {
	BucketName, ObjectKey string
	ObjectMetadata        *ObjectMetadata
}

// InitiateMultipartUploadResponse defined a atruct for bos.InitiateMultipartUpload method's response.
type InitiateMultipartUploadResponse struct {
	Bucket, Key, UploadId string
}

// UploadPartRequest contains all options for bos.UploadPart method.
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

// Get gets a header value by key from http header.
func (res UploadPartResponse) Get(key string) string {
	return http.Header(res).Get(key)
}

// GetETag gets value of Etag field from http header.
func (res UploadPartResponse) GetETag() string {
	return strings.Replace(res.Get("Etag"), "\"", "", -1)
}

// PartSummarySlice defined a slice for bos.PartSummary.
//
// PartSummarySlice implements the interface of sort.Interface,
// so we can use sort.Sort method to sort it.
type PartSummarySlice []PartSummary

// Len gets the length of bos.PartSummarySlice instance.
func (partSummarySlice PartSummarySlice) Len() int {
	return len(partSummarySlice)
}

// Swap swap two elements in bos.PartSummarySlice instance.
func (partSummarySlice PartSummarySlice) Swap(i, j int) {
	partSummarySlice[i], partSummarySlice[j] = partSummarySlice[j], partSummarySlice[i]
}

// Less compares two elements in bos.PartSummarySlice instance.
func (partSummarySlice PartSummarySlice) Less(i, j int) bool {
	return partSummarySlice[i].PartNumber < partSummarySlice[j].PartNumber
}

// CompleteMultipartUploadRequest contains all options for bos.CompleteMultipartUpload method.
type CompleteMultipartUploadRequest struct {
	BucketName, ObjectKey, UploadId string
	Parts                           []PartSummary `json:"parts"`
}

func (completeMultipartUploadRequest *CompleteMultipartUploadRequest) sort() {
	if len(completeMultipartUploadRequest.Parts) > 1 {
		sort.Sort(PartSummarySlice(completeMultipartUploadRequest.Parts))
	}
}

// CompleteMultipartUploadResponse defined a struct for bos.CompleteMultipartUpload method's response.
type CompleteMultipartUploadResponse struct {
	Location, Bucket, Key, ETag string
}

// AbortMultipartUploadRequest contains all options for bos.AbortMultipartUpload method.
type AbortMultipartUploadRequest struct {
	BucketName, ObjectKey, UploadId string
}

// ListMultipartUploadsRequest contains all options for bos.ListMultipartUploads method.
type ListMultipartUploadsRequest struct {
	BucketName, Delimiter, KeyMarker, Prefix string
	MaxUploads                               int
}

// MultipartUploadSummary defined a struct for summary of each multipart upload item.
type MultipartUploadSummary struct {
	Key, UploadId string
	Initiated     time.Time
	NextKeyMarker string
	Owner         BucketOwner
}

// ListMultipartUploadsResponse defined a struct for bos.ListMultipartUploads method's response.
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

// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#ListMultipartUploads.E6.8E.A5.E5.8F.A3
func (listMultipartUploadsResponse *ListMultipartUploadsResponse) GetCommonPrefixes() []string {
	prefixes := make([]string, 0, len(listMultipartUploadsResponse.CommonPrefixes))

	for _, commonPrefix := range listMultipartUploadsResponse.CommonPrefixes {
		prefixes = append(prefixes, commonPrefix["prefix"])
	}

	return prefixes
}

// ListPartsRequest contains all options for bos.ListParts method.
type ListPartsRequest struct {
	BucketName, ObjectKey, UploadId, PartNumberMarker string
	MaxParts                                          int
}

// PartSummary defind a struct for each part's summary of multipart upload.
type PartSummary struct {
	PartNumber   int    `json:"partNumber"`
	ETag         string `json:"eTag"`
	LastModified time.Time
	Size         int64
}

// ListPartsResponse defined a struct for bos.ListParts method's response.
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

// BucketCors defined a struct for all CORS configuration info.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutBucketCors.E6.8E.A5.E5.8F.A3
type BucketCors struct {
	CorsConfiguration []BucketCorsItem `json:"corsConfiguration"`
}

// BucketCorsItem defined a struct for one item of CORS configuration.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutBucketCors.E6.8E.A5.E5.8F.A3
type BucketCorsItem struct {
	AllowedOrigins       []string `json:"allowedOrigins"`
	AllowedMethods       []string `json:"allowedMethods"`
	AllowedHeaders       []string `json:"allowedHeaders"`
	AllowedExposeHeaders []string `json:"allowedExposeHeaders"`
	MaxAgeSeconds        int      `json:"maxAgeSeconds"`
}

// BucketLogging contains all options for bos.GetBucketLogging method.
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#GetBucketLogging
type BucketLogging struct {
	Status       string `json:"status"`
	TargetBucket string `json:"targetBucket"`
	TargetPrefix string `json:"targetPrefix"`
}

type BucketLifecycle struct {
	Rule []BucketLifecycleItem `json:"rule"`
}

// BucketLifecycleRule defined a struct for one item of Lifecycle configuration
//
// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutBucketlifecycle
type BucketLifecycleItem struct {
	Id        string                       `json:"id,omitempty"`
	Status    string                       `json:"status"`
	Resource  []string                     `json:"resource"`
	Condition BucketLifecycleItemCondition `json:"condition"`
	Action    BucketLifecycleItemAction    `json:"action"`
}

// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutBucketlifecycle
type BucketLifecycleItemCondition struct {
	Time BucketLifecycleItemConditionTime `json:"time"`
}

// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutBucketlifecycle
type BucketLifecycleItemConditionTime struct {
	DateGreaterThan string `json:"dateGreaterThan"`
}

// For details, please refer https://cloud.baidu.com/doc/BOS/API.html#PutBucketlifecycle
type BucketLifecycleItemAction struct {
	Name         string `json:"name"`
	StorageClass string `json:"storageClass,omitempty"`
}

// IsUserDefinedMetadata checks the specified metadata if it is custom metadata.
func IsUserDefinedMetadata(metadata string) bool {
	return strings.Index(metadata, UserDefinedMetadataPrefix) == 0
}

// ToUserDefinedMetadata generates a custom metadata value.
func ToUserDefinedMetadata(metadata string) string {
	if IsUserDefinedMetadata(metadata) {
		return metadata
	}

	return UserDefinedMetadataPrefix + metadata
}

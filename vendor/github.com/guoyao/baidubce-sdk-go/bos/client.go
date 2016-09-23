package bos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/guoyao/baidubce-sdk-go/bce"
	"github.com/guoyao/baidubce-sdk-go/util"
)

// Endpoints of baidubce
var Endpoint = map[string]string{
	"bj": "bj.bcebos.com",
	"gz": "gz.bcebos.com",
	"hk": "hk.bcebos.com",
}

type Config struct {
	*bce.Config
}

func NewConfig(config *bce.Config) *Config {
	return &Config{config}
}

// Client is the client for bos.
type Client struct {
	*bce.Client
}

// DefaultClient provided a default `bos.Client` instance.
//var DefaultClient = NewClient(bce.DefaultConfig)

// NewClient returns an instance of type `bos.Client`.
func NewClient(config *Config) *Client {
	bceClient := bce.NewClient(config.Config)
	return &Client{bceClient}
}

func checkBucketName(bucketName string) {
	if bucketName == "" {
		panic("bucket name should not be empty.")
	}

	if strings.Index(bucketName, "/") == 0 {
		panic("bucket name should not be start with '/'")
	}
}

func checkObjectKey(objectKey string) {
	if objectKey == "" {
		panic("object key should not be empty.")
	}

	if strings.Index(objectKey, "/") == 0 {
		panic("object key should not be start with '/'")
	}
}

// GetBucketName returns the actual name of bucket.
func (c *Client) GetBucketName(bucketName string) string {
	return bucketName
}

func (c *Client) GetURL(bucketName, objectKey string, params map[string]string) string {
	host := c.Endpoint

	if host == "" {
		host = Endpoint[c.GetRegion()]
	}

	if bucketName != "" {
		host = bucketName + "." + host
	}

	uriPath := objectKey

	return c.Client.GetURL(host, uriPath, params)
}

// GetBucketLocation returns the location of a bucket.
func (c *Client) GetBucketLocation(bucketName string, option *bce.SignOption) (*Location, error) {
	bucketName = c.GetBucketName(bucketName)
	params := map[string]string{"location": ""}

	req, err := bce.NewRequest("GET", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var location *Location
	err = json.Unmarshal(bodyContent, &location)

	if err != nil {
		return nil, err
	}

	return location, nil
}

// ListBuckets is for getting a collection of bucket.
func (c *Client) ListBuckets(option *bce.SignOption) (*BucketSummary, error) {
	req, err := bce.NewRequest("GET", c.GetURL("", "", nil), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var bucketSummary *BucketSummary
	err = json.Unmarshal(bodyContent, &bucketSummary)

	if err != nil {
		return nil, err
	}

	return bucketSummary, nil
}

// CreateBucket is for creating a bucket.
func (c *Client) CreateBucket(bucketName string, option *bce.SignOption) error {
	req, err := bce.NewRequest("PUT", c.GetURL(bucketName, "", nil), nil)

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) DoesBucketExist(bucketName string, option *bce.SignOption) (bool, error) {
	req, err := bce.NewRequest("HEAD", c.GetURL(bucketName, "", nil), nil)

	if err != nil {
		return false, err
	}

	resp, err := c.SendRequest(req, option)

	if resp != nil {
		switch {
		case resp.StatusCode < http.StatusBadRequest || resp.StatusCode == http.StatusForbidden:
			return true, nil
		case resp.StatusCode == http.StatusNotFound:
			return false, nil
		}
	}

	return false, err
}

func (c *Client) DeleteBucket(bucketName string, option *bce.SignOption) error {
	req, err := bce.NewRequest("DELETE", c.GetURL(bucketName, "", nil), nil)

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) SetBucketPrivate(bucketName string, option *bce.SignOption) error {
	return c.setBucketAclFromString(bucketName, CannedAccessControlList["Private"], option)
}

func (c *Client) SetBucketPublicRead(bucketName string, option *bce.SignOption) error {
	return c.setBucketAclFromString(bucketName, CannedAccessControlList["PublicRead"], option)
}

func (c *Client) SetBucketPublicReadWrite(bucketName string, option *bce.SignOption) error {
	return c.setBucketAclFromString(bucketName, CannedAccessControlList["PublicReadWrite"], option)
}

func (c *Client) GetBucketAcl(bucketName string, option *bce.SignOption) (*BucketAcl, error) {
	params := map[string]string{"acl": ""}
	req, err := bce.NewRequest("GET", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var bucketAcl *BucketAcl
	err = json.Unmarshal(bodyContent, &bucketAcl)

	if err != nil {
		return nil, err
	}

	return bucketAcl, nil
}

func (c *Client) SetBucketAcl(bucketName string, bucketAcl BucketAcl, option *bce.SignOption) error {
	byteArray, err := util.ToJson(bucketAcl, "accessControlList")

	if err != nil {
		return err
	}

	params := map[string]string{"acl": ""}
	req, err := bce.NewRequest("PUT", c.GetURL(bucketName, "", params), bytes.NewReader(byteArray))

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) PutObject(bucketName, objectKey string, data interface{},
	metadata *ObjectMetadata, option *bce.SignOption) (PutObjectResponse, error) {

	checkObjectKey(objectKey)

	var reader io.Reader

	if str, ok := data.(string); ok {
		reader = strings.NewReader(str)
	} else if byteArray, ok := data.([]byte); ok {
		reader = bytes.NewReader(byteArray)
	} else if r, ok := data.(io.Reader); ok {
		reader = r
	} else {
		panic("data type should be string or []byte or io.Reader.")
	}

	req, err := bce.NewRequest("PUT", c.GetURL(bucketName, objectKey, nil), reader)

	if err != nil {
		return nil, err
	}

	option = bce.CheckSignOption(option)
	option.AddHeader("Content-Type", util.GuessMimeType(objectKey))

	if c.Checksum {
		option.AddHeader("x-bce-content-sha256", util.GetSha256(data))
	}

	if metadata != nil {
		metadata.mergeToSignOption(option)
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	putObjectResponse := NewPutObjectResponse(resp.Header)

	return putObjectResponse, nil
}

func (c *Client) DeleteObject(bucketName, objectKey string, option *bce.SignOption) error {
	checkObjectKey(objectKey)

	req, err := bce.NewRequest("DELETE", c.GetURL(bucketName, objectKey, nil), nil)

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) DeleteMultipleObjects(bucketName string, objectKeys []string,
	option *bce.SignOption) (*DeleteMultipleObjectsResponse, error) {

	checkBucketName(bucketName)

	keys := make([]string, 0, len(objectKeys))

	for _, key := range objectKeys {
		if key != "" {
			keys = append(keys, key)
		}
	}

	objectKeys = keys
	length := len(objectKeys)

	if length == 0 {
		return nil, nil
	}

	objectMap := make(map[string][]map[string]string, 1)
	objects := make([]map[string]string, length, length)

	for index, value := range objectKeys {
		objects[index] = map[string]string{"key": value}
	}

	objectMap["objects"] = objects
	byteArray, err := util.ToJson(objectMap)

	if err != nil {
		return nil, err
	}

	params := map[string]string{"delete": ""}
	body := bytes.NewReader(byteArray)

	req, err := bce.NewRequest("POST", c.GetURL(bucketName, "", params), body)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	if len(bodyContent) > 0 {
		var deleteMultipleObjectsResponse *DeleteMultipleObjectsResponse
		err := json.Unmarshal(bodyContent, &deleteMultipleObjectsResponse)

		if err != nil {
			return nil, err
		}

		return deleteMultipleObjectsResponse, nil
	}

	return nil, nil
}

func (c *Client) ListObjects(bucketName string, option *bce.SignOption) (*ListObjectsResponse, error) {
	return c.ListObjectsFromRequest(ListObjectsRequest{BucketName: bucketName}, option)
}

func (c *Client) ListObjectsFromRequest(listObjectsRequest ListObjectsRequest,
	option *bce.SignOption) (*ListObjectsResponse, error) {

	bucketName := listObjectsRequest.BucketName
	params := make(map[string]string)

	if listObjectsRequest.Delimiter != "" {
		params["delimiter"] = listObjectsRequest.Delimiter
	}

	if listObjectsRequest.Marker != "" {
		params["marker"] = listObjectsRequest.Marker
	}

	if listObjectsRequest.Prefix != "" {
		params["prefix"] = listObjectsRequest.Prefix
	}

	if listObjectsRequest.MaxKeys > 0 {
		params["maxKeys"] = strconv.Itoa(listObjectsRequest.MaxKeys)
	}

	req, err := bce.NewRequest("GET", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var listObjectsResponse *ListObjectsResponse
	err = json.Unmarshal(bodyContent, &listObjectsResponse)

	if err != nil {
		return nil, err
	}

	return listObjectsResponse, nil
}

func (c *Client) CopyObject(srcBucketName, srcKey, destBucketName, destKey string,
	option *bce.SignOption) (*CopyObjectResponse, error) {

	return c.CopyObjectFromRequest(CopyObjectRequest{
		SrcBucketName:  srcBucketName,
		SrcKey:         srcKey,
		DestBucketName: destBucketName,
		DestKey:        destKey,
	}, option)
}

func (c *Client) CopyObjectFromRequest(copyObjectRequest CopyObjectRequest,
	option *bce.SignOption) (*CopyObjectResponse, error) {

	checkBucketName(copyObjectRequest.SrcBucketName)
	checkBucketName(copyObjectRequest.DestBucketName)
	checkObjectKey(copyObjectRequest.SrcKey)
	checkObjectKey(copyObjectRequest.DestKey)

	req, err := bce.NewRequest("PUT", c.GetURL(copyObjectRequest.DestBucketName, copyObjectRequest.DestKey, nil), nil)

	if err != nil {
		return nil, err
	}

	option = bce.CheckSignOption(option)

	source := util.URIEncodeExceptSlash(fmt.Sprintf("/%s/%s", copyObjectRequest.SrcBucketName,
		copyObjectRequest.SrcKey))

	option.AddHeader("x-bce-copy-source", source)
	copyObjectRequest.mergeToSignOption(option)

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var copyObjectResponse *CopyObjectResponse
	err = json.Unmarshal(bodyContent, &copyObjectResponse)

	if err != nil {
		return nil, err
	}

	return copyObjectResponse, nil
}

func (c *Client) GetObject(bucketName, objectKey string, option *bce.SignOption) (*Object, error) {
	return c.GetObjectFromRequest(GetObjectRequest{
		BucketName: bucketName,
		ObjectKey:  objectKey,
	}, option)
}

func (c *Client) GetObjectFromRequest(getObjectRequest GetObjectRequest,
	option *bce.SignOption) (*Object, error) {

	checkBucketName(getObjectRequest.BucketName)
	checkObjectKey(getObjectRequest.ObjectKey)

	req, err := bce.NewRequest("GET", c.GetURL(getObjectRequest.BucketName, getObjectRequest.ObjectKey, nil), nil)

	if err != nil {
		return nil, err
	}

	option = bce.CheckSignOption(option)
	getObjectRequest.MergeToSignOption(option)

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	object := &Object{
		ObjectMetadata: NewObjectMetadataFromHeader(resp.Header),
		ObjectContent:  resp.Body,
	}

	return object, nil
}

func (c *Client) GetObjectToFile(getObjectRequest *GetObjectRequest, file *os.File,
	option *bce.SignOption) (*ObjectMetadata, error) {

	defer func() {
		if file != nil {
			file.Close()
		}
	}()

	checkBucketName(getObjectRequest.BucketName)
	checkObjectKey(getObjectRequest.ObjectKey)

	req, err := bce.NewRequest("GET", c.GetURL(getObjectRequest.BucketName, getObjectRequest.ObjectKey, nil), nil)

	if err != nil {
		return nil, err
	}

	option = bce.CheckSignOption(option)
	getObjectRequest.MergeToSignOption(option)

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	objectMetadata := NewObjectMetadataFromHeader(resp.Header)

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return objectMetadata, err
	}

	_, err = file.Write(bodyContent)

	if err != nil {
		return objectMetadata, err
	}

	return objectMetadata, nil
}

func (c *Client) GetObjectMetadata(bucketName, objectKey string, option *bce.SignOption) (*ObjectMetadata, error) {
	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	req, err := bce.NewRequest("HEAD", c.GetURL(bucketName, objectKey, nil), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	objectMetadata := NewObjectMetadataFromHeader(resp.Header)

	return objectMetadata, nil
}

func (c *Client) GeneratePresignedUrl(bucketName, objectKey string, option *bce.SignOption) (string, error) {
	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	req, err := bce.NewRequest("GET", c.GetURL(bucketName, objectKey, nil), nil)

	if err != nil {
		return "", err
	}

	option = bce.CheckSignOption(option)
	option.HeadersToSign = []string{"host"}

	authorization := bce.GenerateAuthorization(*c.Credentials, *req, option)
	url := fmt.Sprintf("%s?authorization=%s", req.URL.String(), util.URLEncode(authorization))

	return url, nil
}

func (c *Client) AppendObject(bucketName, objectKey string, offset int, data interface{},
	metadata *ObjectMetadata, option *bce.SignOption) (AppendObjectResponse, error) {

	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	var reader io.Reader

	if str, ok := data.(string); ok {
		reader = strings.NewReader(str)
	} else if byteArray, ok := data.([]byte); ok {
		reader = bytes.NewReader(byteArray)
	} else if r, ok := data.(io.Reader); ok {
		byteArray, err := ioutil.ReadAll(r)

		if err != nil {
			return nil, err
		}

		reader = bytes.NewReader(byteArray)
	} else {
		panic("data type should be string or []byte or io.Reader.")
	}

	params := map[string]string{"append": ""}

	if offset > 0 {
		params["offset"] = strconv.Itoa(offset)
	}

	req, err := bce.NewRequest("POST", c.GetURL(bucketName, objectKey, params), reader)

	if err != nil {
		return nil, err
	}

	option = bce.CheckSignOption(option)
	option.AddHeader("Content-Type", util.GuessMimeType(objectKey))

	if c.Checksum {
		option.AddHeader("x-bce-content-sha256", util.GetSha256(data))
	}

	if metadata != nil {
		metadata.mergeToSignOption(option)
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	appendObjectResponse := NewAppendObjectResponse(resp.Header)

	return appendObjectResponse, nil
}

func (c *Client) InitiateMultipartUpload(initiateMultipartUploadRequest InitiateMultipartUploadRequest,
	option *bce.SignOption) (*InitiateMultipartUploadResponse, error) {

	bucketName := initiateMultipartUploadRequest.BucketName
	objectKey := initiateMultipartUploadRequest.ObjectKey

	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	params := map[string]string{"uploads": ""}

	req, err := bce.NewRequest("POST", c.GetURL(bucketName, objectKey, params), nil)

	if err != nil {
		return nil, err
	}

	option = bce.CheckSignOption(option)
	option.AddHeader("Content-Type", util.GuessMimeType(objectKey))

	if initiateMultipartUploadRequest.ObjectMetadata != nil {
		initiateMultipartUploadRequest.ObjectMetadata.mergeToSignOption(option)
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var initiateMultipartUploadResponse *InitiateMultipartUploadResponse
	err = json.Unmarshal(bodyContent, &initiateMultipartUploadResponse)

	if err != nil {
		return nil, err
	}

	return initiateMultipartUploadResponse, nil
}

func (c *Client) UploadPart(uploadPartRequest UploadPartRequest,
	option *bce.SignOption) (UploadPartResponse, error) {

	bucketName := uploadPartRequest.BucketName
	objectKey := uploadPartRequest.ObjectKey
	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	if uploadPartRequest.PartNumber < MIN_PART_NUMBER || uploadPartRequest.PartNumber > MAX_PART_NUMBER {
		panic(fmt.Sprintf("Invalid partNumber %d. The valid range is from %d to %d.",
			uploadPartRequest.PartNumber, MIN_PART_NUMBER, MAX_PART_NUMBER))
	}

	if uploadPartRequest.PartSize > 1024*1024*1024*5 {
		panic(fmt.Sprintf("PartNumber %d: Part Size should not be more than 5GB.", uploadPartRequest.PartSize))
	}

	params := map[string]string{
		"partNumber": strconv.Itoa(uploadPartRequest.PartNumber),
		"uploadId":   uploadPartRequest.UploadId,
	}

	req, err := bce.NewRequest("PUT", c.GetURL(bucketName, objectKey, params), uploadPartRequest.PartData)

	if err != nil {
		return nil, err
	}

	option = bce.CheckSignOption(option)
	option.AddHeaders(map[string]string{
		"Content-Length": strconv.FormatInt(uploadPartRequest.PartSize, 10),
		"Content-Type":   "application/octet-stream",
	})

	if _, ok := option.Headers["Content-MD5"]; !ok {
		option.AddHeader("Content-MD5", util.GetMD5(uploadPartRequest.PartData, true))
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	uploadPartResponse := NewUploadPartResponse(resp.Header)

	return uploadPartResponse, nil
}

func (c *Client) CompleteMultipartUpload(completeMultipartUploadRequest CompleteMultipartUploadRequest,
	option *bce.SignOption) (*CompleteMultipartUploadResponse, error) {

	bucketName := completeMultipartUploadRequest.BucketName
	objectKey := completeMultipartUploadRequest.ObjectKey
	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	completeMultipartUploadRequest.sort()
	params := map[string]string{"uploadId": completeMultipartUploadRequest.UploadId}
	byteArray, err := util.ToJson(completeMultipartUploadRequest, "parts")

	if err != nil {
		return nil, err
	}

	req, err := bce.NewRequest("POST", c.GetURL(bucketName, objectKey, params), bytes.NewReader(byteArray))

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var completeMultipartUploadResponse *CompleteMultipartUploadResponse

	err = json.Unmarshal(bodyContent, &completeMultipartUploadResponse)

	if err != nil {
		return nil, err
	}

	return completeMultipartUploadResponse, nil
}

func (c *Client) MultipartUploadFromFile(bucketName, objectKey, filePath string,
	partSize int64) (*CompleteMultipartUploadResponse, error) {

	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	initiateMultipartUploadRequest := InitiateMultipartUploadRequest{
		BucketName: bucketName,
		ObjectKey:  objectKey,
	}

	initiateMultipartUploadResponse, err := c.InitiateMultipartUpload(initiateMultipartUploadRequest, nil)

	if err != nil {
		return nil, err
	}

	uploadId := initiateMultipartUploadResponse.UploadId

	file, err := os.Open(filePath)
	defer file.Close()

	if err != nil {
		return nil, err
	}

	fileInfo, err := file.Stat()

	if err != nil {
		return nil, err
	}

	var totalSize int64 = fileInfo.Size()
	var partCount int = int(math.Ceil(float64(totalSize) / float64(partSize)))

	parts := make([]PartSummary, 0, partCount)

	var waitGroup sync.WaitGroup

	for i := 0; i < partCount; i++ {
		var skipBytes int64 = partSize * int64(i)
		var size int64 = int64(math.Min(float64(totalSize-skipBytes), float64(partSize)))

		tempFile, err := util.TempFile(nil, "", "")

		if err != nil {
			return nil, err
		}

		limitReader := io.LimitReader(file, size)
		_, err = io.Copy(tempFile, limitReader)

		if err != nil {
			return nil, err
		}

		partNumber := i + 1

		uploadPartRequest := UploadPartRequest{
			BucketName: bucketName,
			ObjectKey:  objectKey,
			UploadId:   uploadId,
			PartSize:   size,
			PartNumber: partNumber,
			PartData:   tempFile,
		}

		waitGroup.Add(1)

		parts = append(parts, PartSummary{PartNumber: partNumber})

		go func(partNumber int, f *os.File) {
			defer func() {
				f.Close()
				os.Remove(f.Name())
				waitGroup.Done()
			}()

			uploadPartResponse, uploadPartError := c.UploadPart(uploadPartRequest, nil)
			uploadPartRequest.PartData = nil

			if uploadPartError != nil {
				panic(uploadPartError)
			}

			parts[partNumber-1].ETag = uploadPartResponse.GetETag()
		}(partNumber, tempFile)
	}

	waitGroup.Wait()
	waitGroup.Add(1)

	var completeMultipartUploadResponse *CompleteMultipartUploadResponse

	go func() {
		defer waitGroup.Done()

		completeMultipartUploadRequest := CompleteMultipartUploadRequest{
			BucketName: bucketName,
			ObjectKey:  objectKey,
			UploadId:   uploadId,
			Parts:      parts,
		}

		completeResponse, completeError := c.CompleteMultipartUpload(completeMultipartUploadRequest, nil)

		if completeError != nil {
			panic(completeError)
		}

		completeMultipartUploadResponse = completeResponse
	}()

	waitGroup.Wait()

	return completeMultipartUploadResponse, nil
}

func (c *Client) AbortMultipartUpload(abortMultipartUploadRequest AbortMultipartUploadRequest,
	option *bce.SignOption) error {

	bucketName := abortMultipartUploadRequest.BucketName
	objectKey := abortMultipartUploadRequest.ObjectKey
	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	params := map[string]string{"uploadId": abortMultipartUploadRequest.UploadId}

	req, err := bce.NewRequest("DELETE", c.GetURL(bucketName, objectKey, params), nil)

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) ListParts(bucketName, objectKey, uploadId string,
	option *bce.SignOption) (*ListPartsResponse, error) {

	return c.ListPartsFromRequest(ListPartsRequest{
		BucketName: bucketName,
		ObjectKey:  objectKey,
		UploadId:   uploadId,
	}, option)
}

func (c *Client) ListPartsFromRequest(listPartsRequest ListPartsRequest,
	option *bce.SignOption) (*ListPartsResponse, error) {

	bucketName := listPartsRequest.BucketName
	objectKey := listPartsRequest.ObjectKey

	params := map[string]string{"uploadId": listPartsRequest.UploadId}

	if listPartsRequest.PartNumberMarker != "" {
		params["partNumberMarker"] = listPartsRequest.PartNumberMarker
	}

	if listPartsRequest.MaxParts > 0 {
		params["maxParts"] = strconv.Itoa(listPartsRequest.MaxParts)
	}

	req, err := bce.NewRequest("GET", c.GetURL(bucketName, objectKey, params), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var listPartsResponse *ListPartsResponse

	err = json.Unmarshal(bodyContent, &listPartsResponse)

	if err != nil {
		return nil, err
	}

	return listPartsResponse, nil
}

func (c *Client) ListMultipartUploads(bucketName string,
	option *bce.SignOption) (*ListMultipartUploadsResponse, error) {

	return c.ListMultipartUploadsFromRequest(ListMultipartUploadsRequest{BucketName: bucketName}, option)
}

func (c *Client) ListMultipartUploadsFromRequest(listMultipartUploadsRequest ListMultipartUploadsRequest,
	option *bce.SignOption) (*ListMultipartUploadsResponse, error) {

	bucketName := listMultipartUploadsRequest.BucketName

	params := map[string]string{"uploads": ""}

	if listMultipartUploadsRequest.Delimiter != "" {
		params["delimiter"] = listMultipartUploadsRequest.Delimiter
	}

	if listMultipartUploadsRequest.KeyMarker != "" {
		params["keyMarker"] = listMultipartUploadsRequest.KeyMarker
	}

	if listMultipartUploadsRequest.Prefix != "" {
		params["prefix"] = listMultipartUploadsRequest.Prefix
	}

	if listMultipartUploadsRequest.MaxUploads > 0 {
		params["maxUploads"] = strconv.Itoa(listMultipartUploadsRequest.MaxUploads)
	}

	req, err := bce.NewRequest("GET", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var listMultipartUploadsResponse *ListMultipartUploadsResponse

	err = json.Unmarshal(bodyContent, &listMultipartUploadsResponse)

	if err != nil {
		return nil, err
	}

	return listMultipartUploadsResponse, nil
}

func (c *Client) GetBucketCors(bucketName string, option *bce.SignOption) (*BucketCors, error) {
	params := map[string]string{"cors": ""}
	req, err := bce.NewRequest("GET", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var bucketCors *BucketCors

	err = json.Unmarshal(bodyContent, &bucketCors)

	if err != nil {
		return nil, err
	}

	return bucketCors, nil
}

func (c *Client) SetBucketCors(bucketName string, bucketCors BucketCors, option *bce.SignOption) error {
	byteArray, err := util.ToJson(bucketCors, "corsConfiguration")

	if err != nil {
		return err
	}

	params := map[string]string{"cors": ""}
	req, err := bce.NewRequest("PUT", c.GetURL(bucketName, "", params), bytes.NewReader(byteArray))

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) DeleteBucketCors(bucketName string, option *bce.SignOption) error {
	params := map[string]string{"cors": ""}
	req, err := bce.NewRequest("DELETE", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) OptionsObject(bucketName, objectKey, origin, accessControlRequestMethod,
	accessControlRequestHeaders string) (*bce.Response, error) {

	checkBucketName(bucketName)
	checkObjectKey(objectKey)

	req, err := bce.NewRequest("OPTIONS", c.GetURL(bucketName, objectKey, nil), nil)

	if err != nil {
		return nil, err
	}

	option := bce.CheckSignOption(nil)
	option.AddHeader("Origin", origin)
	option.AddHeader("Access-Control-Request-Method", accessControlRequestMethod)
	option.AddHeader("Access-Control-Request-Headers", accessControlRequestHeaders)

	return c.SendRequest(req, option)
}

func (c *Client) SetBucketLogging(bucketName, targetBucket, targetPrefix string, option *bce.SignOption) error {
	params := map[string]string{"logging": ""}
	body, err := util.ToJson(map[string]string{
		"targetBucket": targetBucket,
		"targetPrefix": targetPrefix,
	})

	if err != nil {
		return err
	}

	req, err := bce.NewRequest("PUT", c.GetURL(bucketName, "", params), bytes.NewReader(body))

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) GetBucketLogging(bucketName string, option *bce.SignOption) (*BucketLogging, error) {
	params := map[string]string{"logging": ""}
	req, err := bce.NewRequest("GET", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return nil, err
	}

	resp, err := c.SendRequest(req, option)

	if err != nil {
		return nil, err
	}

	bodyContent, err := resp.GetBodyContent()

	if err != nil {
		return nil, err
	}

	var bucketLogging *BucketLogging

	err = json.Unmarshal(bodyContent, &bucketLogging)

	if err != nil {
		return nil, err
	}

	return bucketLogging, nil
}

func (c *Client) DeleteBucketLogging(bucketName string, option *bce.SignOption) error {
	params := map[string]string{"logging": ""}
	req, err := bce.NewRequest("DELETE", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return err
	}

	_, err = c.SendRequest(req, option)

	return err
}

func (c *Client) setBucketAclFromString(bucketName, acl string, option *bce.SignOption) error {
	params := map[string]string{"acl": ""}
	req, err := bce.NewRequest("PUT", c.GetURL(bucketName, "", params), nil)

	if err != nil {
		return err
	}

	option = bce.CheckSignOption(option)

	headers := map[string]string{"x-bce-acl": acl}
	option.AddHeaders(headers)

	_, err = c.SendRequest(req, option)

	return err
}

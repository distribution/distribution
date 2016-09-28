package ks3

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/softlns/ks3-sdk-go/util"
)

var debug = false

// SetDebug sets debug mode to log the request/response message
func SetDebug(debugBool bool) {
	debug = debugBool
}

// The KS3 type encapsulates operations with an KS3 region.
type KS3 struct {
	util.Auth
	Region
	Client  *http.Client
	private byte // Reserve the right of using private data.
}

// The Bucket type encapsulates operations with an KS3 bucket.
type Bucket struct {
	*KS3
	Name string
}

// The Owner type represents the owner of the object in an KS3 bucket.
type Owner struct {
	ID          string
	DisplayName string
}

// Options fold options into an Options struct.
type Options struct {
	SSE                bool
	Meta               map[string][]string
	ContentEncoding    string
	CacheControl       string
	ContentMD5         string
	ContentDisposition string
	Range              string
	StorageClass       StorageClass
	// What else?
}

// CopyOptions encapsulates CopyOptions info.
type CopyOptions struct {
	Options
	CopySourceOptions string
	MetadataDirective string
	ContentType       string
}

// CopyObjectResult is the output from a Copy request
type CopyObjectResult struct {
	ETag         string
	LastModified string
}

var attempts = util.AttemptStrategy{
	Min:   5,
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

// New creates a new KS3.
func New(accessKey string, secretKey string, regionName string, secure bool, internal bool, regionEndpoint string) (*KS3, error) {
	auth, err := util.GetAuth(accessKey, secretKey, "", time.Time{})
	if err != nil {
		return nil, fmt.Errorf("unable to resolve ks3 credentials, please ensure that 'accesskey' and 'secretkey' are properly set : %v", err)
	}

	region := GetRegion(fmt.Sprint(regionName))
	if region.Name == "" {
		return nil, fmt.Errorf("Invalid region provided: %v", regionName)
	}
	region.SetCurrentUseEndpoint(internal)
	region.SetProtocol(secure)
	region.SetRegionEndpoint(regionEndpoint)

	return &KS3{
		Auth:    auth,
		Region:  region,
		Client:  http.DefaultClient,
		private: 0,
	}, nil
}

// Bucket returns a Bucket with the given name.
func (ks3 *KS3) Bucket(name string) *Bucket {
	name = strings.ToLower(name)
	return &Bucket{ks3, name}
}

// BucketInfo encapsulates Name and CreationDate of a bucket.
type BucketInfo struct {
	Name         string
	CreationDate string
}

// GetServiceResp encapsulates Owner and Buckets of a region.
type GetServiceResp struct {
	Owner   Owner
	Buckets []BucketInfo `xml:">Bucket"`
}

// GetService gets a list of all buckets owned by an account.
func (ks3 *KS3) GetService() (*GetServiceResp, error) {
	bucket := ks3.Bucket("")

	r, err := bucket.Get("")
	if err != nil {
		return nil, err
	}

	// Parse the XML response.
	var resp GetServiceResp
	if err = xml.Unmarshal(r, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

var createBucketConfiguration = `<CreateBucketConfiguration">
  <LocationConstraint>%s</LocationConstraint>
</CreateBucketConfiguration>`

// locationConstraint returns an io.Reader specifying a LocationConstraint if
// required for the region.
func (ks3 *KS3) locationConstraint() io.Reader {
	constraint := fmt.Sprintf(createBucketConfiguration, ks3.Region)
	return strings.NewReader(constraint)
}

// ACL type
type ACL string

const (
	Private           = ACL("private")
	PublicRead        = ACL("public-read")
	PublicReadWrite   = ACL("public-read-write")
	AuthenticatedRead = ACL("authenticated-read")
	BucketOwnerRead   = ACL("bucket-owner-read")
	BucketOwnerFull   = ACL("bucket-owner-full-control")
)

type StorageClass string

const (
	ReducedRedundancy = StorageClass("REDUCED_REDUNDANCY")
	StandardStorage   = StorageClass("STANDARD")
)

// PutBucket creates a new bucket.
//
// See http://ks3.ksyun.com/doc/api/bucket/put.html for details.
func (b *Bucket) PutBucket(perm ACL) error {
	headers := map[string][]string{
		"x-kss-acl": {string(perm)},
	}
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    "/",
		headers: headers,
		payload: b.locationConstraint(),
	}
	return b.KS3.query(req, nil)
}

// DelBucket removes an existing KS3 bucket. All objects in the bucket must
// be removed before the bucket itself can be removed.
//
// See http://ks3.ksyun.com/doc/api/bucket/delete.html for details.
func (b *Bucket) DelBucket() (err error) {
	req := &request{
		method: "DELETE",
		bucket: b.Name,
		path:   "/",
	}
	for attempt := attempts.Start(); attempt.Next(); {
		err = b.KS3.query(req, nil)
		if !shouldRetry(err) {
			break
		}
	}
	return err
}

// Get retrieves an object from an KS3 bucket.
//
// See http://ks3.ksyun.com/doc/api/object/get.html for details.
func (b *Bucket) Get(path string) (data []byte, err error) {
	body, err := b.GetReader(path)
	if err != nil {
		return nil, err
	}
	data, err = ioutil.ReadAll(body)
	body.Close()
	return data, err
}

// GetReader retrieves an object from an KS3 bucket,
// returning the body of the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading.
func (b *Bucket) GetReader(path string) (rc io.ReadCloser, err error) {
	resp, err := b.GetResponse(path)
	if resp != nil {
		return resp.Body, err
	}
	return nil, err
}

// GetResponse retrieves an object from an KS3 bucket,
// returning the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading
func (b *Bucket) GetResponse(path string) (resp *http.Response, err error) {
	return b.GetResponseWithHeaders(path, make(http.Header))
}

// GetResponseWithHeaders retrieves an object from an KS3 bucket
// Accepts custom headers to be sent as the second parameter
// returning the body of the HTTP response.
// It is the caller's responsibility to call Close on rc when
// finished reading
func (b *Bucket) GetResponseWithHeaders(path string, headers map[string][]string) (resp *http.Response, err error) {
	req := &request{
		bucket:  b.Name,
		path:    path,
		headers: headers,
	}
	err = b.KS3.prepare(req)
	if err != nil {
		return nil, err
	}
	for attempt := attempts.Start(); attempt.Next(); {
		resp, err := b.KS3.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	panic("unreachable")
}

// Exists checks whether or not an object exists on an KS3 bucket using a HEAD request.
func (b *Bucket) Exists(path string) (exists bool, err error) {
	req := &request{
		method: "HEAD",
		bucket: b.Name,
		path:   path,
	}
	err = b.KS3.prepare(req)
	if err != nil {
		return
	}
	for attempt := attempts.Start(); attempt.Next(); {
		resp, err := b.KS3.run(req, nil)

		if shouldRetry(err) && attempt.HasNext() {
			continue
		}

		if err != nil {
			// We can treat a 403 or 404 as non existence
			if e, ok := err.(*Error); ok && (e.StatusCode == 403 || e.StatusCode == 404) {
				return false, nil
			}
			return false, err
		}

		if resp.StatusCode/100 == 2 {
			exists = true
		}
		if resp.Body != nil {
			resp.Body.Close()
		}
		return exists, err
	}
	return false, fmt.Errorf("KS3 Currently Unreachable")
}

// Head HEADs an object in the KS3 bucket, returns the response with
// no body.
//
// See http://ks3.ksyun.com/doc/api/object/head.html for details.
func (b *Bucket) Head(path string, headers map[string][]string) (*http.Response, error) {
	req := &request{
		method:  "HEAD",
		bucket:  b.Name,
		path:    path,
		headers: headers,
	}
	err := b.KS3.prepare(req)
	if err != nil {
		return nil, err
	}

	for attempt := attempts.Start(); attempt.Next(); {
		resp, err := b.KS3.run(req, nil)
		if shouldRetry(err) && attempt.HasNext() {
			continue
		}
		if err != nil {
			return nil, err
		}
		return resp, err
	}
	return nil, fmt.Errorf("KS3 Currently Unreachable")
}

// Put inserts an object into the KS3 bucket.
//
// See http://ks3.ksyun.com/doc/api/object/put.html for details.
func (b *Bucket) Put(path string, data []byte, contType string, perm ACL, options Options) error {
	body := bytes.NewBuffer(data)
	return b.PutReader(path, body, int64(len(data)), contType, perm, options)
}

// PutCopy puts a copy of an object given by the key path into bucket b using b.Path as the target key.
func (b *Bucket) PutCopy(path string, perm ACL, options CopyOptions, source string) (*CopyObjectResult, error) {
	headers := map[string][]string{
		"x-kss-acl":         {string(perm)},
		"x-kss-copy-source": {escapePath(source)},
	}

	options.addHeaders(headers)
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
	}
	resp := &CopyObjectResult{}
	err := b.KS3.query(req, resp)
	if err != nil {
		return resp, err
	}
	return resp, nil
}

// PutReader inserts an object into the KS3 bucket by consuming data
// from r until EOF.
func (b *Bucket) PutReader(path string, r io.Reader, length int64, contType string, perm ACL, options Options) error {
	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(length, 10)},
		"ContentType":    {contType},
		"x-kss-acl":      {string(perm)},
	}

	options.addHeaders(headers)
	req := &request{
		method:  "PUT",
		bucket:  b.Name,
		path:    path,
		headers: headers,
		payload: r,
	}
	return b.KS3.query(req, nil)
}

// addHeaders adds o's specified fields to headers
func (o Options) addHeaders(headers map[string][]string) {
	if o.SSE {
		headers["x-kss-server-side-encryption"] = []string{"AES256"}
	}
	if len(o.Range) != 0 {
		headers["Range"] = []string{o.Range}
	}
	if len(o.ContentEncoding) != 0 {
		headers["Content-Encoding"] = []string{o.ContentEncoding}
	}
	if len(o.CacheControl) != 0 {
		headers["Cache-Control"] = []string{o.CacheControl}
	}
	if len(o.ContentMD5) != 0 {
		headers["Content-MD5"] = []string{o.ContentMD5}
	}
	if len(o.ContentDisposition) != 0 {
		headers["Content-Disposition"] = []string{o.ContentDisposition}
	}
	if len(o.StorageClass) != 0 {
		headers["x-kss-storage-class"] = []string{string(o.StorageClass)}

	}
	for k, v := range o.Meta {
		headers["x-kss-meta-"+k] = v
	}
}

// addHeaders adds o's specified fields to headers
func (o CopyOptions) addHeaders(headers map[string][]string) {
	o.Options.addHeaders(headers)
	if len(o.MetadataDirective) != 0 {
		headers["x-kss-metadata-directive"] = []string{o.MetadataDirective}
	}
	if len(o.CopySourceOptions) != 0 {
		headers["x-kss-copy-source-range"] = []string{o.CopySourceOptions}
	}
	if len(o.ContentType) != 0 {
		headers["Content-Type"] = []string{o.ContentType}
	}
}

func makeXmlBuffer(doc []byte) *bytes.Buffer {
	buf := new(bytes.Buffer)
	buf.WriteString(xml.Header)
	buf.Write(doc)
	return buf
}

// Del removes an object from the KS3 bucket.
//
// See http://ks3.ksyun.com/doc/api/object/delete.html for details.
func (b *Bucket) Del(path string) error {
	req := &request{
		method: "DELETE",
		bucket: b.Name,
		path:   path,
	}
	return b.KS3.query(req, nil)
}

// The Delete type
type Delete struct {
	Quiet   bool     `xml:"Quiet,omitempty"`
	Objects []Object `xml:"Object"`
}

// The Object type
type Object struct {
	Key       string `xml:"Key"`
	VersionId string `xml:"VersionId,omitempty"`
}

// DelMulti removes up to 1000 objects from the KS3 bucket.
func (b *Bucket) DelMulti(objects Delete) error {
	doc, err := xml.Marshal(objects)
	if err != nil {
		return err
	}

	buf := makeXmlBuffer(doc)
	digest := md5.New()
	size, err := digest.Write(buf.Bytes())
	if err != nil {
		return err
	}

	headers := map[string][]string{
		"Content-Length": {strconv.FormatInt(int64(size), 10)},
		"Content-MD5":    {base64.StdEncoding.EncodeToString(digest.Sum(nil))},
		"Content-Type":   {"text/xml"},
	}
	req := &request{
		path:    "/",
		method:  "POST",
		params:  url.Values{"delete": {""}},
		bucket:  b.Name,
		headers: headers,
		payload: buf,
	}

	return b.KS3.query(req, nil)
}

// The ListResp type holds the results of a List bucket operation.
type ListResp struct {
	Name      string
	Prefix    string
	Delimiter string
	Marker    string
	MaxKeys   int
	// IsTruncated is true if the results have been truncated because
	// there are more keys and prefixes than can fit in MaxKeys.
	// N.B. this is the opposite sense to that documented (incorrectly) in
	// http://goo.gl/YjQTc
	IsTruncated    bool
	Contents       []Key
	CommonPrefixes []string `xml:">Prefix"`
	// if IsTruncated is true, pass NextMarker as marker argument to List()
	// to get the next set of keys
	NextMarker string
}

// The Key type represents an item stored in an KS3 bucket.
type Key struct {
	Key          string
	LastModified string
	Size         int64
	// ETag gives the hex-encoded MD5 sum of the contents,
	// surrounded with double-quotes.
	ETag         string
	StorageClass string
	Owner        Owner
}

// List returns information about objects in an KS3 bucket.
//
// The prefix parameter limits the response to keys that begin with the
// specified prefix.
//
// The delim parameter causes the response to group all of the keys that
// share a common prefix up to the next delimiter in a single entry within
// the CommonPrefixes field. You can use delimiters to separate a bucket
// into different groupings of keys, similar to how folders would work.
//
// The marker parameter specifies the key to start with when listing objects
// in a bucket. Amazon KS3 lists objects in alphabetical order and
// will return keys alphabetically greater than the marker.
//
// The max parameter specifies how many keys + common prefixes to return in
// the response. The default is 1000.
//
// For example, given these keys in a bucket:
//
//     index.html
//     index2.html
//     photos/2006/January/sample.jpg
//     photos/2006/February/sample2.jpg
//     photos/2006/February/sample3.jpg
//     photos/2006/February/sample4.jpg
//
// Listing this bucket with delimiter set to "/" would yield the
// following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Contents:  []Key{
//             {Key: "index.html", "index2.html"},
//         },
//         CommonPrefixes: []string{
//             "photos/",
//         },
//     }
//
// Listing the same bucket with delimiter set to "/" and prefix set to
// "photos/2006/" would yield the following result:
//
//     &ListResp{
//         Name:      "sample-bucket",
//         MaxKeys:   1000,
//         Delimiter: "/",
//         Prefix:    "photos/2006/",
//         CommonPrefixes: []string{
//             "photos/2006/February/",
//             "photos/2006/January/",
//         },
//     }
//
// See http://ks3.ksyun.com/doc/api/bucket/get.html for details.
func (b *Bucket) List(prefix, delim, marker string, max int) (result *ListResp, err error) {
	params := map[string][]string{
		"prefix":    {prefix},
		"delimiter": {delim},
		"marker":    {marker},
	}
	if max != 0 {
		params["max-keys"] = []string{strconv.FormatInt(int64(max), 10)}
	}
	req := &request{
		bucket: b.Name,
		params: params,
	}
	result = &ListResp{}
	for attempt := attempts.Start(); attempt.Next(); {
		err = b.KS3.query(req, result)
		if !shouldRetry(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	// if NextMarker is not returned, it should be set to the name of last key,
	// so let's do it so that each caller doesn't have to
	if result.IsTruncated && result.NextMarker == "" {
		n := len(result.Contents)
		if n > 0 {
			result.NextMarker = result.Contents[n-1].Key
		}
	}
	return result, nil
}

// The GetLocationResp encapsulates Location info.
type GetLocationResp struct {
	Location string `xml:",innerxml"`
}

func (b *Bucket) Location() (string, error) {
	r, err := b.Get("/?location")
	if err != nil {
		return "", err
	}

	// Parse the XML response.
	var resp GetLocationResp
	if err = xml.Unmarshal(r, &resp); err != nil {
		return "", err
	}

	if resp.Location == "" {
		return "us-east-1", nil
	} else {
		return resp.Location, nil
	}
}

// URL returns a non-signed URL that allows retriving the
// object at path. It only works if the object is publicly
// readable (see SignedURL).
func (b *Bucket) URL(path string) string {
	req := &request{
		bucket: b.Name,
		path:   path,
	}
	err := b.KS3.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url()
	if err != nil {
		panic(err)
	}
	u.RawQuery = ""
	return u.String()
}

// SignedURL returns a signed URL that allows anyone holding the URL
// to retrieve the object at path. The signature is valid until expires.
func (b *Bucket) SignedURL(path string, expires time.Time) string {
	return b.SignedURLWithArgs(path, expires, nil, nil)
}

// SignedURLWithArgs returns a signed URL that allows anyone holding the URL
// to retrieve the object at path. The signature is valid until expires.
func (b *Bucket) SignedURLWithArgs(path string, expires time.Time, params url.Values, headers http.Header) string {
	return b.SignedURLWithMethod("GET", path, expires, params, headers)
}

// SignedURLWithMethod returns a signed URL that allows anyone holding the URL
// to either retrieve the object at path or make a HEAD request against it. The signature is valid until expires.
func (b *Bucket) SignedURLWithMethod(method, path string, expires time.Time, params url.Values, headers http.Header) string {
	var uv = url.Values{}

	if params != nil {
		uv = params
	}

	uv.Set("Expires", strconv.FormatInt(expires.Unix(), 10))

	req := &request{
		method:  method,
		bucket:  b.Name,
		path:    path,
		params:  uv,
		headers: headers,
	}
	err := b.KS3.prepare(req)
	if err != nil {
		panic(err)
	}
	u, err := req.url()
	if err != nil {
		panic(err)
	}
	if b.KS3.Auth.Token() != "" {
		return u.String() + "&x-kss-security-token=" + url.QueryEscape(req.headers["X-Kss-Security-Token"][0])
	} else {
		return u.String()
	}
}

// UploadSignedURL returns a signed URL that allows anyone holding the URL
// to upload the object at path. The signature is valid until expires.
// contenttype is a string like image/png
// name is the resource name in ks3 terminology like images/ali.png [obviously excluding the bucket name itself]
func (b *Bucket) UploadSignedURL(name, method, content_type string, expires time.Time) string {
	expire_date := expires.Unix()
	if method != "POST" {
		method = "PUT"
	}

	a := b.KS3.Auth
	tokenData := ""

	if a.Token() != "" {
		tokenData = "x-kss-security-token:" + a.Token() + "\n"
	}

	stringToSign := method + "\n\n" + content_type + "\n" + strconv.FormatInt(expire_date, 10) + "\n" + tokenData + "/" + path.Join(b.Name, name)
	secretKey := a.SecretKey
	accessId := a.AccessKey
	mac := hmac.New(sha1.New, []byte(secretKey))
	mac.Write([]byte(stringToSign))
	macsum := mac.Sum(nil)
	signature := base64.StdEncoding.EncodeToString([]byte(macsum))
	signature = strings.TrimSpace(signature)

	var signedurl *url.URL
	var err error

	signedurl, err = url.Parse(b.Region.GetEndpoint())
	name = b.Name + "/" + name

	if err != nil {
		log.Println("ERROR sining url for KS3 upload", err)
		return ""
	}
	signedurl.Path = name
	params := url.Values{}
	params.Add("KS3AccessKeyId", accessId)
	params.Add("Expires", strconv.FormatInt(expire_date, 10))
	params.Add("Signature", signature)
	if a.Token() != "" {
		params.Add("x-kss-security-token", a.Token())
	}

	signedurl.RawQuery = params.Encode()
	return signedurl.String()
}

// PostFormArgs returns the action and input fields needed to allow anonymous
// uploads to a bucket within the expiration limit
// Additional conditions can be specified with conds
func (b *Bucket) PostFormArgsEx(path string, expires time.Time, redirect string, conds []string) (action string, fields map[string]string) {
	conditions := make([]string, 0)
	fields = map[string]string{
		"KS3AccessKeyId": b.Auth.AccessKey,
		"key":            path,
	}

	if token := b.KS3.Auth.Token(); token != "" {
		fields["x-kss-security-token"] = token
		conditions = append(conditions,
			fmt.Sprintf("{\"x-kss-security-token\": \"%s\"}", token))
	}

	if conds != nil {
		conditions = append(conditions, conds...)
	}

	conditions = append(conditions, fmt.Sprintf("{\"key\": \"%s\"}", path))
	conditions = append(conditions, fmt.Sprintf("{\"bucket\": \"%s\"}", b.Name))
	if redirect != "" {
		conditions = append(conditions, fmt.Sprintf("{\"success_action_redirect\": \"%s\"}", redirect))
		fields["success_action_redirect"] = redirect
	}

	vExpiration := expires.Format("2006-01-02T15:04:05Z")
	vConditions := strings.Join(conditions, ",")
	policy := fmt.Sprintf("{\"expiration\": \"%s\", \"conditions\": [%s]}", vExpiration, vConditions)
	policy64 := base64.StdEncoding.EncodeToString([]byte(policy))
	fields["policy"] = policy64

	signer := hmac.New(sha1.New, []byte(b.Auth.SecretKey))
	signer.Write([]byte(policy64))
	fields["signature"] = base64.StdEncoding.EncodeToString(signer.Sum(nil))

	action = fmt.Sprintf("%s/%s/", b.KS3.Region.GetEndpoint(), b.Name)
	return
}

// PostFormArgs returns the action and input fields needed to allow anonymous
// uploads to a bucket within the expiration limit
func (b *Bucket) PostFormArgs(path string, expires time.Time, redirect string) (action string, fields map[string]string) {
	return b.PostFormArgsEx(path, expires, redirect, nil)
}

type request struct {
	method   string
	bucket   string
	path     string
	params   url.Values
	headers  http.Header
	baseurl  string
	payload  io.Reader
	prepared bool
}

func (req *request) url() (*url.URL, error) {
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return nil, fmt.Errorf("bad KS3 endpoint URL %q: %v", req.baseurl, err)
	}
	u.RawQuery = req.params.Encode()
	u.Path = req.path
	return u, nil
}

// query prepares and runs the req request.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (ks3 *KS3) query(req *request, resp interface{}) error {
	err := ks3.prepare(req)
	if err != nil {
		return err
	}
	r, err := ks3.run(req, resp)
	if r != nil && r.Body != nil {
		r.Body.Close()
	}
	return err
}

// Sets baseurl on req from bucket name and the region endpoint
func (ks3 *KS3) setBaseURL(req *request) error {
	if ks3.Region.RegionEndpoint == "" {
		req.baseurl = ks3.Region.GetBucketEndpoint(req.bucket)
	} else {
		req.baseurl = fmt.Sprintf("%s://%s", ks3.Region.Protocl, ks3.Region.RegionEndpoint)
	}

	return nil
}

// partiallyEscapedPath partially escapes the KS3 path allowing for all KS3 REST API calls.
//
// Some commands including:
//      GET Bucket acl              http://goo.gl/aoXflF
//      GET Bucket cors             http://goo.gl/UlmBdx
//      GET Bucket lifecycle        http://goo.gl/8Fme7M
//      GET Bucket policy           http://goo.gl/ClXIo3
//      GET Bucket location         http://goo.gl/5lh8RD
//      GET Bucket Logging          http://goo.gl/sZ5ckF
//      GET Bucket notification     http://goo.gl/qSSZKD
//      GET Bucket tagging          http://goo.gl/QRvxnM
// require the first character after the bucket name in the path to be a literal '?' and
// not the escaped hex representation '%3F'.
func partiallyEscapedPath(path string) string {
	pathEscapedAndSplit := strings.Split((&url.URL{Path: path}).String(), "/")
	if len(pathEscapedAndSplit) >= 3 {
		if len(pathEscapedAndSplit[2]) >= 3 {
			// Check for the one "?" that should not be escaped.
			if pathEscapedAndSplit[2][0:3] == "%3F" {
				pathEscapedAndSplit[2] = "?" + pathEscapedAndSplit[2][3:]
			}
		}
	}
	return strings.Replace(strings.Join(pathEscapedAndSplit, "/"), "+", "%2B", -1)
}

// prepare sets up req to be delivered to KS3.
func (ks3 *KS3) prepare(req *request) error {
	// Copy so they can be mutated without affecting on retries.
	params := make(url.Values)
	headers := make(http.Header)
	for k, v := range req.params {
		params[k] = v
	}
	for k, v := range req.headers {
		headers[k] = v
	}
	req.params = params
	req.headers = headers

	if !req.prepared {
		req.prepared = true
		if req.method == "" {
			req.method = "GET"
		}

		if !strings.HasPrefix(req.path, "/") {
			req.path = "/" + req.path
		}

		err := ks3.setBaseURL(req)
		if err != nil {
			return err
		}
	}

	if ks3.Auth.Token() != "" {
		req.headers["X-Kss-Security-Token"] = []string{ks3.Auth.Token()}
	}

	// Always sign again as it's not clear how far the
	// server has handled a previous attempt.
	u, err := url.Parse(req.baseurl)
	if err != nil {
		return err
	}

	signpathPartiallyEscaped := partiallyEscapedPath(req.path)
	if len(req.bucket) > 0 {
		signpathPartiallyEscaped = "/" + req.bucket + signpathPartiallyEscaped
	}
	req.headers["Host"] = []string{u.Host}
	req.headers["Date"] = []string{time.Now().In(time.UTC).Format(time.RFC1123)}

	sign(ks3.Auth, req.method, signpathPartiallyEscaped, req.params, req.headers)
	return nil
}

// Prepares an *http.Request for doHttpRequest
func (ks3 *KS3) setupHttpRequest(req *request) (*http.Request, error) {
	// Copy so that signing the http request will not mutate it
	headers := make(http.Header)
	for k, v := range req.headers {
		headers[k] = v
	}
	req.headers = headers

	u, err := req.url()
	if err != nil {
		return nil, err
	}

	if ks3.Region.Name != "generic" {
		u.Opaque = fmt.Sprintf("//%s%s", u.Host, partiallyEscapedPath(u.Path))
	}

	hreq := http.Request{
		URL:        u,
		Method:     req.method,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     req.headers,
		Form:       req.params,
	}

	if v, ok := req.headers["Content-Length"]; ok {
		hreq.ContentLength, _ = strconv.ParseInt(v[0], 10, 64)
		delete(req.headers, "Content-Length")
	}
	if req.payload != nil {
		hreq.Body = ioutil.NopCloser(req.payload)
	}

	return &hreq, nil
}

// doHttpRequest sends hreq and returns the http response from the server.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (ks3 *KS3) doHttpRequest(hreq *http.Request, resp interface{}) (*http.Response, error) {
	hresp, err := ks3.Client.Do(hreq)
	if err != nil {
		return nil, err
	}
	if debug {
		dump, _ := httputil.DumpResponse(hresp, true)
		log.Printf("} -> %s\n", dump)
	}
	if hresp.StatusCode != 200 && hresp.StatusCode != 204 && hresp.StatusCode != 206 {
		return nil, buildError(hresp)
	}
	if resp != nil {
		err = xml.NewDecoder(hresp.Body).Decode(resp)
		hresp.Body.Close()

		if debug {
			log.Printf("goamz.ks3> decoded xml into %#v", resp)
		}

	}
	return hresp, err
}

// run sends req and returns the http response from the server.
// If resp is not nil, the XML data contained in the response
// body will be unmarshalled on it.
func (ks3 *KS3) run(req *request, resp interface{}) (*http.Response, error) {
	if debug {
		log.Printf("Running KS3 request: %#v", req)
	}

	hreq, err := ks3.setupHttpRequest(req)
	if err != nil {
		return nil, err
	}

	return ks3.doHttpRequest(hreq, resp)
}

// Error represents an error in an operation with KS3.
type Error struct {
	StatusCode int    // HTTP status code (200, 403, ...)
	Code       string // KS3 error code ("UnsupportedOperation", ...)
	Message    string // The human-oriented error message
	BucketName string
	RequestId  string
	HostId     string
}

func (e *Error) Error() string {
	return fmt.Sprintf("KS3 API Error: RequestId: %s Status Code: %d Code: %s Message: %s", e.RequestId, e.StatusCode, e.Code, e.Message)
}

func buildError(r *http.Response) error {
	if debug {
		log.Printf("got error (status code %v)", r.StatusCode)
		data, err := ioutil.ReadAll(r.Body)
		if err != nil {
			log.Printf("\tread error: %v", err)
		} else {
			log.Printf("\tdata:\n%s\n\n", data)
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(data))
	}

	err := Error{}
	// TODO(softlns) return error if Unmarshal fails?
	xml.NewDecoder(r.Body).Decode(&err)
	r.Body.Close()
	err.StatusCode = r.StatusCode
	if err.Message == "" {
		err.Message = r.Status
	}
	if debug {
		log.Printf("err: %#v\n", err)
	}
	return &err
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	switch err {
	case io.ErrUnexpectedEOF, io.EOF:
		return true
	}
	switch e := err.(type) {
	case *net.DNSError:
		return true
	case *net.OpError:
		switch e.Op {
		case "dial", "read", "write":
			return true
		}
	case *url.Error:
		// url.Error can be returned either by net/url if a URL cannot be
		// parsed, or by net/http if the response is closed before the headers
		// are received or parsed correctly. In that later case, e.Op is set to
		// the HTTP method name with the first letter uppercased. We don't want
		// to retry on POST operations, since those are not idempotent, all the
		// other ones should be safe to retry. The only case where all
		// operations are safe to retry are "dial" errors, since in that case
		// the POST request didn't make it to the server.

		if netErr, ok := e.Err.(*net.OpError); ok && netErr.Op == "dial" {
			return true
		}

		switch e.Op {
		case "Get", "Put", "Delete", "Head":
			return shouldRetry(e.Err)
		default:
			return false
		}
	case *Error:
		switch e.Code {
		case "InternalError", "NoSuchUpload", "NoSuchBucket":
			return true
		}
		switch e.StatusCode {
		case 500, 503, 504:
			return true
		}
	}
	return false
}

func hasCode(err error, code string) bool {
	ks3err, ok := err.(*Error)
	return ok && ks3err.Code == code
}

func escapePath(s string) string {
	return (&url.URL{Path: s}).String()
}

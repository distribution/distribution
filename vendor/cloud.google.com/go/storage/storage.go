// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package storage contains a Google Cloud Storage client.
//
// This package is experimental and may make backwards-incompatible changes.
package storage // import "cloud.google.com/go/storage"

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/api/option"
	"google.golang.org/api/transport"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/storage/v1"
)

var (
	ErrBucketNotExist = errors.New("storage: bucket doesn't exist")
	ErrObjectNotExist = errors.New("storage: object doesn't exist")

	// Done is returned by iterators in this package when they have no more items.
	Done = errors.New("storage: no more results")
)

const userAgent = "gcloud-golang-storage/20151204"

const (
	// ScopeFullControl grants permissions to manage your
	// data and permissions in Google Cloud Storage.
	ScopeFullControl = raw.DevstorageFullControlScope

	// ScopeReadOnly grants permissions to
	// view your data in Google Cloud Storage.
	ScopeReadOnly = raw.DevstorageReadOnlyScope

	// ScopeReadWrite grants permissions to manage your
	// data in Google Cloud Storage.
	ScopeReadWrite = raw.DevstorageReadWriteScope
)

// AdminClient is a client type for performing admin operations on a project's
// buckets.
//
// Deprecated: Client has all of AdminClient's methods.
type AdminClient struct {
	c         *Client
	projectID string
}

// NewAdminClient creates a new AdminClient for a given project.
//
// Deprecated: use NewClient instead.
func NewAdminClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*AdminClient, error) {
	c, err := NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &AdminClient{
		c:         c,
		projectID: projectID,
	}, nil
}

// Close closes the AdminClient.
func (c *AdminClient) Close() error {
	return c.c.Close()
}

// Create creates a Bucket in the project.
// If attrs is nil the API defaults will be used.
//
// Deprecated: use BucketHandle.Create instead.
func (c *AdminClient) CreateBucket(ctx context.Context, bucketName string, attrs *BucketAttrs) error {
	return c.c.Bucket(bucketName).Create(ctx, c.projectID, attrs)
}

// Delete deletes a Bucket in the project.
//
// Deprecated: use BucketHandle.Delete instead.
func (c *AdminClient) DeleteBucket(ctx context.Context, bucketName string) error {
	return c.c.Bucket(bucketName).Delete(ctx)
}

// Client is a client for interacting with Google Cloud Storage.
type Client struct {
	hc  *http.Client
	raw *raw.Service
}

// NewClient creates a new Google Cloud Storage client.
// The default scope is ScopeFullControl. To use a different scope, like ScopeReadOnly, use option.WithScopes.
func NewClient(ctx context.Context, opts ...option.ClientOption) (*Client, error) {
	o := []option.ClientOption{
		option.WithScopes(ScopeFullControl),
		option.WithUserAgent(userAgent),
	}
	opts = append(o, opts...)
	hc, _, err := transport.NewHTTPClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("dialing: %v", err)
	}
	rawService, err := raw.New(hc)
	if err != nil {
		return nil, fmt.Errorf("storage client: %v", err)
	}
	return &Client{
		hc:  hc,
		raw: rawService,
	}, nil
}

// Close closes the Client.
func (c *Client) Close() error {
	c.hc = nil
	return nil
}

// BucketHandle provides operations on a Google Cloud Storage bucket.
// Use Client.Bucket to get a handle.
type BucketHandle struct {
	acl              *ACLHandle
	defaultObjectACL *ACLHandle

	c    *Client
	name string
}

// Bucket returns a BucketHandle, which provides operations on the named bucket.
// This call does not perform any network operations.
//
// name must contain only lowercase letters, numbers, dashes, underscores, and
// dots. The full specification for valid bucket names can be found at:
//   https://cloud.google.com/storage/docs/bucket-naming
func (c *Client) Bucket(name string) *BucketHandle {
	return &BucketHandle{
		c:    c,
		name: name,
		acl: &ACLHandle{
			c:      c,
			bucket: name,
		},
		defaultObjectACL: &ACLHandle{
			c:         c,
			bucket:    name,
			isDefault: true,
		},
	}
}

// Create creates the Bucket in the project.
// If attrs is nil the API defaults will be used.
func (b *BucketHandle) Create(ctx context.Context, projectID string, attrs *BucketAttrs) error {
	var bkt *raw.Bucket
	if attrs != nil {
		bkt = attrs.toRawBucket()
	} else {
		bkt = &raw.Bucket{}
	}
	bkt.Name = b.name
	req := b.c.raw.Buckets.Insert(projectID, bkt)
	_, err := req.Context(ctx).Do()
	return err
}

// Delete deletes the Bucket.
func (b *BucketHandle) Delete(ctx context.Context) error {
	req := b.c.raw.Buckets.Delete(b.name)
	return req.Context(ctx).Do()
}

// ACL returns an ACLHandle, which provides access to the bucket's access control list.
// This controls who can list, create or overwrite the objects in a bucket.
// This call does not perform any network operations.
func (c *BucketHandle) ACL() *ACLHandle {
	return c.acl
}

// DefaultObjectACL returns an ACLHandle, which provides access to the bucket's default object ACLs.
// These ACLs are applied to newly created objects in this bucket that do not have a defined ACL.
// This call does not perform any network operations.
func (c *BucketHandle) DefaultObjectACL() *ACLHandle {
	return c.defaultObjectACL
}

// Object returns an ObjectHandle, which provides operations on the named object.
// This call does not perform any network operations.
//
// name must consist entirely of valid UTF-8-encoded runes. The full specification
// for valid object names can be found at:
//   https://cloud.google.com/storage/docs/bucket-naming
func (b *BucketHandle) Object(name string) *ObjectHandle {
	return &ObjectHandle{
		c:      b.c,
		bucket: b.name,
		object: name,
		acl: &ACLHandle{
			c:      b.c,
			bucket: b.name,
			object: name,
		},
	}
}

// TODO(jbd): Add storage.buckets.list.
// TODO(jbd): Add storage.buckets.update.

// TODO(jbd): Add storage.objects.watch.

// Attrs returns the metadata for the bucket.
func (b *BucketHandle) Attrs(ctx context.Context) (*BucketAttrs, error) {
	resp, err := b.c.raw.Buckets.Get(b.name).Projection("full").Context(ctx).Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrBucketNotExist
	}
	if err != nil {
		return nil, err
	}
	return newBucket(resp), nil
}

// List lists objects from the bucket. You can specify a query
// to filter the results. If q is nil, no filtering is applied.
//
// Deprecated. Use BucketHandle.Objects instead.
func (b *BucketHandle) List(ctx context.Context, q *Query) (*ObjectList, error) {
	it := b.Objects(ctx, q)
	attrs, pres, err := it.NextPage()
	if err != nil && err != Done {
		return nil, err
	}
	objects := &ObjectList{
		Results:  attrs,
		Prefixes: pres,
	}
	if it.NextPageToken() != "" {
		objects.Next = &it.query
	}
	return objects, nil
}

func (b *BucketHandle) Objects(ctx context.Context, q *Query) *ObjectIterator {
	it := &ObjectIterator{
		ctx:    ctx,
		bucket: b,
	}
	if q != nil {
		it.query = *q
	}
	return it
}

type ObjectIterator struct {
	ctx      context.Context
	bucket   *BucketHandle
	query    Query
	pageSize int32
	objs     []*ObjectAttrs
	prefixes []string
	err      error
}

// Next returns the next result. Its second return value is Done if there are
// no more results. Once Next returns Done, all subsequent calls will return
// Done.
//
// Internally, Next retrieves results in bulk. You can call SetPageSize as a
// performance hint to affect how many results are retrieved in a single RPC.
//
// SetPageToken should not be called when using Next.
//
// Next and NextPage should not be used with the same iterator.
//
// If Query.Delimiter is non-empty, Next returns an error. Use NextPage when using delimiters.
func (it *ObjectIterator) Next() (*ObjectAttrs, error) {
	if it.query.Delimiter != "" {
		return nil, errors.New("cannot use ObjectIterator.Next with a delimiter")
	}
	for len(it.objs) == 0 { // "for", not "if", to handle empty pages
		if it.err != nil {
			return nil, it.err
		}
		it.nextPage()
		if it.err != nil {
			it.objs = nil
			return nil, it.err
		}
		if it.query.Cursor == "" {
			it.err = Done
		}
	}
	o := it.objs[0]
	it.objs = it.objs[1:]
	return o, nil
}

const DefaultPageSize = 1000

// NextPage returns the next page of results, both objects (as *ObjectAttrs)
// and prefixes. Prefixes will be nil if query.Delimiter is empty.
//
// NextPage will return exactly the number of results (the total of objects and
// prefixes) specified by the last call to SetPageSize, unless there are not
// enough results available. If no page size was specified, it uses
// DefaultPageSize.
//
// NextPage may return a second return value of Done along with the last page
// of results.
//
// After NextPage returns Done, all subsequent calls to NextPage will return
// (nil, Done).
//
// Next and NextPage should not be used with the same iterator.
func (it *ObjectIterator) NextPage() (objs []*ObjectAttrs, prefixes []string, err error) {
	defer it.SetPageSize(it.pageSize) // restore value at entry
	if it.pageSize <= 0 {
		it.pageSize = DefaultPageSize
	}
	for len(objs)+len(prefixes) < int(it.pageSize) {
		it.pageSize -= int32(len(objs) + len(prefixes))
		it.nextPage()
		if it.err != nil {
			return nil, nil, it.err
		}
		objs = append(objs, it.objs...)
		prefixes = append(prefixes, it.prefixes...)
		if it.query.Cursor == "" {
			it.err = Done
			return objs, prefixes, it.err
		}
	}
	return objs, prefixes, it.err
}

// nextPage gets the next page of results by making a single call to the underlying method.
// It sets it.objs, it.prefixes, it.query.Cursor, and it.err. It never sets it.err to Done.
func (it *ObjectIterator) nextPage() {
	if it.err != nil {
		return
	}
	req := it.bucket.c.raw.Objects.List(it.bucket.name)
	req.Projection("full")
	req.Delimiter(it.query.Delimiter)
	req.Prefix(it.query.Prefix)
	req.Versions(it.query.Versions)
	req.PageToken(it.query.Cursor)
	if it.pageSize > 0 {
		req.MaxResults(int64(it.pageSize))
	}
	resp, err := req.Context(it.ctx).Do()
	if err != nil {
		it.err = err
		return
	}
	it.query.Cursor = resp.NextPageToken
	it.objs = nil
	for _, item := range resp.Items {
		it.objs = append(it.objs, newObject(item))
	}
	it.prefixes = resp.Prefixes
}

// SetPageSize sets the page size for all subsequent calls to NextPage.
// NextPage will return exactly this many items if they are present.
func (it *ObjectIterator) SetPageSize(pageSize int32) {
	it.pageSize = pageSize
}

// SetPageToken sets the page token for the next call to NextPage, to resume
// the iteration from a previous point.
func (it *ObjectIterator) SetPageToken(t string) {
	it.query.Cursor = t
}

// NextPageToken returns a page token that can be used with SetPageToken to
// resume iteration from the next page. It returns the empty string if there
// are no more pages. For an example, see SetPageToken.
func (it *ObjectIterator) NextPageToken() string {
	return it.query.Cursor
}

// SignedURLOptions allows you to restrict the access to the signed URL.
type SignedURLOptions struct {
	// GoogleAccessID represents the authorizer of the signed URL generation.
	// It is typically the Google service account client email address from
	// the Google Developers Console in the form of "xxx@developer.gserviceaccount.com".
	// Required.
	GoogleAccessID string

	// PrivateKey is the Google service account private key. It is obtainable
	// from the Google Developers Console.
	// At https://console.developers.google.com/project/<your-project-id>/apiui/credential,
	// create a service account client ID or reuse one of your existing service account
	// credentials. Click on the "Generate new P12 key" to generate and download
	// a new private key. Once you download the P12 file, use the following command
	// to convert it into a PEM file.
	//
	//    $ openssl pkcs12 -in key.p12 -passin pass:notasecret -out key.pem -nodes
	//
	// Provide the contents of the PEM file as a byte slice.
	// Exactly one of PrivateKey or SignBytes must be non-nil.
	PrivateKey []byte

	// SignBytes is a function for implementing custom signing.
	// If your application is running on Google App Engine, you can use appengine's internal signing function:
	//     ctx := appengine.NewContext(request)
	//     acc, _ := appengine.ServiceAccount(ctx)
	//     url, err := SignedURL("bucket", "object", &SignedURLOptions{
	//     	GoogleAccessID: acc,
	//     	SignBytes: func(b []byte) ([]byte, error) {
	//     		_, signedBytes, err := appengine.SignBytes(ctx, b)
	//     		return signedBytes, err
	//     	},
	//     	// etc.
	//     })
	//
	// Exactly one of PrivateKey or SignBytes must be non-nil.
	SignBytes func([]byte) ([]byte, error)

	// Method is the HTTP method to be used with the signed URL.
	// Signed URLs can be used with GET, HEAD, PUT, and DELETE requests.
	// Required.
	Method string

	// Expires is the expiration time on the signed URL. It must be
	// a datetime in the future.
	// Required.
	Expires time.Time

	// ContentType is the content type header the client must provide
	// to use the generated signed URL.
	// Optional.
	ContentType string

	// Headers is a list of extention headers the client must provide
	// in order to use the generated signed URL.
	// Optional.
	Headers []string

	// MD5 is the base64 encoded MD5 checksum of the file.
	// If provided, the client should provide the exact value on the request
	// header in order to use the signed URL.
	// Optional.
	MD5 []byte
}

// SignedURL returns a URL for the specified object. Signed URLs allow
// the users access to a restricted resource for a limited time without having a
// Google account or signing in. For more information about the signed
// URLs, see https://cloud.google.com/storage/docs/accesscontrol#Signed-URLs.
func SignedURL(bucket, name string, opts *SignedURLOptions) (string, error) {
	if opts == nil {
		return "", errors.New("storage: missing required SignedURLOptions")
	}
	if opts.GoogleAccessID == "" {
		return "", errors.New("storage: missing required GoogleAccessID")
	}
	if (opts.PrivateKey == nil) == (opts.SignBytes == nil) {
		return "", errors.New("storage: exactly one of PrivateKey or SignedBytes must be set")
	}
	if opts.Method == "" {
		return "", errors.New("storage: missing required method option")
	}
	if opts.Expires.IsZero() {
		return "", errors.New("storage: missing required expires option")
	}

	signBytes := opts.SignBytes
	if opts.PrivateKey != nil {
		key, err := parseKey(opts.PrivateKey)
		if err != nil {
			return "", err
		}
		signBytes = func(b []byte) ([]byte, error) {
			sum := sha256.Sum256(b)
			return rsa.SignPKCS1v15(
				rand.Reader,
				key,
				crypto.SHA256,
				sum[:],
			)
		}
	} else {
		signBytes = opts.SignBytes
	}

	u := &url.URL{
		Path: fmt.Sprintf("/%s/%s", bucket, name),
	}

	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "%s\n", opts.Method)
	fmt.Fprintf(buf, "%s\n", opts.MD5)
	fmt.Fprintf(buf, "%s\n", opts.ContentType)
	fmt.Fprintf(buf, "%d\n", opts.Expires.Unix())
	fmt.Fprintf(buf, "%s", strings.Join(opts.Headers, "\n"))
	fmt.Fprintf(buf, "%s", u.String())

	b, err := signBytes(buf.Bytes())
	if err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(b)
	u.Scheme = "https"
	u.Host = "storage.googleapis.com"
	q := u.Query()
	q.Set("GoogleAccessId", opts.GoogleAccessID)
	q.Set("Expires", fmt.Sprintf("%d", opts.Expires.Unix()))
	q.Set("Signature", string(encoded))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ObjectHandle provides operations on an object in a Google Cloud Storage bucket.
// Use BucketHandle.Object to get a handle.
type ObjectHandle struct {
	c      *Client
	bucket string
	object string

	acl   *ACLHandle
	conds []Condition
}

// ACL provides access to the object's access control list.
// This controls who can read and write this object.
// This call does not perform any network operations.
func (o *ObjectHandle) ACL() *ACLHandle {
	return o.acl
}

// WithConditions returns a copy of o using the provided conditions.
func (o *ObjectHandle) WithConditions(conds ...Condition) *ObjectHandle {
	o2 := *o
	o2.conds = conds
	return &o2
}

// Attrs returns meta information about the object.
// ErrObjectNotExist will be returned if the object is not found.
func (o *ObjectHandle) Attrs(ctx context.Context) (*ObjectAttrs, error) {
	if !utf8.ValidString(o.object) {
		return nil, fmt.Errorf("storage: object name %q is not valid UTF-8", o.object)
	}
	call := o.c.raw.Objects.Get(o.bucket, o.object).Projection("full").Context(ctx)
	if err := applyConds("Attrs", o.conds, call); err != nil {
		return nil, err
	}
	obj, err := call.Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExist
	}
	if err != nil {
		return nil, err
	}
	return newObject(obj), nil
}

// Update updates an object with the provided attributes.
// All zero-value attributes are ignored.
// ErrObjectNotExist will be returned if the object is not found.
func (o *ObjectHandle) Update(ctx context.Context, attrs ObjectAttrs) (*ObjectAttrs, error) {
	if !utf8.ValidString(o.object) {
		return nil, fmt.Errorf("storage: object name %q is not valid UTF-8", o.object)
	}
	call := o.c.raw.Objects.Patch(o.bucket, o.object, attrs.toRawObject(o.bucket)).Projection("full").Context(ctx)
	if err := applyConds("Update", o.conds, call); err != nil {
		return nil, err
	}
	obj, err := call.Do()
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return nil, ErrObjectNotExist
	}
	if err != nil {
		return nil, err
	}
	return newObject(obj), nil
}

// Delete deletes the single specified object.
func (o *ObjectHandle) Delete(ctx context.Context) error {
	if !utf8.ValidString(o.object) {
		return fmt.Errorf("storage: object name %q is not valid UTF-8", o.object)
	}
	call := o.c.raw.Objects.Delete(o.bucket, o.object).Context(ctx)
	if err := applyConds("Delete", o.conds, call); err != nil {
		return err
	}
	err := call.Do()
	switch e := err.(type) {
	case nil:
		return nil
	case *googleapi.Error:
		if e.Code == http.StatusNotFound {
			return ErrObjectNotExist
		}
	}
	return err
}

// CopyTo copies the object to the given dst.
// The copied object's attributes are overwritten by attrs if non-nil.
func (o *ObjectHandle) CopyTo(ctx context.Context, dst *ObjectHandle, attrs *ObjectAttrs) (*ObjectAttrs, error) {
	// TODO(djd): move bucket/object name validation to a single helper func.
	if o.bucket == "" || dst.bucket == "" {
		return nil, errors.New("storage: the source and destination bucket names must both be non-empty")
	}
	if o.object == "" || dst.object == "" {
		return nil, errors.New("storage: the source and destination object names must both be non-empty")
	}
	if !utf8.ValidString(o.object) {
		return nil, fmt.Errorf("storage: object name %q is not valid UTF-8", o.object)
	}
	if !utf8.ValidString(dst.object) {
		return nil, fmt.Errorf("storage: dst name %q is not valid UTF-8", dst.object)
	}
	var rawObject *raw.Object
	if attrs != nil {
		attrs.Name = dst.object
		if attrs.ContentType == "" {
			return nil, errors.New("storage: attrs.ContentType must be non-empty")
		}
		rawObject = attrs.toRawObject(dst.bucket)
	}
	call := o.c.raw.Objects.Copy(o.bucket, o.object, dst.bucket, dst.object, rawObject).Projection("full").Context(ctx)
	if err := applyConds("CopyTo destination", dst.conds, call); err != nil {
		return nil, err
	}
	if err := applyConds("CopyTo source", toSourceConds(o.conds), call); err != nil {
		return nil, err
	}
	obj, err := call.Do()
	if err != nil {
		return nil, err
	}
	return newObject(obj), nil
}

// NewReader creates a new Reader to read the contents of the
// object.
// ErrObjectNotExist will be returned if the object is not found.
func (o *ObjectHandle) NewReader(ctx context.Context) (*Reader, error) {
	return o.NewRangeReader(ctx, 0, -1)
}

// NewRangeReader reads part of an object, reading at most length bytes
// starting at the given offset.  If length is negative, the object is read
// until the end.
func (o *ObjectHandle) NewRangeReader(ctx context.Context, offset, length int64) (*Reader, error) {
	if !utf8.ValidString(o.object) {
		return nil, fmt.Errorf("storage: object name %q is not valid UTF-8", o.object)
	}
	if offset < 0 {
		return nil, fmt.Errorf("storage: invalid offset %d < 0", offset)
	}
	u := &url.URL{
		Scheme: "https",
		Host:   "storage.googleapis.com",
		Path:   fmt.Sprintf("/%s/%s", o.bucket, o.object),
	}
	verb := "GET"
	if length == 0 {
		verb = "HEAD"
	}
	req, err := http.NewRequest(verb, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if err := applyConds("NewReader", o.conds, objectsGetCall{req}); err != nil {
		return nil, err
	}
	if length < 0 && offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	} else if length > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	}
	res, err := o.c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusNotFound {
		res.Body.Close()
		return nil, ErrObjectNotExist
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		body, _ := ioutil.ReadAll(res.Body)
		res.Body.Close()
		return nil, &googleapi.Error{
			Code:   res.StatusCode,
			Header: res.Header,
			Body:   string(body),
		}
	}
	if offset > 0 && length != 0 && res.StatusCode != http.StatusPartialContent {
		res.Body.Close()
		return nil, errors.New("storage: partial request not satisfied")
	}
	clHeader := res.Header.Get("X-Goog-Stored-Content-Length")
	cl, err := strconv.ParseInt(clHeader, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("storage: can't parse content length %q: %v", clHeader, err)
	}
	remain := res.ContentLength
	body := res.Body
	if length == 0 {
		remain = 0
		body.Close()
		body = emptyBody
	}
	return &Reader{
		body:        body,
		size:        cl,
		remain:      remain,
		contentType: res.Header.Get("Content-Type"),
	}, nil
}

var emptyBody = ioutil.NopCloser(strings.NewReader(""))

// NewWriter returns a storage Writer that writes to the GCS object
// associated with this ObjectHandle.
//
// A new object will be created if an object with this name already exists.
// Otherwise any previous object with the same name will be replaced.
// The object will not be available (and any previous object will remain)
// until Close has been called.
//
// Attributes can be set on the object by modifying the returned Writer's
// ObjectAttrs field before the first call to Write. If no ContentType
// attribute is specified, the content type will be automatically sniffed
// using net/http.DetectContentType.
//
// It is the caller's responsibility to call Close when writing is done.
func (o *ObjectHandle) NewWriter(ctx context.Context) *Writer {
	return &Writer{
		ctx:         ctx,
		o:           o,
		donec:       make(chan struct{}),
		ObjectAttrs: ObjectAttrs{Name: o.object},
	}
}

// parseKey converts the binary contents of a private key file
// to an *rsa.PrivateKey. It detects whether the private key is in a
// PEM container or not. If so, it extracts the the private key
// from PEM container before conversion. It only supports PEM
// containers with no passphrase.
func parseKey(key []byte) (*rsa.PrivateKey, error) {
	if block, _ := pem.Decode(key); block != nil {
		key = block.Bytes
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(key)
	if err != nil {
		parsedKey, err = x509.ParsePKCS1PrivateKey(key)
		if err != nil {
			return nil, err
		}
	}
	parsed, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("oauth2: private key is invalid")
	}
	return parsed, nil
}

// BucketAttrs represents the metadata for a Google Cloud Storage bucket.
type BucketAttrs struct {
	// Name is the name of the bucket.
	Name string

	// ACL is the list of access control rules on the bucket.
	ACL []ACLRule

	// DefaultObjectACL is the list of access controls to
	// apply to new objects when no object ACL is provided.
	DefaultObjectACL []ACLRule

	// Location is the location of the bucket. It defaults to "US".
	Location string

	// MetaGeneration is the metadata generation of the bucket.
	MetaGeneration int64

	// StorageClass is the storage class of the bucket. This defines
	// how objects in the bucket are stored and determines the SLA
	// and the cost of storage. Typical values are "STANDARD" and
	// "DURABLE_REDUCED_AVAILABILITY". Defaults to "STANDARD".
	StorageClass string

	// Created is the creation time of the bucket.
	Created time.Time
}

func newBucket(b *raw.Bucket) *BucketAttrs {
	if b == nil {
		return nil
	}
	bucket := &BucketAttrs{
		Name:           b.Name,
		Location:       b.Location,
		MetaGeneration: b.Metageneration,
		StorageClass:   b.StorageClass,
		Created:        convertTime(b.TimeCreated),
	}
	acl := make([]ACLRule, len(b.Acl))
	for i, rule := range b.Acl {
		acl[i] = ACLRule{
			Entity: ACLEntity(rule.Entity),
			Role:   ACLRole(rule.Role),
		}
	}
	bucket.ACL = acl
	objACL := make([]ACLRule, len(b.DefaultObjectAcl))
	for i, rule := range b.DefaultObjectAcl {
		objACL[i] = ACLRule{
			Entity: ACLEntity(rule.Entity),
			Role:   ACLRole(rule.Role),
		}
	}
	bucket.DefaultObjectACL = objACL
	return bucket
}

func toRawObjectACL(oldACL []ACLRule) []*raw.ObjectAccessControl {
	var acl []*raw.ObjectAccessControl
	if len(oldACL) > 0 {
		acl = make([]*raw.ObjectAccessControl, len(oldACL))
		for i, rule := range oldACL {
			acl[i] = &raw.ObjectAccessControl{
				Entity: string(rule.Entity),
				Role:   string(rule.Role),
			}
		}
	}
	return acl
}

// toRawBucket copies the editable attribute from b to the raw library's Bucket type.
func (b *BucketAttrs) toRawBucket() *raw.Bucket {
	var acl []*raw.BucketAccessControl
	if len(b.ACL) > 0 {
		acl = make([]*raw.BucketAccessControl, len(b.ACL))
		for i, rule := range b.ACL {
			acl[i] = &raw.BucketAccessControl{
				Entity: string(rule.Entity),
				Role:   string(rule.Role),
			}
		}
	}
	dACL := toRawObjectACL(b.DefaultObjectACL)
	return &raw.Bucket{
		Name:             b.Name,
		DefaultObjectAcl: dACL,
		Location:         b.Location,
		StorageClass:     b.StorageClass,
		Acl:              acl,
	}
}

// toRawObject copies the editable attributes from o to the raw library's Object type.
func (o ObjectAttrs) toRawObject(bucket string) *raw.Object {
	acl := toRawObjectACL(o.ACL)
	return &raw.Object{
		Bucket:             bucket,
		Name:               o.Name,
		ContentType:        o.ContentType,
		ContentEncoding:    o.ContentEncoding,
		ContentLanguage:    o.ContentLanguage,
		CacheControl:       o.CacheControl,
		ContentDisposition: o.ContentDisposition,
		Acl:                acl,
		Metadata:           o.Metadata,
	}
}

// ObjectAttrs represents the metadata for a Google Cloud Storage (GCS) object.
type ObjectAttrs struct {
	// Bucket is the name of the bucket containing this GCS object.
	// This field is read-only.
	Bucket string

	// Name is the name of the object within the bucket.
	// This field is read-only.
	Name string

	// ContentType is the MIME type of the object's content.
	ContentType string

	// ContentLanguage is the content language of the object's content.
	ContentLanguage string

	// CacheControl is the Cache-Control header to be sent in the response
	// headers when serving the object data.
	CacheControl string

	// ACL is the list of access control rules for the object.
	ACL []ACLRule

	// Owner is the owner of the object. This field is read-only.
	//
	// If non-zero, it is in the form of "user-<userId>".
	Owner string

	// Size is the length of the object's content. This field is read-only.
	Size int64

	// ContentEncoding is the encoding of the object's content.
	ContentEncoding string

	// ContentDisposition is the optional Content-Disposition header of the object
	// sent in the response headers.
	ContentDisposition string

	// MD5 is the MD5 hash of the object's content. This field is read-only.
	MD5 []byte

	// CRC32C is the CRC32 checksum of the object's content using
	// the Castagnoli93 polynomial. This field is read-only.
	CRC32C uint32

	// MediaLink is an URL to the object's content. This field is read-only.
	MediaLink string

	// Metadata represents user-provided metadata, in key/value pairs.
	// It can be nil if no metadata is provided.
	Metadata map[string]string

	// Generation is the generation number of the object's content.
	// This field is read-only.
	Generation int64

	// MetaGeneration is the version of the metadata for this
	// object at this generation. This field is used for preconditions
	// and for detecting changes in metadata. A metageneration number
	// is only meaningful in the context of a particular generation
	// of a particular object. This field is read-only.
	MetaGeneration int64

	// StorageClass is the storage class of the bucket.
	// This value defines how objects in the bucket are stored and
	// determines the SLA and the cost of storage. Typical values are
	// "STANDARD" and "DURABLE_REDUCED_AVAILABILITY".
	// It defaults to "STANDARD". This field is read-only.
	StorageClass string

	// Created is the time the object was created. This field is read-only.
	Created time.Time

	// Deleted is the time the object was deleted.
	// If not deleted, it is the zero value. This field is read-only.
	Deleted time.Time

	// Updated is the creation or modification time of the object.
	// For buckets with versioning enabled, changing an object's
	// metadata does not change this property. This field is read-only.
	Updated time.Time
}

// convertTime converts a time in RFC3339 format to time.Time.
// If any error occurs in parsing, the zero-value time.Time is silently returned.
func convertTime(t string) time.Time {
	var r time.Time
	if t != "" {
		r, _ = time.Parse(time.RFC3339, t)
	}
	return r
}

func newObject(o *raw.Object) *ObjectAttrs {
	if o == nil {
		return nil
	}
	acl := make([]ACLRule, len(o.Acl))
	for i, rule := range o.Acl {
		acl[i] = ACLRule{
			Entity: ACLEntity(rule.Entity),
			Role:   ACLRole(rule.Role),
		}
	}
	owner := ""
	if o.Owner != nil {
		owner = o.Owner.Entity
	}
	md5, _ := base64.StdEncoding.DecodeString(o.Md5Hash)
	var crc32c uint32
	d, err := base64.StdEncoding.DecodeString(o.Crc32c)
	if err == nil && len(d) == 4 {
		crc32c = uint32(d[0])<<24 + uint32(d[1])<<16 + uint32(d[2])<<8 + uint32(d[3])
	}
	return &ObjectAttrs{
		Bucket:          o.Bucket,
		Name:            o.Name,
		ContentType:     o.ContentType,
		ContentLanguage: o.ContentLanguage,
		CacheControl:    o.CacheControl,
		ACL:             acl,
		Owner:           owner,
		ContentEncoding: o.ContentEncoding,
		Size:            int64(o.Size),
		MD5:             md5,
		CRC32C:          crc32c,
		MediaLink:       o.MediaLink,
		Metadata:        o.Metadata,
		Generation:      o.Generation,
		MetaGeneration:  o.Metageneration,
		StorageClass:    o.StorageClass,
		Created:         convertTime(o.TimeCreated),
		Deleted:         convertTime(o.TimeDeleted),
		Updated:         convertTime(o.Updated),
	}
}

// Query represents a query to filter objects from a bucket.
type Query struct {
	// Delimiter returns results in a directory-like fashion.
	// Results will contain only objects whose names, aside from the
	// prefix, do not contain delimiter. Objects whose names,
	// aside from the prefix, contain delimiter will have their name,
	// truncated after the delimiter, returned in prefixes.
	// Duplicate prefixes are omitted.
	// Optional.
	Delimiter string

	// Prefix is the prefix filter to query objects
	// whose names begin with this prefix.
	// Optional.
	Prefix string

	// Versions indicates whether multiple versions of the same
	// object will be included in the results.
	Versions bool

	// Cursor is a previously-returned page token
	// representing part of the larger set of results to view.
	// Optional.
	Cursor string

	// MaxResults is the maximum number of items plus prefixes
	// to return. As duplicate prefixes are omitted,
	// fewer total results may be returned than requested.
	// The default page limit is used if it is negative or zero.
	//
	// Deprecated. Use ObjectIterator.SetPageSize.
	MaxResults int
}

// ObjectList represents a list of objects returned from a bucket List call.
type ObjectList struct {
	// Results represent a list of object results.
	Results []*ObjectAttrs

	// Next is the continuation query to retrieve more
	// results with the same filtering criteria. If there
	// are no more results to retrieve, it is nil.
	Next *Query

	// Prefixes represents prefixes of objects
	// matching-but-not-listed up to and including
	// the requested delimiter.
	Prefixes []string
}

// contentTyper implements ContentTyper to enable an
// io.ReadCloser to specify its MIME type.
type contentTyper struct {
	io.Reader
	t string
}

func (c *contentTyper) ContentType() string {
	return c.t
}

// A Condition constrains methods to act on specific generations of
// resources.
//
// Not all conditions or combinations of conditions are applicable to
// all methods.
type Condition interface {
	// method is the high-level ObjectHandle method name, for
	// error messages.  call is the call object to modify.
	modifyCall(method string, call interface{}) error
}

// applyConds modifies the provided call using the conditions in conds.
// call is something that quacks like a *raw.WhateverCall.
func applyConds(method string, conds []Condition, call interface{}) error {
	for _, cond := range conds {
		if err := cond.modifyCall(method, call); err != nil {
			return err
		}
	}
	return nil
}

// toSourceConds returns a slice of Conditions derived from Conds that instead
// function on the equivalent Source methods of a call.
func toSourceConds(conds []Condition) []Condition {
	out := make([]Condition, 0, len(conds))
	for _, c := range conds {
		switch c := c.(type) {
		case genCond:
			var m string
			if strings.HasPrefix(c.method, "If") {
				m = "IfSource" + c.method[2:]
			} else {
				m = "Source" + c.method
			}
			out = append(out, genCond{method: m, val: c.val})
		default:
			// NOTE(djd): If the message from unsupportedCond becomes
			// confusing, we'll need to find a way for Conditions to
			// identify themselves.
			out = append(out, unsupportedCond{})
		}
	}
	return out
}

func Generation(gen int64) Condition               { return genCond{"Generation", gen} }
func IfGenerationMatch(gen int64) Condition        { return genCond{"IfGenerationMatch", gen} }
func IfGenerationNotMatch(gen int64) Condition     { return genCond{"IfGenerationNotMatch", gen} }
func IfMetaGenerationMatch(gen int64) Condition    { return genCond{"IfMetagenerationMatch", gen} }
func IfMetaGenerationNotMatch(gen int64) Condition { return genCond{"IfMetagenerationNotMatch", gen} }

type genCond struct {
	method string
	val    int64
}

func (g genCond) modifyCall(srcMethod string, call interface{}) error {
	rv := reflect.ValueOf(call)
	meth := rv.MethodByName(g.method)
	if !meth.IsValid() {
		return fmt.Errorf("%s: condition %s not supported", srcMethod, g.method)
	}
	meth.Call([]reflect.Value{reflect.ValueOf(g.val)})
	return nil
}

type unsupportedCond struct{}

func (unsupportedCond) modifyCall(srcMethod string, call interface{}) error {
	return fmt.Errorf("%s: condition not supported", srcMethod)
}

func appendParam(req *http.Request, k, v string) {
	sep := ""
	if req.URL.RawQuery != "" {
		sep = "&"
	}
	req.URL.RawQuery += sep + url.QueryEscape(k) + "=" + url.QueryEscape(v)
}

// objectsGetCall wraps an *http.Request for an object fetch call, but adds the methods
// that modifyCall searches for by name. (the same names as the raw, auto-generated API)
type objectsGetCall struct{ req *http.Request }

func (c objectsGetCall) Generation(gen int64) {
	appendParam(c.req, "generation", fmt.Sprint(gen))
}
func (c objectsGetCall) IfGenerationMatch(gen int64) {
	appendParam(c.req, "ifGenerationMatch", fmt.Sprint(gen))
}
func (c objectsGetCall) IfGenerationNotMatch(gen int64) {
	appendParam(c.req, "ifGenerationNotMatch", fmt.Sprint(gen))
}
func (c objectsGetCall) IfMetagenerationMatch(gen int64) {
	appendParam(c.req, "ifMetagenerationMatch", fmt.Sprint(gen))
}
func (c objectsGetCall) IfMetagenerationNotMatch(gen int64) {
	appendParam(c.req, "ifMetagenerationNotMatch", fmt.Sprint(gen))
}

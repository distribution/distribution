// Package swift provides a storagedriver.StorageDriver implementation to
// store blobs in Openstack Swift object storage.
//
// This package leverages the ncw/swift client library for interfacing with
// Swift.
//
// It supports both TempAuth authentication and Keystone authentication
// (up to version 3).
//
// As Swift has a limit on the size of a single uploaded object (by default
// this is 5GB), the driver makes use of the Swift Large Object Support
// (http://docs.openstack.org/developer/swift/overview_large_objects.html).
// Only one container is used for both manifests and data objects. Manifests
// are stored in the 'files' pseudo directory, data objects are stored under
// 'segments'.
package swift

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/ncw/swift"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
	"github.com/docker/distribution/version"
)

const driverName = "swift"

// defaultChunkSize defines the default size of a segment
const defaultChunkSize = 20 * 1024 * 1024

// minChunkSize defines the minimum size of a segment
const minChunkSize = 1 << 20

// contentType defines the Content-Type header associated with stored segments
const contentType = "application/octet-stream"

// readAfterWriteTimeout defines the time we wait before an object appears after having been uploaded
var readAfterWriteTimeout = 15 * time.Second

// readAfterWriteWait defines the time to sleep between two retries
var readAfterWriteWait = 200 * time.Millisecond

// Parameters A struct that encapsulates all of the driver parameters after all values have been set
type Parameters struct {
	Username            string
	Password            string
	AuthURL             string
	Tenant              string
	TenantID            string
	Domain              string
	DomainID            string
	TrustID             string
	Region              string
	Container           string
	Prefix              string
	InsecureSkipVerify  bool
	ChunkSize           int
	SecretKey           string
	AccessKey           string
	TempURLContainerKey bool
	TempURLMethods      []string
}

// swiftInfo maps the JSON structure returned by Swift /info endpoint
type swiftInfo struct {
	Swift struct {
		Version string `mapstructure:"version"`
	}
	Tempurl struct {
		Methods []string `mapstructure:"methods"`
	}
}

func init() {
	factory.Register(driverName, &swiftDriverFactory{})
}

// swiftDriverFactory implements the factory.StorageDriverFactory interface
type swiftDriverFactory struct{}

func (factory *swiftDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	Conn                swift.Connection
	Container           string
	Prefix              string
	BulkDeleteSupport   bool
	ChunkSize           int
	SecretKey           string
	AccessKey           string
	TempURLContainerKey bool
	TempURLMethods      []string
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Openstack Swift
// Objects are stored at absolute keys in the provided container.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
// Required parameters:
// - username
// - password
// - authurl
// - container
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	params := Parameters{
		ChunkSize:          defaultChunkSize,
		InsecureSkipVerify: false,
	}

	if err := mapstructure.Decode(parameters, &params); err != nil {
		return nil, err
	}

	if params.Username == "" {
		return nil, fmt.Errorf("No username parameter provided")
	}

	if params.Password == "" {
		return nil, fmt.Errorf("No password parameter provided")
	}

	if params.AuthURL == "" {
		return nil, fmt.Errorf("No authurl parameter provided")
	}

	if params.Container == "" {
		return nil, fmt.Errorf("No container parameter provided")
	}

	if params.ChunkSize < minChunkSize {
		return nil, fmt.Errorf("The chunksize %#v parameter should be a number that is larger than or equal to %d", params.ChunkSize, minChunkSize)
	}

	return New(params)
}

// New constructs a new Driver with the given Openstack Swift credentials and container name
func New(params Parameters) (*Driver, error) {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		MaxIdleConnsPerHost: 2048,
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: params.InsecureSkipVerify},
	}

	ct := swift.Connection{
		UserName:       params.Username,
		ApiKey:         params.Password,
		AuthUrl:        params.AuthURL,
		Region:         params.Region,
		UserAgent:      "distribution/" + version.Version,
		Tenant:         params.Tenant,
		TenantId:       params.TenantID,
		Domain:         params.Domain,
		DomainId:       params.DomainID,
		TrustId:        params.TrustID,
		Transport:      transport,
		ConnectTimeout: 60 * time.Second,
		Timeout:        15 * 60 * time.Second,
	}
	err := ct.Authenticate()
	if err != nil {
		return nil, fmt.Errorf("Swift authentication failed: %s", err)
	}

	if _, _, err := ct.Container(params.Container); err == swift.ContainerNotFound {
		if err := ct.ContainerCreate(params.Container, nil); err != nil {
			return nil, fmt.Errorf("Failed to create container %s (%s)", params.Container, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("Failed to retrieve info about container %s (%s)", params.Container, err)
	}

	d := &driver{
		Conn:           ct,
		Container:      params.Container,
		Prefix:         params.Prefix,
		ChunkSize:      params.ChunkSize,
		TempURLMethods: make([]string, 0),
		AccessKey:      params.AccessKey,
	}

	info := swiftInfo{}
	if config, err := d.Conn.QueryInfo(); err == nil {
		_, d.BulkDeleteSupport = config["bulk_delete"]

		if err := mapstructure.Decode(config, &info); err == nil {
			d.TempURLContainerKey = info.Swift.Version >= "2.3.0"
			d.TempURLMethods = info.Tempurl.Methods
		}
	} else {
		d.TempURLContainerKey = params.TempURLContainerKey
		d.TempURLMethods = params.TempURLMethods
	}

	if len(d.TempURLMethods) > 0 {
		secretKey := params.SecretKey
		if secretKey == "" {
			secretKey, _ = generateSecret()
		}

		// Since Swift 2.2.2, we can now set secret keys on containers
		// in addition to the account secret keys. Use them in preference.
		if d.TempURLContainerKey {
			_, containerHeaders, err := d.Conn.Container(d.Container)
			if err != nil {
				return nil, fmt.Errorf("Failed to fetch container info %s (%s)", d.Container, err)
			}

			d.SecretKey = containerHeaders["X-Container-Meta-Temp-Url-Key"]
			if d.SecretKey == "" || (params.SecretKey != "" && d.SecretKey != params.SecretKey) {
				m := swift.Metadata{}
				m["temp-url-key"] = secretKey
				if d.Conn.ContainerUpdate(d.Container, m.ContainerHeaders()); err == nil {
					d.SecretKey = secretKey
				}
			}
		} else {
			// Use the account secret key
			_, accountHeaders, err := d.Conn.Account()
			if err != nil {
				return nil, fmt.Errorf("Failed to fetch account info (%s)", err)
			}

			d.SecretKey = accountHeaders["X-Account-Meta-Temp-Url-Key"]
			if d.SecretKey == "" || (params.SecretKey != "" && d.SecretKey != params.SecretKey) {
				m := swift.Metadata{}
				m["temp-url-key"] = secretKey
				if err := d.Conn.AccountUpdate(m.AccountHeaders()); err == nil {
					d.SecretKey = secretKey
				}
			}
		}
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
	content, err := d.Conn.ObjectGetBytes(d.Container, d.swiftPath(path))
	if err == swift.ObjectNotFound {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	return content, err
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	err := d.Conn.ObjectPutBytes(d.Container, d.swiftPath(path), contents, contentType)
	if err == swift.ObjectNotFound {
		return storagedriver.PathNotFoundError{Path: path}
	}
	return err
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	headers := make(swift.Headers)
	headers["Range"] = "bytes=" + strconv.FormatInt(offset, 10) + "-"

	file, _, err := d.Conn.ObjectOpen(d.Container, d.swiftPath(path), false, headers)
	if err == swift.ObjectNotFound {
		return nil, storagedriver.PathNotFoundError{Path: path}
	}
	if swiftErr, ok := err.(*swift.Error); ok && swiftErr.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return ioutil.NopCloser(bytes.NewReader(nil)), nil
	}
	return file, err
}

// Writer returns a FileWriter which will store the content written to it
// at the location designated by "path" after the call to Commit.
func (d *driver) Writer(ctx context.Context, path string, append bool) (storagedriver.FileWriter, error) {
	var largeObj *largeObject

	if append {
		//continue existing large object
		info, headers, err := d.Conn.Object(d.Container, d.swiftPath(path))
		if err == swift.ObjectNotFound {
			return nil, storagedriver.PathNotFoundError{Path: path}
		} else if err != nil {
			return nil, err
		}

		if largeObj, err = d.parseLargeObject(info, headers); err != nil {
			return nil, err
		}

		//...or convert existing simple object into large object
		if largeObj == nil {
			segmentsPath, err := d.swiftSegmentPath(path)
			if err != nil {
				return nil, err
			}
			largeObj = d.makeLargeObject(d.swiftPath(path), segmentsPath)

			//existing object becomes the first segment
			segmentPath := largeObj.NextSegmentPath()
			if err := d.Conn.ObjectMove(d.Container, largeObj.swiftPath, d.Container, segmentPath); err != nil {
				return nil, err
			}
			info, _, err := d.Conn.Object(d.Container, segmentPath)
			if err != nil {
				return nil, err
			}
			largeObj.segments = []swift.Object{info}
		}
	} else {
		//append = false: create new large object
		segmentsPath, err := d.swiftSegmentPath(path)
		if err != nil {
			return nil, err
		}
		largeObj = d.makeLargeObject(d.swiftPath(path), segmentsPath)
	}

	return d.newWriter(path, largeObj), nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	swiftPath := d.swiftPath(path)
	opts := &swift.ObjectsOpts{
		Prefix:    swiftPath,
		Delimiter: '/',
	}

	objects, err := d.Conn.ObjectsAll(d.Container, opts)
	if err != nil {
		if err == swift.ContainerNotFound {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}

	fi := storagedriver.FileInfoFields{
		Path: strings.TrimPrefix(strings.TrimSuffix(swiftPath, "/"), d.swiftPath("/")),
	}

	for _, obj := range objects {
		if obj.PseudoDirectory && obj.Name == swiftPath+"/" {
			fi.IsDir = true
			return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
		} else if obj.Name == swiftPath {
			// The file exists. But on Swift 1.12, the 'bytes' field is always 0 so
			// we need to do a separate HEAD request.
			break
		}
	}

	//Don't trust an empty `objects` slice. A container listing can be
	//outdated. For files, we can make a HEAD request on the object which
	//reports existence (at least) much more reliably.
	info, _, err := d.Conn.Object(d.Container, swiftPath)
	if err != nil {
		if err == swift.ObjectNotFound {
			return nil, storagedriver.PathNotFoundError{Path: path}
		}
		return nil, err
	}
	fi.IsDir = false
	fi.Size = info.Bytes
	fi.ModTime = info.LastModified
	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	var files []string

	prefix := d.swiftPath(path)
	if prefix != "" {
		prefix += "/"
	}

	opts := &swift.ObjectsOpts{
		Prefix:    prefix,
		Delimiter: '/',
	}

	objects, err := d.Conn.ObjectsAll(d.Container, opts)
	for _, obj := range objects {
		files = append(files, strings.TrimPrefix(strings.TrimSuffix(obj.Name, "/"), d.swiftPath("/")))
	}

	if err == swift.ContainerNotFound || (len(objects) == 0 && path != "/") {
		return files, storagedriver.PathNotFoundError{Path: path}
	}
	return files, err
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	info, headers, err := d.Conn.Object(d.Container, d.swiftPath(sourcePath))
	if err == nil {
		//if the object is a DLO or SLO, copy only the manifest
		largeObj, err := d.parseLargeObject(info, headers)
		if err != nil {
			return err
		}
		if largeObj != nil {
			largeObj.swiftPath = d.swiftPath(destPath)
			if err := largeObj.WriteManifest(); err != nil {
				return err
			}
			err = d.Conn.ObjectDelete(d.Container, d.swiftPath(sourcePath))
		} else {
			err = d.Conn.ObjectMove(d.Container, d.swiftPath(sourcePath), d.Container, d.swiftPath(destPath))
		}
	}
	if err == swift.ObjectNotFound {
		return storagedriver.PathNotFoundError{Path: sourcePath}
	}
	return err
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	opts := swift.ObjectsOpts{
		Prefix: d.swiftPath(path) + "/",
	}

	objects, err := d.Conn.ObjectsAll(d.Container, &opts)
	if err != nil {
		if err == swift.ContainerNotFound {
			return storagedriver.PathNotFoundError{Path: path}
		}
		return err
	}

	for _, obj := range objects {
		if obj.PseudoDirectory {
			continue
		}
		//if obj is a large object, also delete all of its segments
		if info, headers, err := d.Conn.Object(d.Container, obj.Name); err == nil {
			largeObj, err := d.parseLargeObject(info, headers)
			if err != nil {
				return err
			}
			if largeObj != nil {
				objects = append(objects, largeObj.segments...)
			}
		} else {
			if err == swift.ObjectNotFound {
				return storagedriver.PathNotFoundError{Path: obj.Name}
			}
			return err
		}
	}

	if d.BulkDeleteSupport && len(objects) > 0 {
		filenames := make([]string, len(objects))
		for i, obj := range objects {
			filenames[i] = obj.Name
		}
		_, err = d.Conn.BulkDelete(d.Container, filenames)
		// Don't fail on ObjectNotFound because eventual consistency
		// makes this situation normal.
		if err != nil && err != swift.Forbidden && err != swift.ObjectNotFound {
			if err == swift.ContainerNotFound {
				return storagedriver.PathNotFoundError{Path: path}
			}
			return err
		}
	} else {
		for _, obj := range objects {
			if err := d.Conn.ObjectDelete(d.Container, obj.Name); err != nil {
				if err == swift.ObjectNotFound {
					return storagedriver.PathNotFoundError{Path: obj.Name}
				}
				return err
			}
		}
	}

	_, _, err = d.Conn.Object(d.Container, d.swiftPath(path))
	if err == nil {
		if err := d.Conn.ObjectDelete(d.Container, d.swiftPath(path)); err != nil {
			if err == swift.ObjectNotFound {
				return storagedriver.PathNotFoundError{Path: path}
			}
			return err
		}
	} else if err == swift.ObjectNotFound {
		if len(objects) == 0 {
			return storagedriver.PathNotFoundError{Path: path}
		}
	} else {
		return err
	}
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	if d.SecretKey == "" {
		return "", storagedriver.ErrUnsupportedMethod{}
	}

	methodString := "GET"
	method, ok := options["method"]
	if ok {
		if methodString, ok = method.(string); !ok {
			return "", storagedriver.ErrUnsupportedMethod{}
		}
	}

	if methodString == "HEAD" {
		// A "HEAD" request on a temporary URL is allowed if the
		// signature was generated with "GET", "POST" or "PUT"
		methodString = "GET"
	}

	supported := false
	for _, method := range d.TempURLMethods {
		if method == methodString {
			supported = true
			break
		}
	}

	if !supported {
		return "", storagedriver.ErrUnsupportedMethod{}
	}

	expiresTime := time.Now().Add(20 * time.Minute)
	expires, ok := options["expiry"]
	if ok {
		et, ok := expires.(time.Time)
		if ok {
			expiresTime = et
		}
	}

	tempURL := d.Conn.ObjectTempUrl(d.Container, d.swiftPath(path), d.SecretKey, methodString, expiresTime)

	if d.AccessKey != "" {
		// On HP Cloud, the signature must be in the form of tenant_id:access_key:signature
		url, _ := url.Parse(tempURL)
		query := url.Query()
		query.Set("temp_url_sig", fmt.Sprintf("%s:%s:%s", d.Conn.TenantId, d.AccessKey, query.Get("temp_url_sig")))
		url.RawQuery = query.Encode()
		tempURL = url.String()
	}

	return tempURL, nil
}

func (d *driver) swiftPath(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.Prefix+"/files"+path, "/"), "/")
}

func (d *driver) swiftSegmentPath(path string) (string, error) {
	checksum := sha1.New()
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	path = hex.EncodeToString(checksum.Sum(append([]byte(path), random...)))
	return strings.TrimLeft(strings.TrimRight(d.Prefix+"/segments/"+path[0:3]+"/"+path[3:], "/"), "/"), nil
}

func generateSecret() (string, error) {
	var secretBytes [32]byte
	if _, err := rand.Read(secretBytes[:]); err != nil {
		return "", fmt.Errorf("could not generate random bytes for Swift secret key: %v", err)
	}
	return hex.EncodeToString(secretBytes[:]), nil
}

type writer struct {
	driver    *driver
	path      string
	object    *largeObject
	bw        *bufio.Writer
	closed    bool
	committed bool
	cancelled bool
}

func (d *driver) newWriter(path string, obj *largeObject) storagedriver.FileWriter {
	return &writer{
		driver: d,
		path:   path,
		object: obj,
		bw: bufio.NewWriterSize(&segmentWriter{
			conn:         d.Conn,
			container:    d.Container,
			object:       obj,
			maxChunkSize: d.ChunkSize,
		}, d.ChunkSize),
	}
}

func (w *writer) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("already closed")
	} else if w.committed {
		return 0, fmt.Errorf("already committed")
	} else if w.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}

	return w.bw.Write(p)
}

func (w *writer) Size() int64 {
	return w.object.Size()
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}

	if err := w.bw.Flush(); err != nil {
		return err
	}

	if !w.committed && !w.cancelled {
		if err := w.object.WriteManifest(); err != nil {
			return err
		}
	}
	w.closed = true

	return nil
}

func (w *writer) Cancel() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true
	return w.driver.Delete(context.Background(), w.path)
}

func (w *writer) Commit() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	} else if w.cancelled {
		return fmt.Errorf("already cancelled")
	}

	if err := w.bw.Flush(); err != nil {
		return err
	}

	if err := w.object.WriteManifest(); err != nil {
		return err
	}
	w.committed = true

	return nil
}

type segmentWriter struct {
	conn         swift.Connection
	container    string
	object       *largeObject
	maxChunkSize int
}

func (sw *segmentWriter) Write(p []byte) (int, error) {
	n := 0
	for offset := 0; offset < len(p); offset += sw.maxChunkSize {
		chunkSize := sw.maxChunkSize
		if offset+chunkSize > len(p) {
			chunkSize = len(p) - offset
		}
		segmentPath := sw.object.NextSegmentPath()
		headers, err := sw.conn.ObjectPut(sw.container, segmentPath, bytes.NewReader(p[offset:offset+chunkSize]), false, "", contentType, nil)
		if err != nil {
			return n, err
		}

		n += chunkSize

		//ObjectPut does not construct a swift.Object although the headers
		//contain all the data
		info := swift.Object{
			Name:               segmentPath,
			ContentType:        contentType,
			ServerLastModified: headers["Last-Modified"],
			Hash:               headers["Etag"],
		}
		if value := headers["Content-Length"]; value != "" {
			if info.Bytes, err = strconv.ParseInt(value, 10, 64); err != nil {
				return n, err
			}
		}
		if info.LastModified, err = time.Parse(http.TimeFormat, info.ServerLastModified); err != nil {
			return n, err
		}

		sw.object.segments = append(sw.object.segments, info)
	}

	return n, nil
}

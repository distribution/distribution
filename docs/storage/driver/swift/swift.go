// Package swift provides a storagedriver.StorageDriver implementation to
// store blobs in Openstack Swift object storage.
//
// This package leverages the ncw/swift client library for interfacing with
// Swift.
//
// Because Swift is a key, value store the Stat call does not support last modification
// time for directories (directories are an abstraction for key, value stores)
package swift

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	gopath "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ncw/swift"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const driverName = "swift"

const defaultChunkSize = 5 * 1024 * 1024

//DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	Username  string
	Password  string
	AuthURL   string
	Tenant    string
	Region    string
	Container string
	Prefix    string
	ChunkSize int64
}

type swiftInfo map[string]interface{}

func init() {
	factory.Register(driverName, &swiftDriverFactory{})
}

// swiftDriverFactory implements the factory.StorageDriverFactory interface
type swiftDriverFactory struct{}

func (factory *swiftDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	Conn              swift.Connection
	Container         string
	Prefix            string
	BulkDeleteSupport bool
	ChunkSize         int64
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by Amazon Swift
// Objects are stored at absolute keys in the provided bucket.
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
	username, ok := parameters["username"]
	if !ok || fmt.Sprint(username) == "" {
		return nil, fmt.Errorf("No username parameter provided")
	}
	password, ok := parameters["password"]
	if !ok || fmt.Sprint(password) == "" {
		return nil, fmt.Errorf("No password parameter provided")
	}
	authURL, ok := parameters["authurl"]
	if !ok || fmt.Sprint(authURL) == "" {
		return nil, fmt.Errorf("No container parameter provided")
	}
	container, ok := parameters["container"]
	if !ok || fmt.Sprint(container) == "" {
		return nil, fmt.Errorf("No container parameter provided")
	}
	tenant, ok := parameters["tenant"]
	if !ok {
		tenant = ""
	}
	region, ok := parameters["region"]
	if !ok {
		region = ""
	}
	rootDirectory, ok := parameters["rootdirectory"]
	if !ok {
		rootDirectory = ""
	}
	chunkSize := int64(defaultChunkSize)
	chunkSizeParam, ok := parameters["chunksize"]
	if ok {
		chunkSize, ok = chunkSizeParam.(int64)
		if !ok {
			return nil, fmt.Errorf("The chunksize parameter should be a number")
		}
	}

	params := DriverParameters{
		fmt.Sprint(username),
		fmt.Sprint(password),
		fmt.Sprint(authURL),
		fmt.Sprint(tenant),
		fmt.Sprint(region),
		fmt.Sprint(container),
		fmt.Sprint(rootDirectory),
		chunkSize,
	}

	return New(params)
}

// New constructs a new Driver with the given Openstack Swift credentials and container name
func New(params DriverParameters) (*Driver, error) {
	ct := swift.Connection{
		UserName:       params.Username,
		ApiKey:         params.Password,
		AuthUrl:        params.AuthURL,
		Region:         params.Region,
		UserAgent:      "distribution",
		Tenant:         params.Tenant,
		ConnectTimeout: 60 * time.Second,
		Timeout:        15 * 60 * time.Second,
	}
	err := ct.Authenticate()
	if err != nil {
		return nil, fmt.Errorf("Swift authentication failed: %s", err)
	}

	if err := ct.ContainerCreate(params.Container, nil); err != nil {
		return nil, fmt.Errorf("Failed to create container %s (%s)", params.Container, err)
	}

	if err := ct.ContainerCreate(params.Container+"_segments", nil); err != nil {
		return nil, fmt.Errorf("Failed to create container %s (%s)", params.Container+"_segments", err)
	}

	d := &driver{
		Conn:              ct,
		Container:         params.Container,
		Prefix:            params.Prefix,
		BulkDeleteSupport: detectBulkDelete(params.AuthURL),
		ChunkSize:         params.ChunkSize,
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
	if err != nil {
		return nil, parseError(path, err)
	}
	return content, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, path string, contents []byte) error {
	if dir, err := d.createParentFolder(path); err != nil {
		return parseError(dir, err)
	}
	err := d.Conn.ObjectPutBytes(d.Container, d.swiftPath(path),
		contents, d.getContentType())
	return parseError(path, err)
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) ReadStream(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	headers := make(swift.Headers)
	headers["Range"] = "bytes=" + strconv.FormatInt(offset, 10) + "-"

	file, _, err := d.Conn.ObjectOpen(d.Container, d.swiftPath(path), false, headers)

	if err != nil {
		if swiftErr, ok := err.(*swift.Error); ok && swiftErr.StatusCode == 416 {
			return ioutil.NopCloser(bytes.NewReader(nil)), nil
		}

		return nil, parseError(path, err)
	}

	return file, nil
}

// WriteStream stores the contents of the provided io.Reader at a
// location designated by the given path. The driver will know it has
// received the full contents when the reader returns io.EOF. The number
// of successfully READ bytes will be returned, even if an error is
// returned. May be used to resume writing a stream by providing a nonzero
// offset. Offsets past the current size will write from the position
// beyond the end of the file.
func (d *driver) WriteStream(ctx context.Context, path string, offset int64, reader io.Reader) (int64, error) {
	var (
		segments      []swift.Object
		paddingReader io.Reader
	)

	partNumber := int64(1)
	bytesRead := int64(0)
	currentLength := int64(0)
	zeroBuf := make([]byte, d.ChunkSize)
	segmentsContainer := d.Container + "_segments"
	cursor := int64(0)

	getSegment := func() string {
		return d.swiftPath(path) + "/" + fmt.Sprintf("%016d", partNumber)
	}

	max := func(a int64, b int64) int64 {
		if a > b {
			return a
		}
		return b
	}

	info, _, err := d.Conn.Object(d.Container, d.swiftPath(path))
	if err != nil {
		if swiftErr, ok := err.(*swift.Error); ok {
			if swiftErr.StatusCode == 404 {
				// Create a object manifest
				if dir, err := d.createParentFolder(path); err != nil {
					return bytesRead, parseError(dir, err)
				}
				headers := make(swift.Headers)
				headers["X-Object-Manifest"] = segmentsContainer + "/" + d.swiftPath(path)
				manifest, err := d.Conn.ObjectCreate(d.Container, d.swiftPath(path), false, "",
					d.getContentType(), headers)
				manifest.Close()
				if err != nil {
					return bytesRead, parseError(path, err)
				}
			} else {
				return bytesRead, parseError(path, err)
			}
		} else {
			return bytesRead, parseError(path, err)
		}
	} else {
		// The manifest already exists. Get all the segments
		currentLength = info.Bytes
		headers := make(swift.Headers)
		headers["Content-Type"] = "application/json"
		opts := &swift.ObjectsOpts{Prefix: d.swiftPath(path), Headers: headers}
		segments, err = d.Conn.Objects(d.Container+"_segments", opts)
		if err != nil {
			return bytesRead, parseError(path, err)
		}
	}

	// First, we skip the existing segments that are not modified by this call
	for i := range segments {
		if offset < cursor+segments[i].Bytes {
			break
		}
		cursor += segments[i].Bytes
		partNumber++
	}

	// We reached the end of the file but we haven't reached 'offset' yet
	// Therefore we add blocks of zeros
	if offset >= currentLength {
		for offset-currentLength >= d.ChunkSize {
			// Insert a block a zero
			d.Conn.ObjectPut(segmentsContainer, getSegment(),
				bytes.NewReader(zeroBuf), false, "",
				d.getContentType(), nil)
			currentLength += d.ChunkSize
			partNumber++
		}

		cursor = currentLength
		paddingReader = bytes.NewReader(zeroBuf)
	} else {
		// Offset is inside the current segment : we need to read the
		// data from the beginning of the segment to offset
		paddingReader, _, err = d.Conn.ObjectOpen(segmentsContainer, getSegment(), false, nil)
		if err != nil {
			return bytesRead, parseError(getSegment(), err)
		}
	}

	multi := io.MultiReader(
		io.LimitReader(paddingReader, offset-cursor),
		io.LimitReader(reader, d.ChunkSize-(offset-cursor)),
	)

	for {
		currentSegment, err := d.Conn.ObjectCreate(segmentsContainer, getSegment(), false, "", d.getContentType(), nil)
		if err != nil {
			return bytesRead, parseError(path, err)
		}

		n, err := io.Copy(currentSegment, multi)
		if err != nil {
			return bytesRead, parseError(path, err)
		}

		if n < d.ChunkSize {
			// We wrote all the data
			if cursor+n < currentLength {
				// Copy the end of the chunk
				headers := make(swift.Headers)
				headers["Range"] = "bytes=" + strconv.FormatInt(cursor+n, 10) + "-" + strconv.FormatInt(cursor+d.ChunkSize, 10)
				file, _, err := d.Conn.ObjectOpen(d.Container, d.swiftPath(path), false, headers)
				if err != nil {
					return bytesRead, parseError(path, err)
				}
				io.Copy(currentSegment, file)
				file.Close()
			}
			if n > 0 {
				currentSegment.Close()
				bytesRead += n - max(0, offset-cursor)
			}
			break
		}

		currentSegment.Close()
		bytesRead += n - max(0, offset-cursor)
		multi = io.MultiReader(io.LimitReader(reader, d.ChunkSize))
		cursor += d.ChunkSize
		partNumber++
	}

	return bytesRead, nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, path string) (storagedriver.FileInfo, error) {
	info, _, err := d.Conn.Object(d.Container, d.swiftPath(path))
	if err != nil {
		return nil, parseError(path, err)
	}

	fi := storagedriver.FileInfoFields{
		Path:    path,
		IsDir:   info.ContentType == "application/directory",
		Size:    info.Bytes,
		ModTime: info.LastModified,
	}

	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given path.
func (d *driver) List(ctx context.Context, path string) ([]string, error) {
	prefix := d.swiftPath(path)
	if prefix != "" {
		prefix += "/"
	}

	opts := &swift.ObjectsOpts{
		Path:      prefix,
		Delimiter: '/',
	}

	files, err := d.Conn.ObjectNames(d.Container, opts)
	for index, name := range files {
		files[index] = "/" + strings.TrimSuffix(name, "/")
	}

	return files, parseError(path, err)
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	err := d.Conn.ObjectMove(d.Container, d.swiftPath(sourcePath),
		d.Container, d.swiftPath(destPath))
	if err != nil {
		return parseError(sourcePath, err)
	}

	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, path string) error {
	opts := swift.ObjectsOpts{
		Prefix: d.swiftPath(path),
	}

	objects, err := d.Conn.ObjectNamesAll(d.Container, &opts)
	if err != nil {
		return parseError(path, err)
	}
	if len(objects) == 0 {
		return storagedriver.PathNotFoundError{Path: path}
	}

	for index, name := range objects {
		objects[index] = name[len(d.Prefix):]
	}

	var multiDelete = true
	if d.BulkDeleteSupport {
		_, err := d.Conn.BulkDelete(d.Container, objects)
		multiDelete = err != nil
	}
	if multiDelete {
		for _, name := range objects {
			if _, headers, err := d.Conn.Object(d.Container, name); err == nil {
				manifest, ok := headers["X-Object-Manifest"]
				if ok {
					components := strings.SplitN(manifest, "/", 2)
					segContainer := components[0]
					segments, err := d.Conn.ObjectNamesAll(segContainer, &swift.ObjectsOpts{Prefix: components[1]})
					if err != nil {
						return parseError(name, err)
					}

					for _, s := range segments {
						if err := d.Conn.ObjectDelete(segContainer, s); err != nil {
							return parseError(s, err)
						}
					}
				}
			} else {
				return parseError(name, err)
			}

			if err := d.Conn.ObjectDelete(d.Container, name); err != nil {
				return parseError(name, err)
			}
		}
	}

	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod
}

func (d *driver) swiftPath(path string) string {
	return strings.TrimLeft(strings.TrimRight(d.Prefix, "/")+path, "/")
}

func (d *driver) createParentFolder(path string) (string, error) {
	dir := gopath.Dir(path)
	if dir != "/" {
		_, _, err := d.Conn.Object(d.Container, d.swiftPath(dir))
		if swiftErr, ok := err.(*swift.Error); ok && swiftErr.StatusCode == 404 {
			_, err := d.Conn.ObjectPut(d.Container, d.swiftPath(dir), bytes.NewReader(make([]byte, 0)),
				false, "", "application/directory", nil)
			if err != nil {
				return dir, err
			}
		}
	}

	return dir, nil
}

func (d *driver) getContentType() string {
	return "application/octet-stream"
}

func detectBulkDelete(authURL string) (bulkDelete bool) {
	resp, err := http.Get(filepath.Join(authURL, "..", "..") + "/info")
	if err == nil {
		defer resp.Body.Close()
		decoder := json.NewDecoder(resp.Body)
		var infos swiftInfo
		if decoder.Decode(&infos) == nil {
			_, bulkDelete = infos["bulk_delete"]
		}
	}
	return
}

func parseError(path string, err error) error {
	if swiftErr, ok := err.(*swift.Error); ok && swiftErr.StatusCode == 404 {
		return storagedriver.PathNotFoundError{Path: path}
	}

	return err
}

package estuary

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/base"
	"github.com/distribution/distribution/v3/registry/storage/driver/factory"
)

const (
	driverName            = "estuary"
	defaultRootDirectory  = "/var/lib/registry"
	defaultBaseUrl        = "https://api.estuary.tech"
	defaultShuttleUrl     = "https://shuttle-4.estuary.tech"
	defaultMaxThreads     = uint64(100)
	defaultGatewayPattern = "https://api.estuary.tech/gw/ipfs/%s"

	// minThreads is the minimum value for the maxthreads configuration
	// parameter. If the driver's parameters are less than this we set the parameters to minThreads
	minThreads = uint64(25)
)

// DriverParameters represents all configuration options available for the
// estuary driver
type DriverParameters struct {
	RootDirectory  string
	MaxThreads     uint64
	AuthToken      string
	BaseUrl        string
	ShuttleUrl     string
	GatewayPattern string
}

func init() {
	factory.Register(driverName, &estuaryDriverFactory{})
}

// estuaryDriverFactory implements the factory.StorageDriverFactory interface
type estuaryDriverFactory struct{}

func (factory *estuaryDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	rootDirectory  string
	authToken      string
	baseUrl        string
	gatewayPattern string
	client         *EstuaryClient
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by
// Estuary. All provided paths will be subpaths of the RootDirectory.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
// Optional Parameters:
// - rootdirectory
// - maxthreads
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	params, err := fromParametersImpl(parameters)
	if err != nil || params == nil {
		return nil, err
	}
	return New(*params), nil
}

func fromParametersImpl(parameters map[string]interface{}) (*DriverParameters, error) {
	var (
		err            error
		maxThreads     = defaultMaxThreads
		rootDirectory  = defaultRootDirectory
		authToken      = ""
		baseUrl        = defaultBaseUrl
		shuttleUrl     = defaultShuttleUrl
		gatewayPattern = defaultGatewayPattern
	)

	if parameters != nil {
		if rootDir, ok := parameters["rootdirectory"]; ok {
			rootDirectory = fmt.Sprint(rootDir)
		}

		maxThreads, err = base.GetLimitFromParameter(parameters["maxthreads"], minThreads, defaultMaxThreads)
		if err != nil {
			return nil, fmt.Errorf("maxthreads config error: %s", err.Error())
		}

		if auth, ok := parameters["auth-token"]; ok {
			authToken = fmt.Sprint(auth)
		}

		if url, ok := parameters["url"]; ok {
			baseUrl = fmt.Sprint(url)
		}

		if url, ok := parameters["shuttle-url"]; ok {
			shuttleUrl = fmt.Sprint(url)
		}

		if url, ok := parameters["gateway-pattern"]; ok {
			gatewayPattern = fmt.Sprint(url)
		}
	}

	params := &DriverParameters{
		RootDirectory:  rootDirectory,
		MaxThreads:     maxThreads,
		AuthToken:      authToken,
		BaseUrl:        baseUrl,
		ShuttleUrl:     shuttleUrl,
		GatewayPattern: gatewayPattern,
	}
	return params, nil
}

// New constructs a new Driver with a given rootDirectory
func New(params DriverParameters) *Driver {
	client := NewEstuaryClient(params.BaseUrl, params.ShuttleUrl, params.AuthToken)

	fsDriver := &driver{
		rootDirectory:  params.RootDirectory,
		authToken:      params.AuthToken,
		baseUrl:        params.BaseUrl,
		gatewayPattern: params.GatewayPattern,
		client:         client,
	}

	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: base.NewRegulator(fsDriver, params.MaxThreads),
			},
		},
	}
}

// Implement the storagedriver.StorageDriver interface

func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, contentPath string) ([]byte, error) {
	rc, err := d.Reader(ctx, contentPath, 0)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	p, err := ioutil.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// PutContent stores the []byte content at a location designated by "path".
func (d *driver) PutContent(ctx context.Context, subPath string, contents []byte) error {
	if path.Base(subPath) == "data" {
		// add and register content
		d.registerContent(subPath, contents)
	} else {
		writer, err := d.Writer(ctx, subPath, false)
		if err != nil {
			return err
		}
		defer writer.Close()
		_, err = io.Copy(writer, bytes.NewReader(contents))
		if err != nil {
			writer.Cancel()
			return err
		}
		err = writer.Commit()
		if err != nil {
			writer.Cancel()
			return err
		}
	}
	return nil
}

// Reader retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, inPath string, offset int64) (io.ReadCloser, error) {
	if path.Base(inPath) == "data" {
		cidHash, err := d.getCidHash(inPath)
		if err != nil {
			return nil, err
		}
		url := fmt.Sprintf(d.gatewayPattern, cidHash)
		fmt.Println(url)

		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		// defer resp.Body.Close()
		return resp.Body, nil
	} else {
		file, err := os.OpenFile(d.fullPath(inPath), os.O_RDONLY, 0644)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, storagedriver.PathNotFoundError{Path: inPath}
			}
			return nil, err
		}
		seekPos, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			file.Close()
			return nil, err
		} else if seekPos < offset {
			file.Close()
			return nil, storagedriver.InvalidOffsetError{Path: inPath, Offset: offset}
		}
		return file, nil
	}
}

func (d *driver) Writer(ctx context.Context, subPath string, append bool) (storagedriver.FileWriter, error) {
	fullPath := d.fullPath(subPath)
	parentDir := path.Dir(fullPath)
	if err := os.MkdirAll(parentDir, 0777); err != nil {
		return nil, err
	}

	fp, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}

	var offset int64

	if !append {
		err := fp.Truncate(0)
		if err != nil {
			fp.Close()
			return nil, err
		}
	} else {
		n, err := fp.Seek(0, io.SeekEnd)
		if err != nil {
			fp.Close()
			return nil, err
		}
		offset = n
	}

	return newFileWriter(fp, offset), nil

}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, subPath string) (storagedriver.FileInfo, error) {
	fullPath := d.fullPath(subPath)
	if path.Base(subPath) == "data" && !strings.Contains(subPath, "/_uploads/") {
		cidHash, err := d.getCidHash(subPath)
		if err != nil {
			return nil, storagedriver.PathNotFoundError{Path: subPath}
		}
		contentElement, err := d.client.GetContentByCid(cidHash)
		if err != nil {
			return nil, storagedriver.PathNotFoundError{Path: subPath}
		}
		fi := storagedriver.FileInfoFields{
			Path:    subPath,
			Size:    int64(contentElement.Content.Size),
			ModTime: contentElement.Content.UpdatedAt,
			IsDir:   false,
		}
		return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil

	} else {
		fi, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, storagedriver.PathNotFoundError{Path: subPath}
			}
			return nil, err
		}
		return fileInfo{
			path:     subPath,
			FileInfo: fi,
		}, nil
	}
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (d *driver) List(ctx context.Context, subPath string) ([]string, error) {
	fullPath := d.fullPath(subPath)

	dir, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storagedriver.PathNotFoundError{Path: subPath}
		}
		return nil, err
	}

	defer dir.Close()

	fileNames, err := dir.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(fileNames))
	for _, fileName := range fileNames {
		keys = append(keys, path.Join(subPath, fileName))
	}

	return keys, nil
}

// Move moves an object stored at sourcePath to destPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, sourcePath string, destPath string) error {
	source := d.fullPath(sourcePath)
	contents, err := ioutil.ReadFile(source)
	if err != nil {
		return err
	}
	d.registerContent(destPath, contents)
	// clean up the source file
	err = os.RemoveAll(source)
	if err != nil {
		return err
	}
	return nil
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, subPath string) error {
	fullPath := d.fullPath(subPath)

	_, err := os.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err != nil {
		return storagedriver.PathNotFoundError{Path: subPath}
	}

	err = os.RemoveAll(fullPath)
	return err
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, subPath string, options map[string]interface{}) (string, error) {
	cidHash, err := d.getCidHash(subPath)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf(d.gatewayPattern, cidHash)
	fmt.Println(url)
	return url, nil
}

// Walk traverses a estuary defined within driver, starting
// from the given path, calling f on each file and directory
func (d *driver) Walk(ctx context.Context, path string, f storagedriver.WalkFn) error {
	return storagedriver.WalkFallback(ctx, d, path, f)
}

// fullPath returns the absolute path of a key within the Driver's storage.
func (d *driver) fullPath(subPath string) string {
	return path.Join(d.rootDirectory, subPath)
}

func (d *driver) getCidHash(path string) (string, error) {
	parts := strings.Split(path, "/")
	sha256Digest := parts[len(parts)-2]
	pin, err := d.client.GetContentByName(sha256Digest)
	if err != nil {
		return "", err
	}
	return pin.Pin.Cid, nil
}

func (d *driver) registerContent(path string, contents []byte) (string, error) {
	resp, err := d.client.AddContent(contents)
	if err != nil {
		return "", err
	}
	return resp.Cid, nil
}

type fileInfo struct {
	os.FileInfo
	path string
}

var _ storagedriver.FileInfo = fileInfo{}

// Path provides the full path of the target of this file info.
func (fi fileInfo) Path() string {
	return fi.path
}

// Size returns current length in bytes of the file. The return value can
// be used to write to the end of the file at path. The value is
// meaningless if IsDir returns true.
func (fi fileInfo) Size() int64 {
	if fi.IsDir() {
		return 0
	}

	return fi.FileInfo.Size()
}

// ModTime returns the modification time for the file. For backends that
// don't have a modification time, the creation time should be returned.
func (fi fileInfo) ModTime() time.Time {
	return fi.FileInfo.ModTime()
}

// IsDir returns true if the path is a directory.
func (fi fileInfo) IsDir() bool {
	return fi.FileInfo.IsDir()
}

type fileWriter struct {
	file      *os.File
	size      int64
	bw        *bufio.Writer
	closed    bool
	committed bool
	cancelled bool
}

func newFileWriter(file *os.File, size int64) *fileWriter {
	return &fileWriter{
		file: file,
		size: size,
		bw:   bufio.NewWriter(file),
	}
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	if fw.closed {
		return 0, fmt.Errorf("already closed")
	} else if fw.committed {
		return 0, fmt.Errorf("already committed")
	} else if fw.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}
	n, err := fw.bw.Write(p)
	fw.size += int64(n)
	return n, err
}

func (fw *fileWriter) Size() int64 {
	return fw.size
}

func (fw *fileWriter) Close() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	if err := fw.bw.Flush(); err != nil {
		return err
	}

	if err := fw.file.Sync(); err != nil {
		return err
	}

	if err := fw.file.Close(); err != nil {
		return err
	}
	fw.closed = true
	return nil
}

func (fw *fileWriter) Cancel() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	fw.cancelled = true
	fw.file.Close()
	return os.Remove(fw.file.Name())
}

func (fw *fileWriter) Commit() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	} else if fw.committed {
		return fmt.Errorf("already committed")
	} else if fw.cancelled {
		return fmt.Errorf("already cancelled")
	}

	if err := fw.bw.Flush(); err != nil {
		return err
	}

	if err := fw.file.Sync(); err != nil {
		return err
	}

	fw.committed = true
	return nil
}

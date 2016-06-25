// Package hdfsweb provides a storagedriver.StorageDriver implementation to
// store blobs in Hadoop HDFS cloud storage.
//
// This package leverages the vladimirvivien/gowfs client functions for
// interfacing with Hadoop HDFS.
package hdfsweb

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/user"
	"path"
	"reflect"
	"strconv"
	"time"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/base"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const (
	driverName         = "hdfsweb"
	defaultBlockSize   = int64(64 << 20)
	defaultBufferSize  = int32(4096)
	defaultReplication = int16(1)
)

//DriverParameters A struct that encapsulates all of the driver parameters after all values have been set
type DriverParameters struct {
	NameNodeHost  string
	NameNodePort  string
	RootDirectory string
	UserName      string
	BlockSize     int64
	BufferSize    int32
	Replication   int16
}

func init() {
	factory.Register(driverName, &hdfsDriverFactory{})
}

// hdfsDriverFactory implements the factory.StorageDriverFactory interface
type hdfsDriverFactory struct{}

func (factory *hdfsDriverFactory) Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	return FromParameters(parameters)
}

type driver struct {
	RootDirectory string
	fs            *FileSystem
}

type baseEmbed struct {
	base.Base
}

// Driver is a storagedriver.StorageDriver implementation backed by a local
// hdfs. All provided paths will be subpaths of the RootDirectory.
type Driver struct {
	baseEmbed
}

// FromParameters constructs a new Driver with a given parameters map
// Optional Parameters:
func FromParameters(parameters map[string]interface{}) (*Driver, error) {
	params := DriverParameters{
		BlockSize:   defaultBlockSize,
		BufferSize:  defaultBufferSize,
		Replication: defaultReplication,
	}

	if parameters != nil {
		if v, ok := parameters["namenodehost"]; ok {
			params.NameNodeHost = fmt.Sprint(v)
		} else {
			return nil, fmt.Errorf("parameter doesn't provided")
		}

		if v, ok := parameters["namenodeport"]; ok {
			params.NameNodePort = fmt.Sprint(v)
		} else {
			return nil, fmt.Errorf("parameter doesn't provided")
		}

		if v, ok := parameters["rootdirectory"]; ok {
			params.RootDirectory = fmt.Sprint(v)
		} else {
			return nil, fmt.Errorf("parameter doesn't provided")
		}

		if v, ok := parameters["username"]; ok {
			params.UserName = fmt.Sprint(v)
		} else {
			usr, err := user.Current()
			if err != nil {
				return nil, err
			}
			params.UserName = usr.Username
		}

		if blockSizeParam, ok := parameters["blocksize"]; ok {
			switch v := blockSizeParam.(type) {
			case string:
				vv, err := strconv.ParseInt(v, 0, 64)
				if err != nil {
					return nil, err
				}
				params.BlockSize = vv
			case int64:
				params.BlockSize = v
			case int, uint, int32, uint32, uint64:
				params.BlockSize = reflect.ValueOf(v).Convert(reflect.TypeOf(params.BlockSize)).Int()
			default:
				return nil, fmt.Errorf("invalid valud %#v for blocksize", blockSizeParam)
			}
		}

		if bufferSizeParam, ok := parameters["buffersize"]; ok {
			switch v := bufferSizeParam.(type) {
			case string:
				vv, err := strconv.ParseInt(v, 0, 32)
				if err != nil {
					return nil, err
				}
				params.BufferSize = int32(vv)
			case int32:
				params.BufferSize = v
			case int, int64, uint, uint32, uint64:
				params.BufferSize = int32(reflect.ValueOf(v).Convert(reflect.TypeOf(params.BufferSize)).Int())
			default:
				return nil, fmt.Errorf("invalid valud %#v for buffersize", bufferSizeParam)
			}
		}

		if replicationParam, ok := parameters["replication"]; ok {
			switch v := replicationParam.(type) {
			case string:
				vv, err := strconv.ParseInt(v, 0, 16)
				if err != nil {
					return nil, err
				}
				params.Replication = int16(vv)
			case int16:
				params.Replication = v
			case int, int64, uint, int32, uint32, uint64:
				params.Replication = int16(reflect.ValueOf(v).Convert(reflect.TypeOf(params.Replication)).Int())
			default:
				return nil, fmt.Errorf("invalid valud %#v for replication", replicationParam)
			}
		}

	}
	return New(params)
}

// New constructs a new Driver with given parameters
func New(params DriverParameters) (*Driver, error) {
	return &Driver{
		baseEmbed: baseEmbed{
			Base: base.Base{
				StorageDriver: &driver{
					fs: &FileSystem{
						NameNodeHost: params.NameNodeHost,
						NameNodePort: params.NameNodePort,
						UserName:     params.UserName,
						BlockSize:    params.BlockSize,
						BufferSize:   params.BufferSize,
						Replication:  params.Replication,
						Client:       &http.Client{Transport: &http.Transport{Dial: dialTimeout}},
					},
					RootDirectory: params.RootDirectory,
				},
			},
		},
	}, nil
}

func (d *driver) Name() string {
	return driverName
}

// GetContent retrieves the content stored at "path" as a []byte.
func (d *driver) GetContent(ctx context.Context, subPath string) ([]byte, error) {
	rc, err := d.Reader(ctx, subPath, 0)
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
	hdfsFullPath := d.fullHdfsPath(subPath)
	return d.fs.Create(
		bytes.NewReader(contents),
		hdfsFullPath)
}

// ReadStream retrieves an io.ReadCloser for the content stored at "path" with a
// given byte offset.
func (d *driver) Reader(ctx context.Context, subPath string, offset int64) (io.ReadCloser, error) {
	hdfsFullPath := d.fullHdfsPath(subPath)

	status, err := d.fs.GetFileStatus(hdfsFullPath)
	if err != nil {
		if re, ok := err.(RemoteException); ok && re.Exception == ExceptionFileNotFound {
			return nil, storagedriver.PathNotFoundError{Path: subPath}
		}
		return nil, err
	} else if status.Type != "FILE" {
		return nil, fmt.Errorf("not file type")
	} else if status.Length < offset {
		return nil, storagedriver.InvalidOffsetError{Path: subPath, Offset: offset}
	}

	if status.Length == offset {
		r := bytes.NewReader([]byte{})
		rc := ioutil.NopCloser(r)
		return rc, nil
	}

	reader, err := d.fs.Open(hdfsFullPath, offset, (status.Length - offset))
	if err != nil {
		return nil, err
	}

	return reader, nil
}

// WriteStream stores the contents of the provided io.Reader at a location
// designated by the given path.
func (d *driver) Writer(ctx context.Context, subPath string, append bool) (storagedriver.FileWriter, error) {
	var size int64
	stat, err := d.fs.GetFileStatus(d.fullHdfsPath(subPath))
	if err == nil {
		if append {
			size = stat.Length
		}
	} else if re, ok := err.(RemoteException); ok && re.Exception == ExceptionFileNotFound {
		if append {
			return nil, storagedriver.PathNotFoundError{Path: subPath}
		}
	} else {
		return nil, err
	}
	if !append {
		err = d.fs.Create(bytes.NewReader(make([]byte, 0)), d.fullHdfsPath(subPath))
	}
	return d.newWriter(subPath, size), nil
}

// Stat retrieves the FileInfo for the given path, including the current size
// in bytes and the creation time.
func (d *driver) Stat(ctx context.Context, subPath string) (storagedriver.FileInfo, error) {
	fi := storagedriver.FileInfoFields{
		Path: subPath,
	}
	hdfsFullPath := d.fullHdfsPath(subPath)
	status, err := d.fs.GetFileStatus(hdfsFullPath)
	if err != nil {
		if re, ok := err.(RemoteException); ok && re.Exception == ExceptionFileNotFound {
			return nil, storagedriver.PathNotFoundError{Path: subPath}
		}
		return nil, err
	}
	if status.Type == "FILE" {
		fi.IsDir = false
	} else if status.Type == "DIRECTORY" {
		fi.IsDir = true
	} else {
		return nil, fmt.Errorf("unknown path type")
	}
	fi.Size = status.Length
	fi.ModTime = time.Unix((status.ModificationTime / 1000), 0)
	return storagedriver.FileInfoInternal{FileInfoFields: fi}, nil
}

// List returns a list of the objects that are direct descendants of the given
// path.
func (d *driver) List(ctx context.Context, subPath string) ([]string, error) {
	legalPath := subPath
	if legalPath[len(legalPath)-1] != '/' {
		legalPath += "/"
	}
	hdfsFullPath := d.fullHdfsPath(legalPath)
	list, err := d.fs.ListStatus(hdfsFullPath)
	if err != nil {
		if re, ok := err.(RemoteException); ok && re.Exception == ExceptionFileNotFound {
			return nil, storagedriver.PathNotFoundError{Path: subPath}
		}
		return nil, err
	}

	keys := make([]string, 0, len(list))
	for _, stat := range list {
		keys = append(keys, path.Join(legalPath, stat.PathSuffix))
	}
	return keys, nil
}

// Move moves an object stored at subSrcPath to subDstPath, removing the original
// object.
func (d *driver) Move(ctx context.Context, subSrcPath, subDstPath string) error {
	fullSrcPath := d.fullHdfsPath(subSrcPath)
	fullDstPath := d.fullHdfsPath(subDstPath)

	// check if source path exist
	if _, err := d.fs.GetFileStatus(fullSrcPath); err != nil {
		if re, ok := err.(RemoteException); ok && re.Exception == ExceptionFileNotFound {
			return storagedriver.PathNotFoundError{Path: subSrcPath}
		}
		return err
	}
	// delete destination if exists
	// for loop is for avoiding the clients competitive
	for {
		if _, err := d.fs.GetFileStatus(fullDstPath); err == nil {
			if err := d.Delete(ctx, subDstPath); err != nil && err != ErrBoolean {
				return err
			}
		}
		if err := d.fs.MkDirs(path.Dir(fullDstPath)); err != nil {
			return err
		}
		if err := d.fs.Rename(fullSrcPath, fullDstPath); err == ErrBoolean {
			continue
		} else {
			return err
		}
	}
}

// Delete recursively deletes all objects stored at "path" and its subpaths.
func (d *driver) Delete(ctx context.Context, subPath string) error {
	hdfsFullPath := d.fullHdfsPath(subPath)
	// check if path exist
	if _, err := d.fs.GetFileStatus(hdfsFullPath); err != nil {
		if re, ok := err.(RemoteException); ok && re.Exception == ExceptionFileNotFound {
			return storagedriver.PathNotFoundError{Path: subPath}
		}
		return err
	}
	if err := d.fs.Delete(hdfsFullPath); err != nil {
		return err
	}
	return nil
}

// URLFor returns a URL which may be used to retrieve the content stored at the given path.
// May return an UnsupportedMethodErr in certain StorageDriver implementations.
func (d *driver) URLFor(ctx context.Context, path string, options map[string]interface{}) (string, error) {
	return "", storagedriver.ErrUnsupportedMethod{}
}

func (d *driver) fullHdfsPath(subPath string) string {
	return path.Join(d.RootDirectory, subPath)
}

func checkFileExist(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}

	return false
}

func convIntParameter(param interface{}) (int64, error) {
	var num int64
	switch v := param.(type) {
	case string:
		vv, err := strconv.ParseInt(v, 0, 64)
		if err != nil {
			return 0, fmt.Errorf("parameter must be an integer, %v invalid", param)
		}
		num = vv
	case int64:
		num = v
	case int, uint, int32, uint32, uint64:
		num = reflect.ValueOf(v).Convert(reflect.TypeOf(num)).Int()
	default:
		return 0, fmt.Errorf("invalid valud %#v", param)
	}
	return num, nil
}

func dialTimeout(network, addr string) (net.Conn, error) {
	return net.DialTimeout(network, addr, time.Duration(2*time.Second))
}

type writer struct {
	driver    *driver
	bw        *bufio.Writer
	path      string
	size      int64
	closed    bool
	committed bool
	cancelled bool
}

func (d *driver) newWriter(subPath string, size int64) storagedriver.FileWriter {
	return &writer{
		driver: d,
		size:   size,
		path:   d.fullHdfsPath(subPath),
		bw: bufio.NewWriterSize(&blobWriter{
			fs:   d.fs,
			path: d.fullHdfsPath(subPath),
		}, int(d.fs.BlockSize)),
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

	n, err := w.bw.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *writer) Size() int64 {
	return w.size
}

func (w *writer) Close() error {
	if w.closed {
		return fmt.Errorf("already closed")
	}
	w.closed = true
	return w.bw.Flush()
}

func (w *writer) Cancel() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	}
	w.cancelled = true
	return w.driver.fs.Delete(w.path)
}

func (w *writer) Commit() error {
	if w.closed {
		return fmt.Errorf("already closed")
	} else if w.committed {
		return fmt.Errorf("already committed")
	} else if w.cancelled {
		return fmt.Errorf("already cancelled")
	}
	w.committed = true
	return w.bw.Flush()
}

type blobWriter struct {
	fs   *FileSystem
	path string
}

func (bw *blobWriter) Write(p []byte) (int, error) {
	var err error
	err = bw.fs.Append(bytes.NewReader(p), bw.path)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

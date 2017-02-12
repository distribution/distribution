// Package b2 provides a storagedriver.StorageDriver implementation for saving
// blobs in Backblaze's B2 object store.
//
// This package uses the github.com/kurin/blazer/b2 library.
//
// +build include_b2

package b2

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/kurin/blazer/b2"

	ctx "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const (
	driverName               = "b2"
	uploadSessionContentType = "application/x-docker-upload-session"
)

func init() {
	factory.Register(driverName, b2Factory{})
}

type b2Factory struct{}

func getInt(i interface{}) int {
	switch i := i.(type) {
	case int:
		return i
	case int32:
		return int(i)
	case int64:
		return int(i)
	case uint32:
		return int(i)
	case uint64:
		return int(i)
	case float32:
		return int(i)
	case float64:
		return int(i)
	}
	return 0
}

func getString(i interface{}) string {
	v, ok := i.(string)
	if !ok {
		return ""
	}
	return v
}

func getContext(i interface{}) context.Context {
	v, ok := i.(context.Context)
	if !ok {
		return nil
	}
	return v
}

// Create StorageDriver from parameters
func (b2Factory) Create(p map[string]interface{}) (storagedriver.StorageDriver, error) {
	id := getString(p["id"])
	if id == "" {
		return nil, errors.New("id not provided")
	}
	key := getString(p["key"])
	if key == "" {
		return nil, errors.New("key not provided")
	}
	bName := getString(p["bucket"])
	if bName == "" {
		return nil, errors.New("bucket not provided")
	}
	ctx := getContext(p["context"])
	if ctx == nil {
		return nil, errors.New("context not provided")
	}

	client, err := b2.NewClient(ctx, id, key)
	if err != nil {
		return nil, err
	}
	bucket, err := client.NewBucket(ctx, bName, nil)
	if err != nil {
		return nil, err
	}
	return &driver{
		bucket:    bucket,
		downNum:   getInt(p["concurrentdownloads"]),
		downChunk: getInt(p["downloadchunksize"]),
		upNum:     getInt(p["concurrentuploads"]),
		upChunk:   getInt(p["uploadchunksize"]),
		rootDir:   getString(p["rootdirectory"]),
	}, nil
}

type driver struct {
	bucket *b2.Bucket

	downNum, upNum, downChunk, upChunk int
	rootDir                            string
}

var noGo = storagedriver.ErrUnsupportedMethod{DriverName: driverName}

func (*driver) Name() string { return driverName }

func (d *driver) fullPath(path string) string {
	return strings.TrimPrefix(filepath.Join(d.rootDir, path), "/")
}

func (d *driver) reader(ctx ctx.Context, path string) io.ReadCloser {
	return d.readerOffset(ctx, path, 0)
}

func (d *driver) readerOffset(ctx ctx.Context, path string, off int64) io.ReadCloser {
	r := d.bucket.Object(d.fullPath(path)).NewRangeReader(ctx, off, -1)
	r.ConcurrentDownloads = d.downNum
	r.ChunkSize = d.downChunk
	return r
}

func (d *driver) writer(ctx ctx.Context, path string) io.WriteCloser {
	w := d.bucket.Object(d.fullPath(path)).NewWriter(ctx).WithAttrs(&b2.Attrs{
		ContentType:  uploadSessionContentType,
		LastModified: time.Now(),
	})
	w.ConcurrentUploads = d.upNum
	w.ChunkSize = d.upChunk
	if w.ChunkSize < 1e8 {
		w.ChunkSize = 1e8
	}
	return w
}

func (d *driver) GetContent(ctx ctx.Context, path string) ([]byte, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	r := d.reader(ctx, path)
	defer r.Close()
	b, err := ioutil.ReadAll(r)
	return b, wrapErr(err)
}

func (d *driver) Delete(ctx ctx.Context, path string) error {
	// Path, apparently, can be a "directory".  So first list the path, and if it
	// is an object, delete it.  If it is a directory, delete everything under it.
	c := &b2.Cursor{Prefix: d.fullPath(path), Delimiter: "/"}
	obj, _, err := d.bucket.ListCurrentObjects(ctx, 1, c)
	if err != nil && err != io.EOF {
		return wrapErr(err)
	}
	if len(obj) == 0 {
		return storagedriver.PathNotFoundError{
			Path:       path,
			DriverName: driverName,
		}
	}
	if !strings.HasSuffix(obj[0].Name(), "/") {
		return wrapErr(obj[0].Delete(ctx))
	}
	c.Delimiter = ""
	c.Prefix += "/"
	for {
		objs, nc, err := d.bucket.ListCurrentObjects(ctx, 100, c)
		if err != nil && err != io.EOF {
			return wrapErr(err)
		}
		c = nc
		for _, obj := range objs {
			if !strings.HasSuffix(obj.Name(), "/") {
				if err := obj.Delete(ctx); err != nil {
					return wrapErr(err)
				}
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

func (d *driver) List(ctx ctx.Context, path string) ([]string, error) {
	var resp []string
	// Docker passes paths of the form "/foo/bar", with no trailing slash, but B2
	// will only return a subdir listing if the request ends with a slash.  We
	// save a round trip here by assuming that every argument to this function
	// actually is a directory, and append a slash unconditionally.
	c := &b2.Cursor{
		Delimiter: "/",
		Prefix:    filepath.Clean(d.fullPath(path)) + "/",
	}
	// B2 objects cannot start with /, but Docker paths all do.
	root := strings.TrimPrefix(d.rootDir, "/")
	for {
		objs, nc, err := d.bucket.ListCurrentObjects(ctx, 100, c)
		if err != nil && err != io.EOF {
			return nil, err
		}
		c = nc
		for _, obj := range objs {
			// Remove trailing slashes from object names that correspond to
			// "subdirectories."
			name := strings.TrimSuffix(obj.Name(), "/")
			resp = append(resp, strings.TrimPrefix(name, root))
		}
		if err == io.EOF {
			if len(resp) == 0 {
				return nil, storagedriver.PathNotFoundError{
					Path:       path,
					DriverName: driverName,
				}
			}
			return resp, nil
		}
	}
}

func (d *driver) Move(ctx ctx.Context, src, dst string) error {
	// This is terrible.
	if err := checkPath(dst); err != nil {
		return err
	}
	// Check that src exists.  We can't simply do this by trying to read it,
	// since otherwise we might delete dst erronously.
	if _, err := d.bucket.Object(d.fullPath(src)).Attrs(ctx); err != nil {
		return wrapErr(err)
	}
	d.bucket.Object(d.fullPath(dst)).Delete(ctx)
	r := d.reader(ctx, src)
	w := d.writer(ctx, dst)
	if _, err := io.Copy(w, r); err != nil {
		return wrapErr(err)
	}
	if err := w.Close(); err != nil {
		return wrapErr(err)
	}
	return d.bucket.Object(d.fullPath(src)).Delete(ctx)
}

func (d *driver) PutContent(ctx ctx.Context, path string, data []byte) error {
	if err := checkPath(path); err != nil {
		return err
	}
	// Remove any pre-existing object.
	d.bucket.Object(d.fullPath(path)).Delete(ctx)
	r := bytes.NewReader(data)
	w := d.writer(ctx, path)
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

type fileWriter struct {
	wc io.WriteCloser
	n  int64
}

func (fw *fileWriter) Size() int64   { return fw.n }
func (fw *fileWriter) Commit() error { return nil }
func (fw *fileWriter) Close() error  { return fw.wc.Close() }
func (fw *fileWriter) Cancel() error { return fw.Close() }

func (fw *fileWriter) Write(p []byte) (int, error) {
	n, err := fw.wc.Write(p)
	fw.n += int64(n)
	return n, err
}

func (d *driver) Reader(ctx ctx.Context, path string, off int64) (io.ReadCloser, error) {
	if err := checkPath(path); err != nil {
		return nil, err
	}
	if off < 0 {
		return nil, storagedriver.InvalidOffsetError{
			Path:       path,
			Offset:     off,
			DriverName: driverName,
		}
	}
	return d.readerOffset(ctx, path, off), nil
}

func (d *driver) Writer(ctx ctx.Context, path string, append bool) (storagedriver.FileWriter, error) {
	// There are a few ways to handle the "append" bool, but they're all awful:
	// (a) Download the file, then upload it again and return the writer.
	// (b) Put pieces of the file in B2, and stitch them together with the reader.
	// (c) Return a not-supported error.
	//
	// I'm partial to (c), but that fails tests, so we'll go with (a) until we
	// get an "append is unreasonably slow" bug.
	if err := checkPath(path); err != nil {
		return nil, err
	}
	w := &fileWriter{
		wc: d.writer(ctx, path),
	}
	if append {
		r := d.reader(ctx, path)
		defer r.Close()
		if _, err := io.Copy(w, r); err != nil {
			w.Cancel()
			return nil, wrapErr(err)
		}
		d.bucket.Object(d.fullPath(path)).Delete(ctx)
	}
	return w, nil
}

type info struct {
	path string
	size int64
	mod  time.Time
	dir  bool
}

func (i info) Path() string       { return i.path }
func (i info) Size() int64        { return i.size }
func (i info) ModTime() time.Time { return i.mod }
func (i info) IsDir() bool        { return i.dir }

func (d *driver) Stat(ctx ctx.Context, path string) (storagedriver.FileInfo, error) {
	attrs, err := d.bucket.Object(d.fullPath(path)).Attrs(ctx)
	if b2.IsNotExist(err) {
		// May have been a directory.
		c := &b2.Cursor{
			Delimiter: "/",
			Prefix:    filepath.Clean(d.fullPath(path)) + "/",
		}
		objs, _, err := d.bucket.ListObjects(ctx, 1, c)
		if err != nil && err != io.EOF {
			return nil, wrapErr(err)
		}
		if len(objs) == 0 {
			return nil, storagedriver.PathNotFoundError{
				Path:       path,
				DriverName: driverName,
			}
		}
		return info{
			path: path,
			dir:  true,
		}, nil
	}
	if err != nil {
		return nil, wrapErr(err)
	}
	root := strings.TrimPrefix(d.rootDir, "/")
	return info{
		path: strings.TrimPrefix(attrs.Name, root),
		size: attrs.Size,
		mod:  attrs.LastModified,
	}, nil
}

func (d *driver) URLFor(ctx ctx.Context, path string, m map[string]interface{}) (string, error) {
	return "", noGo
}

func wrapErr(err error) error {
	if b2.IsNotExist(err) {
		path := strings.Split(err.Error(), ":")[0]
		err = storagedriver.PathNotFoundError{
			Path:       path,
			DriverName: driverName,
		}
	}
	return err
}

func checkPath(path string) error {
	// Check for "invalid" paths here.  There's no documentation about what
	// constitutes an invalid path, so I'm extrapolating from the tests.
	//
	// (Why do we care about malformed paths they get cleaned up augh)
	if !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || path == "" || strings.Contains(path, "//") {
		return storagedriver.InvalidPathError{Path: path, DriverName: driverName}
	}
	return nil
}

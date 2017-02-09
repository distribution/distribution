// Package b2 provides a storagedriver.StorageDriver implementation for saving
// blobs in Backblaze's B2 object store.
//
// This package uses the github.com/kurin/blazer/b2 library.
//
// We implement a consistent file system by keeping a pseudo-journal.  The only
// operation that B2 provides which supports atomicity and consistency is
// b2_update_bucket.  We therefore (ab)use the bucket info attribute to record the
// location of an intent file within B2.  This allows us to provide file system
// semantics while protecting us from multiple reader/writer conflicts (or
// inconsistent state from crashes / unclean shutdowns).
//
// As B2 objects are immutable, and append is (apparently) a required feature,
// this package does not have a one-to-one mapping between content and objects,
// and because of this it cannot support StorageDriver.URLFor.
//
// +build include_b2

package b2

import (
	"context"
	"errors"
	"io"

	"github.com/kurin/blazer/b2"

	ctx "github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

const (
	driverName     = "b2"
	dummyProjectID = "<unknown>"

	uploadSessionContentType = "application/x-docker-upload-session"
)

func init() {
	factory.Register(driverName, b2Factory{})
}

type b2Factory struct{}

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
	bucket, err := client.Bucket(ctx, bName)
	if err != nil {
		return nil, err
	}
	return &driver{
		bucket: bucket,
	}, nil
}

type driver struct {
	bucket *b2.Bucket
}

func (*driver) Name() string { return driverName }

func (d *driver) GetContent(ctx ctx.Context, path string) ([]byte, error) {
	return nil, nil
}

func (d *driver) Delete(ctx ctx.Context, path string) error {
	return nil
}

func (d *driver) List(ctx ctx.Context, path string) ([]string, error) {
	return nil, nil
}

func (d *driver) Move(ctx ctx.Context, src, dst string) error {
	return nil
}

func (d *driver) PutContent(ctx ctx.Context, path string, data []byte) error {
	return nil
}

func (d *driver) Reader(ctx ctx.Context, path string, off int64) (io.ReadCloser, error) {
	return nil, nil
}

func (d *driver) Writer(ctx ctx.Context, path string, append bool) (storagedriver.FileWriter, error) {
	return nil, nil
}

func (d *driver) Stat(ctx ctx.Context, path string) (storagedriver.FileInfo, error) {
	return nil, nil
}

func (d *driver) URLFor(ctx ctx.Context, path string, m map[string]interface{}) (string, error) {
	return "", nil
}

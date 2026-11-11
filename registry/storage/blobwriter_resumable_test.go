//go:build !noresumabledigest

package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
)

type checkpointDriver struct {
	storagedriver.StorageDriver
	writes int
	err    error
}

func (d *checkpointDriver) PutContent(ctx context.Context, path string, content []byte) error {
	if strings.Contains(path, "/hashstates/") {
		d.writes++
		if d.err != nil {
			return d.err
		}
	}
	return d.StorageDriver.PutContent(ctx, path, content)
}

type closeRetryWriter struct {
	storagedriver.FileWriter
	closes int
	err    error
}

func (w *closeRetryWriter) Close() error {
	w.closes++
	if w.err != nil {
		return w.err
	}
	return w.FileWriter.Close()
}

func TestBlobWriterClose(t *testing.T) {
	checkpointErr := errors.New("checkpoint failed")
	closeErr := errors.New("driver close failed")
	for _, tc := range []struct {
		name          string
		disabled      bool
		checkpointErr error
		closeErr      error
	}{
		{name: "success"},
		{name: "disabled digests", disabled: true},
		{name: "checkpoint retry", checkpointErr: checkpointErr},
		{name: "driver retry", closeErr: closeErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			driver := &checkpointDriver{StorageDriver: inmemory.New()}
			registry, err := NewRegistry(ctx, driver)
			if err != nil {
				t.Fatal(err)
			}
			name, _ := reference.WithName("foo/bar")
			repository, err := registry.Repository(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			blobs := repository.Blobs(ctx)
			upload, err := blobs.Create(ctx)
			if err != nil {
				t.Fatal(err)
			}
			bw := upload.(*blobWriter)
			bw.resumableDigestEnabled = !tc.disabled
			writer := &closeRetryWriter{FileWriter: bw.fileWriter, err: tc.closeErr}
			bw.fileWriter = writer
			if _, err := upload.Write([]byte{1, 2, 3}); err != nil {
				t.Fatal(err)
			}
			driver.err = tc.checkpointErr
			wantErr := tc.checkpointErr
			if wantErr == nil {
				wantErr = tc.closeErr
			}
			if err := upload.Close(); err != wantErr {
				t.Fatalf("first close: got %v, want %v", err, wantErr)
			}
			wantWrites := 1
			if tc.disabled {
				wantWrites = 0
			}
			wantCloses := 1
			if tc.checkpointErr != nil {
				wantCloses = 0
			}
			if writer.closes != wantCloses || driver.writes != wantWrites {
				t.Fatalf("first close: %d driver closes, %d checkpoints", writer.closes, driver.writes)
			}

			if wantErr != nil {
				driver.err, writer.err = nil, nil
				if err := upload.Close(); err != nil {
					t.Fatalf("retry: %v", err)
				}
				wantWrites++
				wantCloses++
			}
			if err := upload.Close(); err == nil || err.Error() != "already closed" {
				t.Fatalf("repeated close: got %v, want driver already-closed error", err)
			}
			if writer.closes != wantCloses+1 || driver.writes != wantWrites {
				t.Fatalf("repeated close: %d driver closes, %d checkpoints; want %d, %d", writer.closes, driver.writes, wantCloses+1, wantWrites)
			}

		})
	}
}

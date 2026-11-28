//go:build !noresumabledigest

package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/filesystem"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
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

func TestBlobWriterCancelAfterCheckpointError(t *testing.T) {
	for _, backend := range []string{"inmemory", "filesystem"} {
		for _, resumed := range []bool{false, true} {
			for _, valid := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/resumed=%t/valid=%t", backend, resumed, valid), func(t *testing.T) {
					ctx := context.Background()
					var native storagedriver.StorageDriver = inmemory.New()
					if backend == "filesystem" {
						native = filesystem.New(filesystem.DriverParameters{RootDirectory: t.TempDir(), MaxThreads: 25})
					}
					cleanupDriver := &uploadCleanupErrorDriver{StorageDriver: native}
					driver := &checkpointDriver{StorageDriver: cleanupDriver}
					reg, err := NewRegistry(ctx, driver)
					if err != nil {
						t.Fatal(err)
					}
					name, _ := reference.WithName("foo/bar")
					repo, err := reg.Repository(ctx, name)
					if err != nil {
						t.Fatal(err)
					}
					bs := repo.Blobs(ctx)
					upload, err := bs.Create(ctx)
					if err != nil {
						t.Fatal(err)
					}
					data := []byte("payload")
					offset := 0
					if resumed {
						offset = 1
						if _, err := upload.Write(data[:offset]); err != nil {
							t.Fatal(err)
						}
						if err := upload.Close(); err != nil {
							t.Fatal(err)
						}
						upload, err = bs.Resume(ctx, upload.ID())
						if err != nil {
							t.Fatal(err)
						}
					}
					defer upload.Close()
					bw := upload.(*blobWriter)
					writer := &closeRetryWriter{FileWriter: bw.fileWriter}
					bw.fileWriter = writer
					if _, err := upload.Write(data[offset:]); err != nil {
						t.Fatal(err)
					}
					driver.err = errors.New("checkpoint failed")
					dgst := digest.FromString("wrong payload")
					if valid {
						dgst = digest.FromBytes(data)
					}
					if _, err := upload.Commit(ctx, v1.Descriptor{Digest: dgst}); (err == nil) != valid {
						t.Fatalf("Commit with valid=%t: %v", valid, err)
					}
					if writer.closes != 1 {
						t.Errorf("driver closes after Commit = %d, want 1", writer.closes)
					}
					if valid {
						cleanupDriver.err = errors.New("backend unavailable after Commit")
					}
					if err := upload.Cancel(ctx); err != nil {
						t.Errorf("Cancel after checkpoint error returned %v", err)
					}
					if err := bw.fileWriter.Close(); err == nil || err.Error() != "already closed" {
						t.Errorf("file writer after Cancel returned %v, want already closed", err)
					}
					if _, err := driver.Stat(ctx, path.Dir(bw.path)); err == nil {
						t.Error("upload directory remains after Cancel")
					} else if _, ok := err.(storagedriver.PathNotFoundError); !ok {
						t.Fatal(err)
					}
					if _, err := bs.Resume(ctx, upload.ID()); err != distribution.ErrBlobUploadUnknown {
						t.Errorf("Resume after Cancel returned %v, want ErrBlobUploadUnknown", err)
					}
					if valid {
						got, err := bs.Get(ctx, dgst)
						if err != nil || !bytes.Equal(got, data) {
							t.Errorf("committed blob after Cancel = %q, %v; want %q", got, err, data)
						}
					}
				})
			}
		}
	}
}

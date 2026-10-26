package storage

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type failingMoveDriver struct {
	storagedriver.StorageDriver
	failures       int
	moveBeforeFail bool
	moves          int
}

func (d *failingMoveDriver) Move(ctx context.Context, sourcePath string, destPath string) error {
	d.moves++
	if d.moves > d.failures {
		return d.StorageDriver.Move(ctx, sourcePath, destPath)
	}
	if d.moveBeforeFail && d.moves == 1 {
		if err := d.StorageDriver.Move(ctx, sourcePath, destPath); err != nil {
			return err
		}
	}
	return errors.New("OperationAborted: A conflicting conditional operation is currently in progress against this resource")
}

func commitWithDriver(t *testing.T, driver storagedriver.StorageDriver) error {
	ctx := context.Background()
	registry, err := NewRegistry(ctx, driver, BlobDescriptorCacheProvider(memory.NewInMemoryBlobDescriptorCacheProvider(memory.UnlimitedSize)), EnableDelete)
	if err != nil {
		t.Fatalf("error creating registry: %v", err)
	}
	name, _ := reference.WithName("foo/bar")
	repository, err := registry.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error getting repo: %v", err)
	}
	rd, dgst, err := testutil.CreateRandomTarFile()
	if err != nil {
		t.Fatalf("error creating random reader: %v", err)
	}
	bs := repository.Blobs(ctx)
	wr, err := bs.Create(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting upload: %v", err)
	}
	if _, err := io.Copy(wr, rd); err != nil {
		t.Fatalf("unexpected error writing blob: %v", err)
	}
	if _, err := wr.Commit(ctx, v1.Descriptor{Digest: dgst}); err != nil {
		return err
	}
	if _, err := bs.Stat(ctx, dgst); err != nil {
		t.Fatalf("blob not found after commit: %v", err)
	}
	return nil
}

func withShortMoveRetryDelays(t *testing.T) {
	saved := moveBlobRetryDelays
	moveBlobRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { moveBlobRetryDelays = saved })
}

func TestMoveBlobRetriesFailedMove(t *testing.T) {
	withShortMoveRetryDelays(t)
	driver := &failingMoveDriver{StorageDriver: inmemory.New(), failures: 2}
	if err := commitWithDriver(t, driver); err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if driver.moves != 3 {
		t.Fatalf("expected 3 moves, got %d", driver.moves)
	}
}

func TestMoveBlobStopsWhenBlobAppears(t *testing.T) {
	withShortMoveRetryDelays(t)
	driver := &failingMoveDriver{StorageDriver: inmemory.New(), failures: 10, moveBeforeFail: true}
	if err := commitWithDriver(t, driver); err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if driver.moves != 1 {
		t.Fatalf("expected 1 move, got %d", driver.moves)
	}
}

func TestMoveBlobGivesUp(t *testing.T) {
	withShortMoveRetryDelays(t)
	driver := &failingMoveDriver{StorageDriver: inmemory.New(), failures: 10}
	if err := commitWithDriver(t, driver); err == nil {
		t.Fatal("expected commit error")
	}
	if driver.moves != len(moveBlobRetryDelays)+1 {
		t.Fatalf("expected %d moves, got %d", len(moveBlobRetryDelays)+1, driver.moves)
	}
}

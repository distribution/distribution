package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/cache/memory"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/distribution/v3/testutil"
	"github.com/distribution/reference"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

type failingMoveDriver struct {
	storagedriver.StorageDriver
	failures int
	winner   bool
	onFail   func()
	moves    int
}

func (d *failingMoveDriver) Move(ctx context.Context, sourcePath string, destPath string) error {
	d.moves++
	if d.moves > d.failures {
		return d.StorageDriver.Move(ctx, sourcePath, destPath)
	}
	if d.winner {
		content, err := d.StorageDriver.GetContent(ctx, sourcePath)
		if err != nil {
			return err
		}
		if err := d.StorageDriver.PutContent(ctx, destPath, content); err != nil {
			return err
		}
	}
	if d.onFail != nil {
		d.onFail()
	}
	return errors.New("OperationAborted: A conflicting conditional operation is currently in progress against this resource")
}

func commitWithDriver(ctx context.Context, t *testing.T, driver storagedriver.StorageDriver) (string, error) {
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
	dataPath, err := pathFor(uploadDataPathSpec{name: name.Name(), id: wr.ID()})
	if err != nil {
		t.Fatalf("unexpected error resolving upload path: %v", err)
	}
	if _, err := wr.Commit(ctx, v1.Descriptor{Digest: dgst}); err != nil {
		return dataPath, err
	}
	if _, err := bs.Stat(ctx, dgst); err != nil {
		t.Fatalf("blob not found after commit: %v", err)
	}
	return dataPath, nil
}

func contextWithLogHook() (context.Context, *test.Hook) {
	logger, hook := test.NewNullLogger()
	return dcontext.WithLogger(context.Background(), logrus.NewEntry(logger)), hook
}

func withShortMoveRetryDelays(t *testing.T) {
	saved := moveBlobRetryDelays
	moveBlobRetryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { moveBlobRetryDelays = saved })
}

func TestMoveBlobRetriesFailedMove(t *testing.T) {
	withShortMoveRetryDelays(t)
	ctx, hook := contextWithLogHook()
	driver := &failingMoveDriver{StorageDriver: inmemory.New(), failures: 2}
	if _, err := commitWithDriver(ctx, t, driver); err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if driver.moves != 3 {
		t.Fatalf("expected 3 moves, got %d", driver.moves)
	}
	entries := hook.AllEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	for _, entry := range entries {
		if entry.Message != "moving blob failed, retrying in 1ms" {
			t.Fatalf("unexpected log message: %q", entry.Message)
		}
		for _, field := range []string{"upload.id", "digest"} {
			if entry.Data[field] == nil {
				t.Fatalf("log entry has no %s field", field)
			}
		}
	}
}

func TestMoveBlobStopsWhenBlobAppears(t *testing.T) {
	withShortMoveRetryDelays(t)
	driver := &failingMoveDriver{StorageDriver: inmemory.New(), failures: 10, winner: true}
	dataPath, err := commitWithDriver(context.Background(), t, driver)
	if err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}
	if driver.moves != 1 {
		t.Fatalf("expected 1 move, got %d", driver.moves)
	}
	if _, err := driver.Stat(context.Background(), dataPath); !errors.As(err, &storagedriver.PathNotFoundError{}) {
		t.Fatalf("expected upload data to be removed, got: %v", err)
	}
}

func TestMoveBlobGivesUp(t *testing.T) {
	withShortMoveRetryDelays(t)
	driver := &failingMoveDriver{StorageDriver: inmemory.New(), failures: 10}
	if _, err := commitWithDriver(context.Background(), t, driver); err == nil {
		t.Fatal("expected commit error")
	}
	if driver.moves != len(moveBlobRetryDelays)+1 {
		t.Fatalf("expected %d moves, got %d", len(moveBlobRetryDelays)+1, driver.moves)
	}
}

func TestMoveBlobStopsWhenContextCancelled(t *testing.T) {
	withShortMoveRetryDelays(t)
	ctx, hook := contextWithLogHook()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	driver := &failingMoveDriver{StorageDriver: inmemory.New(), failures: 10, onFail: cancel}
	if _, err := commitWithDriver(ctx, t, driver); err == nil {
		t.Fatal("expected commit error")
	}
	if driver.moves != 1 {
		t.Fatalf("expected 1 move, got %d", driver.moves)
	}
	for _, entry := range hook.AllEntries() {
		if strings.Contains(entry.Message, "retrying") {
			t.Fatalf("unexpected retry log after cancellation: %q", entry.Message)
		}
	}
}

//go:build !windows

package filesystem

import (
	"context"
	"errors"
	"fmt"
	"os"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

// syncDir fsyncs the given directory so that a preceding rename is durably
// persisted. On POSIX systems the directory entry change is only guaranteed to
// survive a crash once the directory itself has been synced.
func syncDir(dir string) (retErr error) {
	dirF, err := os.Open(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("sync dir: %w", err)
	}
	defer func() {
		if err := dirF.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("failed to close dir: %w", err))
		}
	}()
	if err := dirF.Sync(); err != nil {
		return fmt.Errorf("sync dir: %w", err)
	}
	return nil
}

// rename moves source to dest. On POSIX systems os.Rename atomically replaces
// an existing destination.
func rename(source, dest string) error {
	return os.Rename(source, dest)
}

// replace moves tempPath to subPath to make writer's content available at its
// final location. On POSIX systems a rename works even while writer is still
// open, so it is left open for its deferred Close in PutContent.
func replace(ctx context.Context, d *driver, writer storagedriver.FileWriter, tempPath, subPath string) error {
	return d.Move(ctx, tempPath, subPath)
}

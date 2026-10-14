//go:build !windows

package filesystem

import (
	"errors"
	"fmt"
	"os"
)

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

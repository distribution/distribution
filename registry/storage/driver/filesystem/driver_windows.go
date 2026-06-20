//go:build windows

package filesystem

import "os"

// syncDir is a no-op on Windows. Syncing a directory handle (the POSIX
// durability barrier after a rename) is not a supported operation on Windows;
// os.Open(dir).Sync() returns an error there.
func syncDir(dir string) error {
	return nil
}

// rename moves source to dest. Unlike POSIX, a rename on Windows can fail when
// dest already exists, so we fall back to removing dest and retrying. This
// fallback is not atomic: there is a brief window where dest does not exist. It
// is acceptable here because the registry only renames into content-addressed
// paths whose contents are identical regardless of which writer wins.
func rename(source, dest string) error {
	err := os.Rename(source, dest)
	if err != nil {
		if _, statErr := os.Stat(dest); statErr == nil {
			if removeErr := os.RemoveAll(dest); removeErr == nil {
				err = os.Rename(source, dest)
			}
		}
	}
	return err
}

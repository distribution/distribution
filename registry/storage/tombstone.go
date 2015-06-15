package storage

// Delete in the registry is currently handled by placing tombstone files on the
// filesystem which represent deleted files.
// Any operation which accesses a file should use this type to check the for
// the existence of tombstone files and act accordingly.

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage/driver"
)

// tomb is responsible for managing tombstone files
type tomb struct {
	pm      *pathMapper
	driver  driver.StorageDriver
	enabled bool
}

// tombstoneExists queries existance of a tombstone for the given digest
func (t *tomb) tombstoneExists(ctx context.Context, repositoryName string, digest digest.Digest) (bool, error) {
	if !t.enabled {
		return false, distribution.ErrUnsupported
	}
	tombstone, err := t.pm.path(tombstoneSpec{name: repositoryName, digest: digest})
	if err != nil {
		return false, err
	}

	tombstoneExists, err := exists(ctx, t.driver, tombstone)
	if err != nil {
		return false, err
	}

	return tombstoneExists, nil
}

// putTombstone creates a tombstone for the given digest
func (t *tomb) putTombstone(ctx context.Context, repositoryName string, digest digest.Digest) error {
	if !t.enabled {
		return distribution.ErrUnsupported
	}

	tombstone, err := t.pm.path(tombstoneSpec{name: repositoryName, digest: digest})
	if err != nil {
		return err
	}

	if err := t.driver.PutContent(ctx, tombstone, []byte(digest)); err != nil {
		return err
	}

	return nil
}

// deleteTombstone deletes a tombstone for the given digest
func (t *tomb) deleteTombstone(ctx context.Context, repositoryName string, digest digest.Digest) error {
	if !t.enabled {
		return distribution.ErrUnsupported
	}

	tombstone, err := t.pm.path(tombstoneSpec{name: repositoryName, digest: digest})
	if err != nil {
		return err
	}

	err = t.driver.Delete(ctx, tombstone)
	if err != nil {
		return err
	}

	return nil
}

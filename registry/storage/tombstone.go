package storage

// Functions for manipulating tombstones

import (
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

type tomb struct {
	pm     *pathMapper
	driver storagedriver.StorageDriver
}

func (t *tomb) tombstoneExists(ctx context.Context, repositoryName string, digest digest.Digest) (bool, error) {
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

func (t *tomb) putTombstone(ctx context.Context, repositoryName string, digest digest.Digest) error {
	tombstone, err := t.pm.path(tombstoneSpec{name: repositoryName, digest: digest})
	if err != nil {
		return err
	}

	if err := t.driver.PutContent(ctx, tombstone, []byte(digest)); err != nil {
		return err
	}

	return nil
}

func (t *tomb) deleteTombstone(ctx context.Context, repositoryName string, digest digest.Digest) error {
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

package storage

// Functions for manipulating tombstones

import (
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

type tombstone struct {
	pm     *pathMapper
	driver storagedriver.StorageDriver
}

func (ts *tombstone) tombstoneExists(ctx context.Context, repositoryName string, digest digest.Digest) (bool, error) {
	tombstone, err := ts.pm.path(tombstoneSpec{name: repositoryName, digest: digest})
	if err != nil {
		return false, err
	}

	tombstoneExists, err := exists(ctx, ts.driver, tombstone)
	if err != nil {
		return false, err
	}

	return tombstoneExists, nil
}

func (ts *tombstone) putTombstone(ctx context.Context, repositoryName string, digest digest.Digest) error {
	tombstone, err := ts.pm.path(tombstoneSpec{name: repositoryName, digest: digest})
	if err != nil {
		return err
	}

	if err := ts.driver.PutContent(ctx, tombstone, []byte(digest)); err != nil {
		return err
	}

	return nil
}

func (ts *tombstone) deleteTombstone(ctx context.Context, repositoryName string, digest digest.Digest) error {
	tombstone, err := ts.pm.path(tombstoneSpec{name: repositoryName, digest: digest})
	if err != nil {
		return err
	}

	err = ts.driver.Delete(ctx, tombstone)
	if err != nil {
		return err
	}

	return nil
}

package storage

import (
	"path"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage/driver"
)

// vacuum contains functions for cleaning up repositories and blobs
// These functions will only reliably work on strongly consistent
// storage systems.
// https://en.wikipedia.org/wiki/Consistency_model

// NewVacuum creates a new Vacuum
func NewVacuum(ctx context.Context, driver driver.StorageDriver) Vacuum {
	return Vacuum{
		ctx:    ctx,
		driver: driver,
		pm:     defaultPathMapper,
	}
}

// Vacuum removes content from the filesystem
type Vacuum struct {
	pm     *pathMapper
	driver driver.StorageDriver
	ctx    context.Context
}

// RemoveBlob removes a blob from the filesystem
func (v Vacuum) RemoveBlob(dgst string) error {
	d, err := digest.ParseDigest(dgst)
	if err != nil {
		return err
	}

	blobPath, err := v.pm.path(blobDataPathSpec{digest: d})
	if err != nil {
		return err
	}
	context.GetLogger(v.ctx).Infof("Deleting blob: %s", blobPath)
	err = v.driver.Delete(v.ctx, blobPath)
	if err != nil {
		return err
	}

	return nil
}

// RemoveRepository removes a repository directory from the
// filesystem
func (v Vacuum) RemoveRepository(repoName string) error {
	rootForRepository, err := v.pm.path(repositoriesRootPathSpec{})
	if err != nil {
		return err
	}
	repoDir := path.Join(rootForRepository, repoName)
	context.GetLogger(v.ctx).Infof("Deleting repo: %s", repoDir)
	err = v.driver.Delete(v.ctx, repoDir)
	if err != nil {
		return err
	}

	return nil
}

package storage

import (
	"context"
	"path"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/opencontainers/go-digest"
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
	}
}

// Vacuum removes content from the filesystem
type Vacuum struct {
	driver driver.StorageDriver
	ctx    context.Context
}

// RemoveBlob removes a blob from the filesystem
func (v Vacuum) RemoveBlob(dgst string) error {
	d, err := digest.Parse(dgst)
	if err != nil {
		return err
	}

	blobPath, err := pathFor(blobPathSpec{digest: d})
	if err != nil {
		return err
	}

	dcontext.GetLogger(v.ctx).Infof("Deleting blob: %s", blobPath)

	err = v.driver.Delete(v.ctx, blobPath)
	if err != nil {
		return err
	}

	return nil
}

// RemoveManifest removes a manifest from the filesystem
func (v Vacuum) RemoveManifest(name string, dgst digest.Digest, tags []string) error {
	// remove a tag manifest reference, in case of not found continue to next one
	for _, tag := range tags {

		tagsPath, err := pathFor(manifestTagIndexEntryPathSpec{name: name, revision: dgst, tag: tag})
		if err != nil {
			return err
		}

		_, err = v.driver.Stat(v.ctx, tagsPath)
		if err != nil {
			switch err := err.(type) {
			case driver.PathNotFoundError:
				continue
			default:
				return err
			}
		}
		dcontext.GetLogger(v.ctx).Infof("deleting manifest tag reference: %s", tagsPath)
		err = v.driver.Delete(v.ctx, tagsPath)
		if err != nil {
			return err
		}
	}

	manifestPath, err := pathFor(manifestRevisionPathSpec{name: name, revision: dgst})
	if err != nil {
		return err
	}
	dcontext.GetLogger(v.ctx).Infof("deleting manifest: %s", manifestPath)
	return v.driver.Delete(v.ctx, manifestPath)
}

// RemoveRepository removes a repository directory from the
// filesystem
func (v Vacuum) RemoveRepository(repoName string) error {
	rootForRepository, err := pathFor(repositoriesRootPathSpec{})
	if err != nil {
		return err
	}
	repoManifestDir := path.Join(rootForRepository, repoName, "_manifests")
	dcontext.GetLogger(v.ctx).Infof("Deleting repo: %s", repoManifestDir)
	err = v.driver.Delete(v.ctx, repoManifestDir)
	if err != nil {
		if _, ok := err.(driver.PathNotFoundError); !ok {
			return err
		}
	}
	repoLayerDir := path.Join(rootForRepository, repoName, "_layers")
	dcontext.GetLogger(v.ctx).Infof("Deleting repo: %s", repoLayerDir)
	err = v.driver.Delete(v.ctx, repoLayerDir)
	if err != nil {
		if _, ok := err.(driver.PathNotFoundError); !ok {
			return err
		}
	}

	repoUploadDir := path.Join(rootForRepository, repoName, "_uploads")
	dcontext.GetLogger(v.ctx).Infof("Deleting repo: %s", repoUploadDir)
	err = v.driver.Delete(v.ctx, repoUploadDir)
	if err != nil {
		if _, ok := err.(driver.PathNotFoundError); !ok {
			return err
		}
	}

	return nil
}

// RemoveLayer removes a layer link path from the storage
func (v Vacuum) RemoveLayer(repoName string, dgst digest.Digest) error {
	layerLinkPath, err := pathFor(layerLinkPathSpec{name: repoName, digest: dgst})
	if err != nil {
		return err
	}
	dcontext.GetLogger(v.ctx).Infof("Deleting layer link path: %s", layerLinkPath)
	err = v.driver.Delete(v.ctx, layerLinkPath)
	if err != nil {
		return err
	}

	return nil
}

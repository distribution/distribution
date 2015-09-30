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
	}
}

// Vacuum removes content from the filesystem
type Vacuum struct {
	driver driver.StorageDriver
	ctx    context.Context
}

// RemoveBlob removes a blob from the filesystem. withDirectory
// allows to remove <hex-digest> directory containing the blob as well.
func (v Vacuum) RemoveBlob(dgst string, withDirectory bool) error {
	d, err := digest.ParseDigest(dgst)
	if err != nil {
		return err
	}

	var blobPath string
	if withDirectory {
		blobPath, err = pathFor(blobPathSpec{digest: d})
	} else {
		blobPath, err = pathFor(blobDataPathSpec{digest: d})
	}
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

// UnlinkLayer removes an entry from repository layers. withDirectory
// allows to remove <hex-digest> directory containing the link as well.
func (v Vacuum) UnlinkLayer(repoName string, dgst string, withDirectory bool) error {
	d, err := digest.ParseDigest(dgst)
	if err != nil {
		return err
	}

	layerPath, err := pathFor(layerLinkPathSpec{name: repoName, digest: d})
	if err != nil {
		return err
	}
	if withDirectory {
		layerPath = path.Dir(layerPath)
	}
	context.GetLogger(v.ctx).Infof("Unlinking layer %s", layerPath)
	err = v.driver.Delete(v.ctx, layerPath)
	if err != nil {
		return err
	}

	return nil
}

// UnlinkSignature removes a link from signatures store of manifest revision.
// withDirectory allows to remove <hex-digest> directory containing the link as
// well.
func (v Vacuum) UnlinkSignature(repoName string, dgst string, sdgst string, withDirectory bool) error {
	d, err := digest.ParseDigest(dgst)
	if err != nil {
		return err
	}
	sd, err := digest.ParseDigest(sdgst)
	if err != nil {
		return err
	}

	sigPath, err := pathFor(manifestSignatureLinkPathSpec{
		name:      repoName,
		revision:  d,
		signature: sd,
	})
	if err != nil {
		return err
	}
	if withDirectory {
		sigPath = path.Dir(sigPath)
	}
	context.GetLogger(v.ctx).Infof("Unlinking signature %s", sigPath)
	err = v.driver.Delete(v.ctx, sigPath)
	if err != nil {
		return err
	}

	return nil
}

// UnlinkManifestRevision removes an entry from repository manifest revisions
// together with signatures.
func (v Vacuum) UnlinkManifestRevision(repoName string, dgst string) error {
	d, err := digest.ParseDigest(dgst)
	if err != nil {
		return err
	}

	revPath, err := pathFor(manifestRevisionPathSpec{name: repoName, revision: d})
	if err != nil {
		return err
	}
	context.GetLogger(v.ctx).Infof("Unlinking manifest revision %s", revPath)
	err = v.driver.Delete(v.ctx, revPath)
	if err != nil {
		return err
	}

	return nil
}

// DeleteTag removes a tag from repository with index store from the filesystem.
func (v Vacuum) DeleteTag(repoName, tag string) error {
	tagPath, err := pathFor(manifestTagPathSpec{name: repoName, tag: tag})
	if err != nil {
		return err
	}
	context.GetLogger(v.ctx).Infof("Deleting tag %s:%s", repoName, tag)
	err = v.driver.Delete(v.ctx, tagPath)
	if err != nil {
		return err
	}

	return nil
}

// RemoveRepository removes a repository directory from the
// filesystem
func (v Vacuum) RemoveRepository(repoName string) error {
	rootForRepository, err := pathFor(repositoriesRootPathSpec{})
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

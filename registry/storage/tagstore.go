package storage

import (
	"path"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// tagStore provides methods to manage repository tags in a backend storage driver.
type tagStore struct {
	*repository
}

var _ distribution.TagService = &tagStore{}

func (ts *tagStore) List() ([]string, error) {
	ctxu.GetLogger(ts.ctx).Info("(*tagStore).List")
	p, err := ts.pm.path(manifestTagPathSpec{
		name: ts.name,
	})
	if err != nil {
		return nil, err
	}

	var tags []string
	entries, err := ts.driver.List(p)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return nil, distribution.ErrRepositoryUnknown{Name: ts.name}
		default:
			return nil, err
		}
	}

	for _, entry := range entries {
		_, filename := path.Split(entry)

		tags = append(tags, filename)
	}

	return tags, nil
}

func (ts *tagStore) Exists(tag string) (bool, error) {
	ctxu.GetLogger(ts.repository.ctx).Debug("(*tagStore).Exists")
	tagPath, err := ts.pm.path(manifestTagCurrentPathSpec{
		name: ts.Name(),
		tag:  tag,
	})
	if err != nil {
		return false, err
	}

	exists, err := exists(ts.driver, tagPath)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (ts *tagStore) GetRevision(tag string) (digest.Digest, error) {
	ctxu.GetLogger(ts.repository.ctx).Debug("(*tagStore).GetRevision")
	return ts.resolve(tag)
}

func (ts *tagStore) GetAllRevisions(tag string) ([]digest.Digest, error) {
	ctxu.GetLogger(ts.repository.ctx).Debug("(*tagStore).GetAllRevisions")
	return ts.revisions(tag)
}

// resolve the current revision for name and tag.
func (ts *tagStore) resolve(tag string) (digest.Digest, error) {
	currentPath, err := ts.pm.path(manifestTagCurrentPathSpec{
		name: ts.Name(),
		tag:  tag,
	})

	if err != nil {
		return "", err
	}

	if exists, err := exists(ts.driver, currentPath); err != nil {
		return "", err
	} else if !exists {
		return "", distribution.ErrManifestUnknown{Name: ts.Name(), Tag: tag}
	}

	revision, err := ts.blobStore.readlink(currentPath)
	if err != nil {
		return "", err
	}

	return revision, nil
}

// revisions returns all revisions with the specified name and tag.
func (ts *tagStore) revisions(tag string) ([]digest.Digest, error) {
	manifestTagIndexPath, err := ts.pm.path(manifestTagIndexPathSpec{
		name: ts.Name(),
		tag:  tag,
	})

	if err != nil {
		return nil, err
	}

	// TODO(stevvooe): Need to append digest alg to get listing of revisions.
	manifestTagIndexPath = path.Join(manifestTagIndexPath, "sha256")

	entries, err := ts.driver.List(manifestTagIndexPath)
	if err != nil {
		return nil, err
	}

	var revisions []digest.Digest
	for _, entry := range entries {
		revisions = append(revisions, digest.NewDigestFromHex("sha256", path.Base(entry)))
	}

	return revisions, nil
}

// delete removes the tag from repository, including the history of all
// revisions that have the specified tag.
func (ts *tagStore) delete(tag string) error {
	tagPath, err := ts.pm.path(manifestTagPathSpec{
		name: ts.Name(),
		tag:  tag,
	})
	if err != nil {
		return err
	}

	return ts.driver.Delete(tagPath)
}

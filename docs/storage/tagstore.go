package storage

import (
	"path"

	"github.com/docker/distribution"
	//	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// tagStore provides methods to manage manifest tags in a backend storage driver.
type tagStore struct {
	*repository
}

// tags lists the manifest tags for the specified repository.
func (ts *tagStore) tags() ([]string, error) {
	p, err := ts.pm.path(manifestTagPathSpec{
		name: ts.name,
	})
	if err != nil {
		return nil, err
	}

	var tags []string
	entries, err := ts.driver.List(ts.repository.ctx, p)
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

// exists returns true if the specified manifest tag exists in the repository.
func (ts *tagStore) exists(tag string) (bool, error) {
	tagPath, err := ts.pm.path(manifestTagCurrentPathSpec{
		name: ts.Name(),
		tag:  tag,
	})
	if err != nil {
		return false, err
	}

	exists, err := exists(ts.repository.ctx, ts.driver, tagPath)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// tag tags the digest with the given tag, updating the the store to point at
// the current tag. The digest must point to a manifest.
func (ts *tagStore) tag(tag string, revision digest.Digest) error {
	indexEntryPath, err := ts.pm.path(manifestTagIndexEntryLinkPathSpec{
		name:     ts.Name(),
		tag:      tag,
		revision: revision,
	})

	if err != nil {
		return err
	}

	currentPath, err := ts.pm.path(manifestTagCurrentPathSpec{
		name: ts.Name(),
		tag:  tag,
	})

	if err != nil {
		return err
	}

	// Link into the index
	if err := ts.blobStore.link(indexEntryPath, revision); err != nil {
		return err
	}

	// Overwrite the current link
	return ts.blobStore.link(currentPath, revision)
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

	if exists, err := exists(ts.repository.ctx, ts.driver, currentPath); err != nil {
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

	entries, err := ts.driver.List(ts.repository.ctx, manifestTagIndexPath)
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

	return ts.driver.Delete(ts.repository.ctx, tagPath)
}

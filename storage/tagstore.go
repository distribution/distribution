package storage

import (
	"path"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storagedriver"
)

// tagStore provides methods to manage manifest tags in a backend storage driver.
type tagStore struct {
	driver     storagedriver.StorageDriver
	blobStore  *blobStore
	pathMapper *pathMapper
}

// tags lists the manifest tags for the specified repository.
func (ts *tagStore) tags(name string) ([]string, error) {
	p, err := ts.pathMapper.path(manifestTagPathSpec{
		name: name,
	})
	if err != nil {
		return nil, err
	}

	var tags []string
	entries, err := ts.driver.List(p)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return nil, ErrUnknownRepository{Name: name}
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
func (ts *tagStore) exists(name, tag string) (bool, error) {
	tagPath, err := ts.pathMapper.path(manifestTagCurrentPathSpec{
		name: name,
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

// tag tags the digest with the given tag, updating the the store to point at
// the current tag. The digest must point to a manifest.
func (ts *tagStore) tag(name, tag string, revision digest.Digest) error {
	indexEntryPath, err := ts.pathMapper.path(manifestTagIndexEntryPathSpec{
		name:     name,
		tag:      tag,
		revision: revision,
	})

	if err != nil {
		return err
	}

	currentPath, err := ts.pathMapper.path(manifestTagCurrentPathSpec{
		name: name,
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
func (ts *tagStore) resolve(name, tag string) (digest.Digest, error) {
	currentPath, err := ts.pathMapper.path(manifestTagCurrentPathSpec{
		name: name,
		tag:  tag,
	})

	if err != nil {
		return "", err
	}

	if exists, err := exists(ts.driver, currentPath); err != nil {
		return "", err
	} else if !exists {
		return "", ErrUnknownManifest{Name: name, Tag: tag}
	}

	revision, err := ts.blobStore.readlink(currentPath)
	if err != nil {
		return "", err
	}

	return revision, nil
}

// revisions returns all revisions with the specified name and tag.
func (ts *tagStore) revisions(name, tag string) ([]digest.Digest, error) {
	manifestTagIndexPath, err := ts.pathMapper.path(manifestTagIndexPathSpec{
		name: name,
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
func (ts *tagStore) delete(name, tag string) error {
	tagPath, err := ts.pathMapper.path(manifestTagPathSpec{
		name: name,
		tag:  tag,
	})
	if err != nil {
		return err
	}

	return ts.driver.Delete(tagPath)
}

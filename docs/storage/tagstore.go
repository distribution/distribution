package storage

import (
	"path"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// tagStore provides methods to manage manifest tags in a backend storage driver.
type tagStore struct {
	repository *repository
	blobStore  *blobStore
	ctx        context.Context
}

// tags lists the manifest tags for the specified repository.
func (ts *tagStore) tags() ([]string, error) {
	p, err := ts.blobStore.pm.path(manifestTagPathSpec{
		name: ts.repository.Name(),
	})
	if err != nil {
		return nil, err
	}

	var tags []string
	entries, err := ts.blobStore.driver.List(ts.ctx, p)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return nil, distribution.ErrRepositoryUnknown{Name: ts.repository.Name()}
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
	tagPath, err := ts.blobStore.pm.path(manifestTagCurrentPathSpec{
		name: ts.repository.Name(),
		tag:  tag,
	})
	if err != nil {
		return false, err
	}

	exists, err := exists(ts.ctx, ts.blobStore.driver, tagPath)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// tag tags the digest with the given tag, updating the the store to point at
// the current tag. The digest must point to a manifest.
func (ts *tagStore) tag(tag string, revision digest.Digest) error {
	currentPath, err := ts.blobStore.pm.path(manifestTagCurrentPathSpec{
		name: ts.repository.Name(),
		tag:  tag,
	})

	if err != nil {
		return err
	}

	nbs := ts.linkedBlobStore(ts.ctx, tag)
	// Link into the index
	if err := nbs.linkBlob(ts.ctx, distribution.Descriptor{Digest: revision}); err != nil {
		return err
	}

	// Overwrite the current link
	return ts.blobStore.link(ts.ctx, currentPath, revision)
}

// resolve the current revision for name and tag.
func (ts *tagStore) resolve(tag string) (digest.Digest, error) {
	currentPath, err := ts.blobStore.pm.path(manifestTagCurrentPathSpec{
		name: ts.repository.Name(),
		tag:  tag,
	})
	if err != nil {
		return "", err
	}

	revision, err := ts.blobStore.readlink(ts.ctx, currentPath)
	if err != nil {
		switch err.(type) {
		case storagedriver.PathNotFoundError:
			return "", distribution.ErrManifestUnknown{Name: ts.repository.Name(), Tag: tag}
		}

		return "", err
	}

	return revision, nil
}

// delete removes the tag from repository, including the history of all
// revisions that have the specified tag.
func (ts *tagStore) delete(tag string) error {
	tagPath, err := ts.blobStore.pm.path(manifestTagPathSpec{
		name: ts.repository.Name(),
		tag:  tag,
	})
	if err != nil {
		return err
	}

	return ts.blobStore.driver.Delete(ts.ctx, tagPath)
}

// linkedBlobStore returns the linkedBlobStore for the named tag, allowing one
// to index manifest blobs by tag name. While the tag store doesn't map
// precisely to the linked blob store, using this ensures the links are
// managed via the same code path.
func (ts *tagStore) linkedBlobStore(ctx context.Context, tag string) *linkedBlobStore {
	return &linkedBlobStore{
		blobStore:  ts.blobStore,
		repository: ts.repository,
		ctx:        ctx,
		linkPathFns: []linkPathFunc{func(pm *pathMapper, name string, dgst digest.Digest) (string, error) {
			return pm.path(manifestTagIndexEntryLinkPathSpec{
				name:     name,
				tag:      tag,
				revision: dgst,
			})
		}},
	}
}

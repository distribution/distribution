package storage

import (
	"context"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/opencontainers/go-digest"
	"golang.org/x/sync/errgroup"

	"github.com/distribution/distribution/v3"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
)

var _ distribution.TagService = &tagStore{}

// tagStore provides methods to manage manifest tags in a backend storage driver.
// This implementation uses the same on-disk layout as the (now deleted) tag
// store.  This provides backward compatibility with current registry deployments
// which only makes use of the Digest field of the returned distribution.Descriptor
// but does not enable full roundtripping of Descriptor objects
type tagStore struct {
	repository       *repository
	blobStore        *blobStore
	concurrencyLimit int
}

// All returns all tags
func (ts *tagStore) All(ctx context.Context) ([]string, error) {
	pathSpec, err := pathFor(manifestTagsPathSpec{
		name: ts.repository.Named().Name(),
	})
	if err != nil {
		return nil, err
	}

	entries, err := ts.blobStore.driver.List(ctx, pathSpec)
	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return nil, distribution.ErrRepositoryUnknown{Name: ts.repository.Named().Name()}
		default:
			return nil, err
		}
	}

	tags := make([]string, 0, len(entries))
	for _, entry := range entries {
		_, filename := path.Split(entry)
		tags = append(tags, filename)
	}

	// there is no guarantee for the order,
	// therefore sort before return.
	sort.Strings(tags)

	return tags, nil
}

// Tag tags the digest with the given tag, updating the store to point at
// the current tag. The digest must point to a manifest.
func (ts *tagStore) Tag(ctx context.Context, tag string, desc distribution.Descriptor) error {
	currentPath, err := pathFor(manifestTagCurrentPathSpec{
		name: ts.repository.Named().Name(),
		tag:  tag,
	})
	if err != nil {
		return err
	}

	lbs := ts.linkedBlobStore(ctx, tag)

	// Link into the index
	if err := lbs.linkBlob(ctx, desc); err != nil {
		return err
	}

	// Overwrite the current link
	return ts.blobStore.link(ctx, currentPath, desc.Digest)
}

// resolve the current revision for name and tag.
func (ts *tagStore) Get(ctx context.Context, tag string) (distribution.Descriptor, error) {
	currentPath, err := pathFor(manifestTagCurrentPathSpec{
		name: ts.repository.Named().Name(),
		tag:  tag,
	})
	if err != nil {
		return distribution.Descriptor{}, err
	}

	revision, err := ts.blobStore.readlink(ctx, currentPath)
	if err != nil {
		switch err.(type) {
		case storagedriver.PathNotFoundError:
			return distribution.Descriptor{}, distribution.ErrTagUnknown{Tag: tag}
		}

		return distribution.Descriptor{}, err
	}

	return distribution.Descriptor{Digest: revision}, nil
}

// Untag removes the tag association
func (ts *tagStore) Untag(ctx context.Context, tag string) error {
	tagPath, err := pathFor(manifestTagPathSpec{
		name: ts.repository.Named().Name(),
		tag:  tag,
	})
	if err != nil {
		return err
	}

	return ts.blobStore.driver.Delete(ctx, tagPath)
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
		linkPath: func(name string, dgst digest.Digest) (string, error) {
			return pathFor(manifestTagIndexEntryLinkPathSpec{
				name:     name,
				tag:      tag,
				revision: dgst,
			})
		},
	}
}

// Lookup recovers a list of tags which refer to this digest.  When a manifest is deleted by
// digest, tag entries which point to it need to be recovered to avoid dangling tags.
func (ts *tagStore) Lookup(ctx context.Context, desc distribution.Descriptor) ([]string, error) {
	allTags, err := ts.All(ctx)
	switch err.(type) {
	case distribution.ErrRepositoryUnknown:
		// This tag store has been initialized but not yet populated
		break
	case nil:
		break
	default:
		return nil, err
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(ts.concurrencyLimit)

	var (
		tags []string
		mu   sync.Mutex
	)
	for _, tag := range allTags {
		if ctx.Err() != nil {
			break
		}
		tag := tag

		g.Go(func() error {
			tagLinkPathSpec := manifestTagCurrentPathSpec{
				name: ts.repository.Named().Name(),
				tag:  tag,
			}

			tagLinkPath, _ := pathFor(tagLinkPathSpec)
			tagDigest, err := ts.blobStore.readlink(ctx, tagLinkPath)
			if err != nil {
				switch err.(type) {
				case storagedriver.PathNotFoundError:
					return nil
				}
				return err
			}

			if tagDigest == desc.Digest {
				mu.Lock()
				tags = append(tags, tag)
				mu.Unlock()
			}

			return nil
		})
	}

	err = g.Wait()
	if err != nil {
		return nil, err
	}

	return tags, nil
}

func (ts *tagStore) ManifestDigests(ctx context.Context, tag string) ([]digest.Digest, error) {
	tagLinkPath := func(name string, dgst digest.Digest) (string, error) {
		return pathFor(manifestTagIndexEntryLinkPathSpec{
			name:     name,
			tag:      tag,
			revision: dgst,
		})
	}
	lbs := &linkedBlobStore{
		blobStore: ts.blobStore,
		blobAccessController: &linkedBlobStatter{
			blobStore:  ts.blobStore,
			repository: ts.repository,
			linkPath:   manifestRevisionLinkPath,
		},
		repository: ts.repository,
		ctx:        ctx,
		linkPath:   tagLinkPath,
		linkDirectoryPathSpec: manifestTagIndexPathSpec{
			name: ts.repository.Named().Name(),
			tag:  tag,
		},
	}
	var dgsts []digest.Digest
	err := lbs.Enumerate(ctx, func(dgst digest.Digest) error {
		dgsts = append(dgsts, dgst)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dgsts, nil
}

// List returns the tags for the repository.
func (ts *tagStore) List(ctx context.Context, limit int, last string) ([]string, error) {
	filledBuffer := false
	foundTags := 0
	var tags []string

	if limit == 0 {
		return tags, errors.New("attempted to list 0 tags")
	}

	root, err := pathFor(manifestTagsPathSpec{
		name: ts.repository.Named().Name(),
	})
	if err != nil {
		return tags, err
	}

	startAfter := ""
	if last != "" {
		startAfter, err = pathFor(manifestTagPathSpec{
			name: ts.repository.Named().Name(),
			tag:  last,
		})
		if err != nil {
			return tags, err
		}
	}

	err = ts.blobStore.driver.Walk(ctx, root, func(fileInfo storagedriver.FileInfo) error {
		return handleTag(fileInfo, root, last, func(tagPath string) error {
			tags = append(tags, tagPath)
			foundTags += 1
			// if we've filled our slice, no need to walk any further
			if limit > 0 && foundTags == limit {
				filledBuffer = true
				return storagedriver.ErrFilledBuffer
			}
			return nil
		})
	}, storagedriver.WithStartAfterHint(startAfter))

	if err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return tags, distribution.ErrRepositoryUnknown{Name: ts.repository.Named().Name()}
		default:
			return tags, err
		}
	}

	if filledBuffer {
		// There are potentially more tags to list
		return tags, nil
	}

	// We didn't fill the buffer, so that's the end of the list of tags
	return tags, io.EOF
}

// handleTag calls function fn with a tag path if fileInfo
// has a path of a tag under root and that it is lexographically
// after last. Otherwise, it will return ErrSkipDir or ErrFilledBuffer.
// These should be used with Walk to do handling with repositories in a
// storage.
func handleTag(fileInfo storagedriver.FileInfo, root, last string, fn func(tagPath string) error) error {
	filePath := fileInfo.Path()

	// lop the base path off
	tag := filePath[len(root)+1:]
	parts := strings.SplitN(tag, "/", 2)
	if len(parts) > 1 {
		return storagedriver.ErrSkipDir
	}

	if lessPath(last, tag) {
		if err := fn(tag); err != nil {
			return err
		}
	}
	return storagedriver.ErrSkipDir
}

package storage

import (
	"fmt"

	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"golang.org/x/net/context"
)

// TODO(stevvooe): Currently, the blobStore implementation used by the
// manifest store. The layer store should be refactored to better leverage the
// blobStore, reducing duplicated code.

// blobStore implements a generalized blob store over a driver, supporting the
// read side and link management. This object is intentionally a leaky
// abstraction, providing utility methods that support creating and traversing
// backend links.
type blobStore struct {
	driver storagedriver.StorageDriver
	pm     *pathMapper
	ctx    context.Context
}

// exists reports whether or not the path exists. If the driver returns error
// other than storagedriver.PathNotFound, an error may be returned.
func (bs *blobStore) exists(dgst digest.Digest) (bool, error) {
	path, err := bs.path(dgst)

	if err != nil {
		return false, err
	}

	ok, err := exists(bs.driver, path)
	if err != nil {
		return false, err
	}

	return ok, nil
}

// get retrieves the blob by digest, returning it a byte slice. This should
// only be used for small objects.
func (bs *blobStore) get(dgst digest.Digest) ([]byte, error) {
	bp, err := bs.path(dgst)
	if err != nil {
		return nil, err
	}

	return bs.driver.GetContent(bp)
}

// link links the path to the provided digest by writing the digest into the
// target file.
func (bs *blobStore) link(path string, dgst digest.Digest) error {
	if exists, err := bs.exists(dgst); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("cannot link non-existent blob")
	}

	// The contents of the "link" file are the exact string contents of the
	// digest, which is specified in that package.
	return bs.driver.PutContent(path, []byte(dgst))
}

// linked reads the link at path and returns the content.
func (bs *blobStore) linked(path string) ([]byte, error) {
	linked, err := bs.readlink(path)
	if err != nil {
		return nil, err
	}

	return bs.get(linked)
}

// readlink returns the linked digest at path.
func (bs *blobStore) readlink(path string) (digest.Digest, error) {
	content, err := bs.driver.GetContent(path)
	if err != nil {
		return "", err
	}

	linked, err := digest.ParseDigest(string(content))
	if err != nil {
		return "", err
	}

	if exists, err := bs.exists(linked); err != nil {
		return "", err
	} else if !exists {
		return "", fmt.Errorf("link %q invalid: blob %s does not exist", path, linked)
	}

	return linked, nil
}

// resolve reads the digest link at path and returns the blob store link.
func (bs *blobStore) resolve(path string) (string, error) {
	dgst, err := bs.readlink(path)
	if err != nil {
		return "", err
	}

	return bs.path(dgst)
}

// put stores the content p in the blob store, calculating the digest. If the
// content is already present, only the digest will be returned. This should
// only be used for small objects, such as manifests.
func (bs *blobStore) put(p []byte) (digest.Digest, error) {
	dgst, err := digest.FromBytes(p)
	if err != nil {
		ctxu.GetLogger(bs.ctx).Errorf("error digesting content: %v, %s", err, string(p))
		return "", err
	}

	bp, err := bs.path(dgst)
	if err != nil {
		return "", err
	}

	// If the content already exists, just return the digest.
	if exists, err := bs.exists(dgst); err != nil {
		return "", err
	} else if exists {
		return dgst, nil
	}

	return dgst, bs.driver.PutContent(bp, p)
}

// path returns the canonical path for the blob identified by digest. The blob
// may or may not exist.
func (bs *blobStore) path(dgst digest.Digest) (string, error) {
	bp, err := bs.pm.path(blobDataPathSpec{
		digest: dgst,
	})

	if err != nil {
		return "", err
	}

	return bp, nil
}

// exists provides a utility method to test whether or not
func exists(driver storagedriver.StorageDriver, path string) (bool, error) {
	if _, err := driver.Stat(path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

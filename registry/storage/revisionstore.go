package storage

import (
	"encoding/json"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/libtrust"
)

// revisionStore supports storing and managing manifest revisions.
type revisionStore struct {
	*repository
	tomb tombstone
}

// exists returns true if the revision is available in the named repository.
func (rs *revisionStore) exists(revision digest.Digest) (bool, error) {
	ctx := rs.repository.ctx

	tombstoneExists, err := rs.tomb.tombstoneExists(ctx, rs.Name(), revision)
	if err != nil {
		return false, err
	}
	if tombstoneExists {
		return false, nil
	}

	revpath, err := rs.pm.path(manifestRevisionPathSpec{
		name:     rs.Name(),
		revision: revision,
	})

	if err != nil {
		return false, err
	}

	exists, err := exists(ctx, rs.driver, revpath)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// get retrieves the manifest, keyed by revision digest.
func (rs *revisionStore) get(revision digest.Digest) (*manifest.SignedManifest, error) {
	// Ensure that this revision is available in this repository.
	if exists, err := rs.exists(revision); err != nil {
		return nil, err
	} else if !exists {
		return nil, distribution.ErrUnknownManifestRevision{
			Name:     rs.Name(),
			Revision: revision,
		}
	}

	content, err := rs.blobStore.get(revision)
	if err != nil {
		return nil, err
	}

	// Fetch the signatures for the manifest
	signatures, err := rs.Signatures().Get(revision)
	if err != nil {
		return nil, err
	}

	jsig, err := libtrust.NewJSONSignature(content, signatures...)
	if err != nil {
		return nil, err
	}

	// Extract the pretty JWS
	raw, err := jsig.PrettySignature("signatures")
	if err != nil {
		return nil, err
	}

	var sm manifest.SignedManifest
	if err := json.Unmarshal(raw, &sm); err != nil {
		return nil, err
	}

	return &sm, nil
}

// put stores the manifest in the repository, if not already present. Any
// updated signatures will be stored, as well.
func (rs *revisionStore) put(sm *manifest.SignedManifest) (digest.Digest, error) {
	ctx := rs.repository.ctx
	// Resolve the payload in the manifest.
	payload, err := sm.Payload()
	if err != nil {
		return "", err
	}

	// This put may be restoring a previously deleted manifest
	// If so, remove the tombstone
	dgst, err := digest.FromBytes(payload)
	if err != nil {
		return "", err
	}

	tombstoneExists, err := rs.tomb.tombstoneExists(ctx, rs.Name(), dgst)
	if err != nil {
		return "", err
	}
	if tombstoneExists {
		if err := rs.tomb.deleteTombstone(ctx, rs.Name(), dgst); err != nil {
			return "", err
		}
	}

	// Digest and store the manifest payload in the blob store.
	revision, err := rs.blobStore.put(payload)
	if err != nil {
		logrus.Errorf("error putting payload into blobstore: %v", err)
		return "", err
	}

	// Link the revision into the repository.
	if err := rs.link(revision); err != nil {
		return "", err
	}

	// Grab each json signature and store them.
	signatures, err := sm.Signatures()
	if err != nil {
		return "", err
	}

	if err := rs.Signatures().Put(revision, signatures...); err != nil {
		return "", err
	}

	return revision, nil
}

// link links the revision into the repository.
func (rs *revisionStore) link(revision digest.Digest) error {
	revisionPath, err := rs.pm.path(manifestRevisionLinkPathSpec{
		name:     rs.Name(),
		revision: revision,
	})

	if err != nil {
		return err
	}

	if exists, err := exists(rs.repository.ctx, rs.driver, revisionPath); err != nil {
		return err
	} else if exists {
		// Revision has already been linked!
		return nil
	}

	return rs.blobStore.link(revisionPath, revision)
}

// delete creates a tombstone for the given digest revision.  The tombstone
// is a link file which points to the deleted manifest.
func (rs *revisionStore) delete(revision digest.Digest) error {
	// don't allow deleting a blob that doesn't already exist
	exists, err := rs.exists(revision)
	if err != nil {
		return err
	}

	if !exists {
		return distribution.ErrUnknownManifestRevision{
			Name:     rs.Name(),
			Revision: revision,
		}
	}

	if err := rs.tomb.putTombstone(rs.repository.ctx, rs.Name(), revision); err != nil {
		return err
	}

	return nil
}

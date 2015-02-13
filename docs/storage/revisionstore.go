package storage

import (
	"encoding/json"
	"path"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/libtrust"
)

// revisionStore supports storing and managing manifest revisions.
type revisionStore struct {
	*repository
}

// exists returns true if the revision is available in the named repository.
func (rs *revisionStore) exists(revision digest.Digest) (bool, error) {
	revpath, err := rs.pm.path(manifestRevisionPathSpec{
		name:     rs.Name(),
		revision: revision,
	})

	if err != nil {
		return false, err
	}

	exists, err := exists(rs.driver, revpath)
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
	signatures, err := rs.getSignatures(revision)
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
	// Resolve the payload in the manifest.
	payload, err := sm.Payload()
	if err != nil {
		return "", err
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

	for _, signature := range signatures {
		if err := rs.putSignature(revision, signature); err != nil {
			return "", err
		}
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

	if exists, err := exists(rs.driver, revisionPath); err != nil {
		return err
	} else if exists {
		// Revision has already been linked!
		return nil
	}

	return rs.blobStore.link(revisionPath, revision)
}

// delete removes the specified manifest revision from storage.
func (rs *revisionStore) delete(revision digest.Digest) error {
	revisionPath, err := rs.pm.path(manifestRevisionPathSpec{
		name:     rs.Name(),
		revision: revision,
	})

	if err != nil {
		return err
	}

	return rs.driver.Delete(revisionPath)
}

// getSignatures retrieves all of the signature blobs for the specified
// manifest revision.
func (rs *revisionStore) getSignatures(revision digest.Digest) ([][]byte, error) {
	signaturesPath, err := rs.pm.path(manifestSignaturesPathSpec{
		name:     rs.Name(),
		revision: revision,
	})

	if err != nil {
		return nil, err
	}

	// Need to append signature digest algorithm to path to get all items.
	// Perhaps, this should be in the pathMapper but it feels awkward. This
	// can be eliminated by implementing listAll on drivers.
	signaturesPath = path.Join(signaturesPath, "sha256")

	signaturePaths, err := rs.driver.List(signaturesPath)
	if err != nil {
		return nil, err
	}

	var signatures [][]byte
	for _, sigPath := range signaturePaths {
		// Append the link portion
		sigPath = path.Join(sigPath, "link")

		// TODO(stevvooe): These fetches should be parallelized for performance.
		p, err := rs.blobStore.linked(sigPath)
		if err != nil {
			return nil, err
		}

		signatures = append(signatures, p)
	}

	return signatures, nil
}

// putSignature stores the signature for the provided manifest revision.
func (rs *revisionStore) putSignature(revision digest.Digest, signature []byte) error {
	signatureDigest, err := rs.blobStore.put(signature)
	if err != nil {
		return err
	}

	signaturePath, err := rs.pm.path(manifestSignatureLinkPathSpec{
		name:      rs.Name(),
		revision:  revision,
		signature: signatureDigest,
	})

	if err != nil {
		return err
	}

	return rs.blobStore.link(signaturePath, signatureDigest)
}

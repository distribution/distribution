package storage

import (
	"fmt"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/libtrust"
)

type manifestStore struct {
	repository *repository

	revisionStore *revisionStore
}

var _ distribution.ManifestService = &manifestStore{}

func (ms *manifestStore) Exists(dgst digest.Digest) (bool, error) {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).Exists")
	return ms.revisionStore.exists(dgst)
}

func (ms *manifestStore) Get(dgst digest.Digest) (*manifest.SignedManifest, error) {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).Get")
	return ms.revisionStore.get(dgst)
}

func (ms *manifestStore) Put(manifest *manifest.SignedManifest) error {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).Put")

	// TODO(stevvooe): Add check here to see if the revision is already
	// present in the repository. If it is, we should merge the signatures, do
	// a shallow verify (or a full one, doesn't matter) and return an error
	// indicating what happened.

	// Verify the manifest.
	if err := ms.verifyManifest(manifest); err != nil {
		return err
	}

	// Store the revision of the manifest
	revision, err := ms.revisionStore.put(manifest)
	if err != nil {
		return err
	}

	// Now, tag the manifest
	return ms.tag(manifest.Tag, revision)
}

// Delete removes the revision of the specified manfiest.
func (ms *manifestStore) Delete(dgst digest.Digest) error {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).Delete - unsupported")
	return fmt.Errorf("deletion of manifests not supported")
}

// tag tags the digest with the given tag, updating the the store to point at
// the current tag. The digest must point to a manifest.
func (ms *manifestStore) tag(tag string, revision digest.Digest) error {
	indexEntryPath, err := ms.repository.pm.path(manifestTagIndexEntryLinkPathSpec{
		name:     ms.repository.Name(),
		tag:      tag,
		revision: revision,
	})

	if err != nil {
		return err
	}

	currentPath, err := ms.repository.pm.path(manifestTagCurrentPathSpec{
		name: ms.repository.Name(),
		tag:  tag,
	})

	if err != nil {
		return err
	}

	// Link into the index
	if err := ms.repository.blobStore.link(indexEntryPath, revision); err != nil {
		return err
	}

	// Overwrite the current link
	return ms.repository.blobStore.link(currentPath, revision)
}

// verifyManifest ensures that the manifest content is valid from the
// perspective of the registry. It ensures that the signature is valid for the
// enclosed payload. As a policy, the registry only tries to store valid
// content, leaving trust policies of that content up to consumers.
func (ms *manifestStore) verifyManifest(mnfst *manifest.SignedManifest) error {
	var errs distribution.ErrManifestVerification
	if mnfst.Name != ms.repository.Name() {
		// TODO(stevvooe): This needs to be an exported error
		errs = append(errs, fmt.Errorf("repository name does not match manifest name"))
	}

	if _, err := manifest.Verify(mnfst); err != nil {
		switch err {
		case libtrust.ErrMissingSignatureKey, libtrust.ErrInvalidJSONContent, libtrust.ErrMissingSignatureKey:
			errs = append(errs, distribution.ErrManifestUnverified{})
		default:
			if err.Error() == "invalid signature" { // TODO(stevvooe): This should be exported by libtrust
				errs = append(errs, distribution.ErrManifestUnverified{})
			} else {
				errs = append(errs, err)
			}
		}
	}

	for _, fsLayer := range mnfst.FSLayers {
		exists, err := ms.repository.Layers().Exists(fsLayer.BlobSum)
		if err != nil {
			errs = append(errs, err)
		}

		if !exists {
			errs = append(errs, distribution.ErrUnknownLayer{FSLayer: fsLayer})
		}
	}

	if len(errs) != 0 {
		// TODO(stevvooe): These need to be recoverable by a caller.
		return errs
	}

	return nil
}

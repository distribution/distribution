package storage

import (
	"encoding/json"
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
	tagStore      *tagStore
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
	return ms.tagStore.tag(manifest.Tag, revision)
}

// Delete removes the revision of the specified manfiest.
func (ms *manifestStore) Delete(dgst digest.Digest) error {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).Delete - unsupported")
	return fmt.Errorf("deletion of manifests not supported")
}

func (ms *manifestStore) Tags() ([]string, error) {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).Tags")
	return ms.tagStore.tags()
}

func (ms *manifestStore) ExistsByTag(tag string) (bool, error) {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).ExistsByTag")
	return ms.tagStore.exists(tag)
}

func (ms *manifestStore) GetByTag(tag string) (*manifest.SignedManifest, error) {
	ctxu.GetLogger(ms.repository.ctx).Debug("(*manifestStore).GetByTag")
	dgst, err := ms.tagStore.resolve(tag)
	if err != nil {
		return nil, err
	}

	return ms.revisionStore.get(dgst)
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

	if len(mnfst.FSLayers) == 0 || len(mnfst.History) == 0 {
		errs = append(errs, distribution.ErrManifestValidation{
			Reason: "no layers present"})
	}

	if len(mnfst.FSLayers) != len(mnfst.History) {
		errs = append(errs, distribution.ErrManifestValidation{
			Reason: "mismatched layers and history"})
	}

	// image provides a local type for validating the image relationship.
	type image struct {
		ID     string `json:"id"`
		Parent string `json:"parent"`
	}

	// Process the history portion to ensure that the parent links are
	// correctly represented. We serialize the image json, then walk the
	// entries, checking the parent link.
	var images []image
	for _, entry := range mnfst.History {
		var im image
		if err := json.Unmarshal([]byte(entry.V1Compatibility), &im); err != nil {
			errs = append(errs, err)
		}

		images = append(images, im)
	}

	// go back through each image, checking the parent link and rank
	var parentID string
	for i := len(images) - 1; i >= 0; i-- {
		// ensure that the parent id matches but only if there is a parent.
		// There are cases where successive layers don't fill in the parents.
		if images[i].Parent != parentID {
			errs = append(errs, distribution.ErrManifestValidation{
				Reason: "parent not adjacent in manifest"})
		}

		parentID = images[i].ID
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

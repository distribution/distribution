package storage

import (
	"encoding/json"
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/libtrust"
)

type manifestStore struct {
	repository                 *repository
	revisionStore              *revisionStore
	tagStore                   *tagStore
	ctx                        context.Context
	skipDependencyVerification bool
}

var _ distribution.ManifestService = &manifestStore{}

func (ms *manifestStore) Exists(dgst digest.Digest) (bool, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Exists")

	_, err := ms.revisionStore.blobStore.Stat(ms.ctx, dgst)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (ms *manifestStore) Get(dgst digest.Digest) (*manifest.SignedManifest, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Get")
	return ms.revisionStore.get(ms.ctx, dgst)
}

// SkipLayerVerification allows a manifest to be Put before it's
// layers are on the filesystem
func SkipLayerVerification(ms distribution.ManifestService) error {
	if ms, ok := ms.(*manifestStore); ok {
		ms.skipDependencyVerification = true
		return nil
	}
	return fmt.Errorf("skip layer verification only valid for manifeststore")
}

func (ms *manifestStore) Put(manifest *manifest.SignedManifest) error {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Put")

	if err := ms.verifyManifest(ms.ctx, manifest); err != nil {
		return err
	}

	// Store the revision of the manifest
	revision, err := ms.revisionStore.put(ms.ctx, manifest)
	if err != nil {
		return err
	}

	// Now, tag the manifest
	return ms.tagStore.tag(manifest.Tag, revision.Digest)
}

// Delete removes the revision of the specified manfiest.
func (ms *manifestStore) Delete(dgst digest.Digest) error {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Delete")
	return ms.revisionStore.delete(ms.ctx, dgst)
}

func (ms *manifestStore) Tags() ([]string, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Tags")
	return ms.tagStore.tags()
}

func (ms *manifestStore) ExistsByTag(tag string) (bool, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).ExistsByTag")
	return ms.tagStore.exists(tag)
}

func (ms *manifestStore) GetByTag(tag string, options ...distribution.ManifestServiceOption) (*manifest.SignedManifest, error) {
	for _, option := range options {
		err := option(ms)
		if err != nil {
			return nil, err
		}
	}

	context.GetLogger(ms.ctx).Debug("(*manifestStore).GetByTag")
	dgst, err := ms.tagStore.resolve(tag)
	if err != nil {
		return nil, err
	}

	return ms.revisionStore.get(ms.ctx, dgst)
}

// verifyManifest ensures that the manifest content is valid from the
// perspective of the registry. It ensures that the signature is valid for the
// enclosed payload. As a policy, the registry only tries to store valid
// content, leaving trust policies of that content up to consumers.
func (ms *manifestStore) verifyManifest(ctx context.Context, mnfst *manifest.SignedManifest) error {
	var errs distribution.ErrManifestVerification
	if mnfst.Name != ms.repository.Name() {
		errs = append(errs, distribution.ErrManifestValidation{
			Reason: "repository name does not match manifest name"})
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

	if !ms.skipDependencyVerification {
		if len(mnfst.FSLayers) == 0 || len(mnfst.History) == 0 {
			errs = append(errs, distribution.ErrManifestValidation{
				Reason: "no layers present"})
		}

		if len(mnfst.FSLayers) != len(mnfst.History) {
			errs = append(errs, distribution.ErrManifestValidation{
				Reason: "mismatched layers and history"})
		}

		for _, fsLayer := range mnfst.FSLayers {
			_, err := ms.repository.Blobs(ctx).Stat(ctx, fsLayer.BlobSum)
			if err != nil {
				if err != distribution.ErrBlobUnknown {
					errs = append(errs, err)
				}

				// On error here, we always append unknown blob errors.
				errs = append(errs, distribution.ErrManifestBlobUnknown{Digest: fsLayer.BlobSum})
			}
		}

		// image provides a local type for validating the image relationship.
		type image struct {
			Id     string
			Parent string
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
			// ensure that the parent id matches parent. There are cases where
			// successive layers don't fill in the parents but parent id
			// should be empty so it should work out.
			if images[i].Parent != parentID {
				errs = append(errs, distribution.ErrManifestValidation{
					Reason: "parent not adjacent in manifest"})
			}

			parentID = images[i].Id
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

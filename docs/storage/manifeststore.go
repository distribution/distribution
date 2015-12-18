package storage

import (
	"encoding/json"
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/reference"
	"github.com/docker/libtrust"
)

// manifestStore is a storage driver based store for storing schema1 manifests.
type manifestStore struct {
	repository                 *repository
	blobStore                  *linkedBlobStore
	ctx                        context.Context
	signatures                 *signatureStore
	skipDependencyVerification bool
}

var _ distribution.ManifestService = &manifestStore{}

func (ms *manifestStore) Exists(ctx context.Context, dgst digest.Digest) (bool, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Exists")

	_, err := ms.blobStore.Stat(ms.ctx, dgst)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

func (ms *manifestStore) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Get")
	// Ensure that this revision is available in this repository.
	_, err := ms.blobStore.Stat(ctx, dgst)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			return nil, distribution.ErrManifestUnknownRevision{
				Name:     ms.repository.Name(),
				Revision: dgst,
			}
		}

		return nil, err
	}

	// TODO(stevvooe): Need to check descriptor from above to ensure that the
	// mediatype is as we expect for the manifest store.

	content, err := ms.blobStore.Get(ctx, dgst)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			return nil, distribution.ErrManifestUnknownRevision{
				Name:     ms.repository.Name(),
				Revision: dgst,
			}
		}

		return nil, err
	}

	// Fetch the signatures for the manifest
	signatures, err := ms.signatures.Get(dgst)
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

	var sm schema1.SignedManifest
	if err := json.Unmarshal(raw, &sm); err != nil {
		return nil, err
	}

	return &sm, nil
}

// SkipLayerVerification allows a manifest to be Put before its
// layers are on the filesystem
func SkipLayerVerification() distribution.ManifestServiceOption {
	return skipLayerOption{}
}

type skipLayerOption struct{}

func (o skipLayerOption) Apply(m distribution.ManifestService) error {
	if ms, ok := m.(*manifestStore); ok {
		ms.skipDependencyVerification = true
		return nil
	}
	return fmt.Errorf("skip layer verification only valid for manifestStore")
}

func (ms *manifestStore) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (digest.Digest, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Put")

	sm, ok := manifest.(*schema1.SignedManifest)
	if !ok {
		return "", fmt.Errorf("non-v1 manifest put to signed manifestStore: %T", manifest)
	}

	if err := ms.verifyManifest(ms.ctx, *sm); err != nil {
		return "", err
	}

	mt := schema1.MediaTypeManifest
	payload := sm.Canonical

	revision, err := ms.blobStore.Put(ctx, mt, payload)
	if err != nil {
		context.GetLogger(ctx).Errorf("error putting payload into blobstore: %v", err)
		return "", err
	}

	// Link the revision into the repository.
	if err := ms.blobStore.linkBlob(ctx, revision); err != nil {
		return "", err
	}

	// Grab each json signature and store them.
	signatures, err := sm.Signatures()
	if err != nil {
		return "", err
	}

	if err := ms.signatures.Put(revision.Digest, signatures...); err != nil {
		return "", err
	}

	return revision.Digest, nil
}

// Delete removes the revision of the specified manfiest.
func (ms *manifestStore) Delete(ctx context.Context, dgst digest.Digest) error {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Delete")
	return ms.blobStore.Delete(ctx, dgst)
}

func (ms *manifestStore) Enumerate(ctx context.Context, manifests []distribution.Manifest, last distribution.Manifest) (n int, err error) {
	return 0, distribution.ErrUnsupported
}

// verifyManifest ensures that the manifest content is valid from the
// perspective of the registry. It ensures that the signature is valid for the
// enclosed payload. As a policy, the registry only tries to store valid
// content, leaving trust policies of that content up to consumems.
func (ms *manifestStore) verifyManifest(ctx context.Context, mnfst schema1.SignedManifest) error {
	var errs distribution.ErrManifestVerification

	if len(mnfst.Name) > reference.NameTotalLengthMax {
		errs = append(errs,
			distribution.ErrManifestNameInvalid{
				Name:   mnfst.Name,
				Reason: fmt.Errorf("manifest name must not be more than %v characters", reference.NameTotalLengthMax),
			})
	}

	if !reference.NameRegexp.MatchString(mnfst.Name) {
		errs = append(errs,
			distribution.ErrManifestNameInvalid{
				Name:   mnfst.Name,
				Reason: fmt.Errorf("invalid manifest name format"),
			})
	}

	if len(mnfst.History) != len(mnfst.FSLayers) {
		errs = append(errs, fmt.Errorf("mismatched history and fslayer cardinality %d != %d",
			len(mnfst.History), len(mnfst.FSLayers)))
	}

	if _, err := schema1.Verify(&mnfst); err != nil {
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
		for _, fsLayer := range mnfst.References() {
			_, err := ms.repository.Blobs(ctx).Stat(ctx, fsLayer.Digest)
			if err != nil {
				if err != distribution.ErrBlobUnknown {
					errs = append(errs, err)
				}

				// On error here, we always append unknown blob erroms.
				errs = append(errs, distribution.ErrManifestBlobUnknown{Digest: fsLayer.Digest})
			}
		}
	}
	if len(errs) != 0 {
		return errs
	}

	return nil
}

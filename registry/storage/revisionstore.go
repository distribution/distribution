package storage

import (
	"encoding/json"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/libtrust"
)

// revisionStore supports storing and managing manifest revisions.
type revisionStore struct {
	repository *repository
	blobStore  *linkedBlobStore
	ctx        context.Context
}

// get retrieves the manifest, keyed by revision digest.
func (rs *revisionStore) get(ctx context.Context, revision digest.Digest) (*manifest.SignedManifest, error) {
	// Ensure that this revision is available in this repository.
	_, err := rs.blobStore.Stat(ctx, revision)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			return nil, distribution.ErrManifestUnknownRevision{
				Name:     rs.repository.Name(),
				Revision: revision,
			}
		}

		return nil, err
	}

	// TODO(stevvooe): Need to check descriptor from above to ensure that the
	// mediatype is as we expect for the manifest store.

	content, err := rs.blobStore.Get(ctx, revision)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			return nil, distribution.ErrManifestUnknownRevision{
				Name:     rs.repository.Name(),
				Revision: revision,
			}
		}

		return nil, err
	}

	// Fetch the signatures for the manifest
	signatures, err := rs.repository.Signatures().Get(revision)
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
func (rs *revisionStore) put(ctx context.Context, sm *manifest.SignedManifest) (distribution.Descriptor, error) {
	// Resolve the payload in the manifest.
	payload, err := sm.Payload()
	if err != nil {
		return distribution.Descriptor{}, err
	}

	// Digest and store the manifest payload in the blob store.
	revision, err := rs.blobStore.Put(ctx, manifest.ManifestMediaType, payload)
	if err != nil {
		context.GetLogger(ctx).Errorf("error putting payload into blobstore: %v", err)
		return distribution.Descriptor{}, err
	}

	// Link the revision into the repository.
	if err := rs.blobStore.linkBlob(ctx, revision); err != nil {
		return distribution.Descriptor{}, err
	}

	// Grab each json signature and store them.
	signatures, err := sm.Signatures()
	if err != nil {
		return distribution.Descriptor{}, err
	}

	if err := rs.repository.Signatures().Put(revision.Digest, signatures...); err != nil {
		return distribution.Descriptor{}, err
	}

	return revision, nil
}

func (rs *revisionStore) delete(ctx context.Context, revision digest.Digest) error {
	return rs.blobStore.Delete(ctx, revision)
}

package storage

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/opencontainers/go-digest"
)

// manifestListHandler is a ManifestHandler that covers schema2 manifest lists.
type manifestListHandler struct {
	repository           distribution.Repository
	blobStore            distribution.BlobStore
	ctx                  context.Context
	validateImageIndexes validateImageIndexes
}

var _ ManifestHandler = &manifestListHandler{}

func (ms *manifestListHandler) Unmarshal(ctx context.Context, dgst digest.Digest, content []byte) (distribution.Manifest, error) {
	dcontext.GetLogger(ms.ctx).Debug("(*manifestListHandler).Unmarshal")

	m := &manifestlist.DeserializedManifestList{}
	if err := m.UnmarshalJSON(content); err != nil {
		return nil, err
	}

	return m, nil
}

func (ms *manifestListHandler) Put(ctx context.Context, manifestList distribution.Manifest, skipDependencyVerification bool) (digest.Digest, error) {
	dcontext.GetLogger(ms.ctx).Debug("(*manifestListHandler).Put")

	var schemaVersion, expectedSchemaVersion int
	switch m := manifestList.(type) {
	case *manifestlist.DeserializedManifestList:
		expectedSchemaVersion = manifestlist.SchemaVersion.SchemaVersion
		schemaVersion = m.SchemaVersion
	case *ocischema.DeserializedImageIndex:
		expectedSchemaVersion = ocischema.IndexSchemaVersion.SchemaVersion
		schemaVersion = m.SchemaVersion
	default:
		return "", fmt.Errorf("wrong type put to manifestListHandler: %T", manifestList)
	}
	if schemaVersion != expectedSchemaVersion {
		return "", fmt.Errorf("unrecognized manifest list schema version %d, expected %d", schemaVersion, expectedSchemaVersion)
	}

	if err := ms.verifyManifest(ms.ctx, manifestList, skipDependencyVerification); err != nil {
		return "", err
	}

	mt, payload, err := manifestList.Payload()
	if err != nil {
		return "", err
	}

	revision, err := ms.blobStore.Put(ctx, mt, payload)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("error putting payload into blobstore: %v", err)
		return "", err
	}

	return revision.Digest, nil
}

// verifyManifest ensures that the manifest content is valid from the
// perspective of the registry. As a policy, the registry only tries to
// store valid content, leaving trust policies of that content up to
// consumers.
func (ms *manifestListHandler) verifyManifest(ctx context.Context, mnfst distribution.Manifest, skipDependencyVerification bool) error {
	var errs distribution.ErrManifestVerification

	// Check if we should be validating the existence of any child images in images indexes
	if ms.validateImageIndexes.imagesExist && !skipDependencyVerification {
		// Get the manifest service we can use to check for the existence of child images
		manifestService, err := ms.repository.Manifests(ctx)
		if err != nil {
			return err
		}

		for _, manifestDescriptor := range mnfst.References() {
			if ms.platformMustExist(manifestDescriptor) {
				exists, err := manifestService.Exists(ctx, manifestDescriptor.Digest)
				if err != nil && err != distribution.ErrBlobUnknown {
					errs = append(errs, err)
				}
				if err != nil || !exists {
					// On error here, we always append unknown blob errors.
					errs = append(errs, distribution.ErrManifestBlobUnknown{Digest: manifestDescriptor.Digest})
				}
			}
		}
	}
	if len(errs) != 0 {
		return errs
	}

	return nil
}

// platformMustExist checks if a descriptor within an index should be validated as existing before accepting the manifest into the registry.
func (ms *manifestListHandler) platformMustExist(descriptor distribution.Descriptor) bool {
	// If there are no image platforms configured to validate, we must check the existence of all child images.
	if len(ms.validateImageIndexes.imagePlatforms) == 0 {
		return true
	}

	imagePlatform := descriptor.Platform

	// If the platform matches a platform that is configured to validate, we must check the existence.
	for _, platform := range ms.validateImageIndexes.imagePlatforms {
		if imagePlatform.Architecture == platform.architecture &&
			imagePlatform.OS == platform.os {
			return true
		}
	}

	// If the platform doesn't match a platform configured to validate, we don't need to check the existence.
	return false
}

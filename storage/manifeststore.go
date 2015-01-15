package storage

import (
	"fmt"
	"strings"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storagedriver"
	"github.com/docker/libtrust"
)

// ErrUnknownRepository is returned if the named repository is not known by
// the registry.
type ErrUnknownRepository struct {
	Name string
}

func (err ErrUnknownRepository) Error() string {
	return fmt.Sprintf("unknown respository name=%s", err.Name)
}

// ErrUnknownManifest is returned if the manifest is not known by the
// registry.
type ErrUnknownManifest struct {
	Name string
	Tag  string
}

func (err ErrUnknownManifest) Error() string {
	return fmt.Sprintf("unknown manifest name=%s tag=%s", err.Name, err.Tag)
}

// ErrUnknownManifestRevision is returned when a manifest cannot be found by
// revision within a repository.
type ErrUnknownManifestRevision struct {
	Name     string
	Revision digest.Digest
}

func (err ErrUnknownManifestRevision) Error() string {
	return fmt.Sprintf("unknown manifest name=%s revision=%s", err.Name, err.Revision)
}

// ErrManifestUnverified is returned when the registry is unable to verify
// the manifest.
type ErrManifestUnverified struct{}

func (ErrManifestUnverified) Error() string {
	return fmt.Sprintf("unverified manifest")
}

// ErrManifestVerification provides a type to collect errors encountered
// during manifest verification. Currently, it accepts errors of all types,
// but it may be narrowed to those involving manifest verification.
type ErrManifestVerification []error

func (errs ErrManifestVerification) Error() string {
	var parts []string
	for _, err := range errs {
		parts = append(parts, err.Error())
	}

	return fmt.Sprintf("errors verifying manifest: %v", strings.Join(parts, ","))
}

type manifestStore struct {
	driver        storagedriver.StorageDriver
	pathMapper    *pathMapper
	revisionStore *revisionStore
	tagStore      *tagStore
	blobStore     *blobStore
	layerService  LayerService
}

var _ ManifestService = &manifestStore{}

func (ms *manifestStore) Tags(name string) ([]string, error) {
	return ms.tagStore.tags(name)
}

func (ms *manifestStore) Exists(name, tag string) (bool, error) {
	return ms.tagStore.exists(name, tag)
}

func (ms *manifestStore) Get(name, tag string) (*manifest.SignedManifest, error) {
	dgst, err := ms.tagStore.resolve(name, tag)
	if err != nil {
		return nil, err
	}

	return ms.revisionStore.get(name, dgst)
}

func (ms *manifestStore) Put(name, tag string, manifest *manifest.SignedManifest) error {
	// Verify the manifest.
	if err := ms.verifyManifest(name, tag, manifest); err != nil {
		return err
	}

	// Store the revision of the manifest
	revision, err := ms.revisionStore.put(name, manifest)
	if err != nil {
		return err
	}

	// Now, tag the manifest
	return ms.tagStore.tag(name, tag, revision)
}

// Delete removes all revisions of the given tag. We may want to change these
// semantics in the future, but this will maintain consistency. The underlying
// blobs are left alone.
func (ms *manifestStore) Delete(name, tag string) error {
	revisions, err := ms.tagStore.revisions(name, tag)
	if err != nil {
		return err
	}

	for _, revision := range revisions {
		if err := ms.revisionStore.delete(name, revision); err != nil {
			return err
		}
	}

	return ms.tagStore.delete(name, tag)
}

// verifyManifest ensures that the manifest content is valid from the
// perspective of the registry. It ensures that the name and tag match and
// that the signature is valid for the enclosed payload. As a policy, the
// registry only tries to store valid content, leaving trust policies of that
// content up to consumers.
func (ms *manifestStore) verifyManifest(name, tag string, mnfst *manifest.SignedManifest) error {
	var errs ErrManifestVerification
	if mnfst.Name != name {
		// TODO(stevvooe): This needs to be an exported error
		errs = append(errs, fmt.Errorf("name does not match manifest name"))
	}

	if mnfst.Tag != tag {
		// TODO(stevvooe): This needs to be an exported error.
		errs = append(errs, fmt.Errorf("tag does not match manifest tag"))
	}

	if _, err := manifest.Verify(mnfst); err != nil {
		switch err {
		case libtrust.ErrMissingSignatureKey, libtrust.ErrInvalidJSONContent, libtrust.ErrMissingSignatureKey:
			errs = append(errs, ErrManifestUnverified{})
		default:
			if err.Error() == "invalid signature" { // TODO(stevvooe): This should be exported by libtrust
				errs = append(errs, ErrManifestUnverified{})
			} else {
				errs = append(errs, err)
			}
		}
	}

	for _, fsLayer := range mnfst.FSLayers {
		exists, err := ms.layerService.Exists(name, fsLayer.BlobSum)
		if err != nil {
			errs = append(errs, err)
		}

		if !exists {
			errs = append(errs, ErrUnknownLayer{FSLayer: fsLayer})
		}
	}

	if len(errs) != 0 {
		// TODO(stevvooe): These need to be recoverable by a caller.
		return errs
	}

	return nil
}

package storage

import (
	"fmt"

	"encoding/json"
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
)

type manifestRevisionEnumKind int

const (
	// TODO(miminar): shall we distinguish more kinds?
	// m. revision may be unlinked, malformed, failing verification,
	// dangling (link points to missing blob) ...
	manifestRevisionEnumValid manifestRevisionEnumKind = iota
	manifestRevisionEnumOrphaned
	manifestRevisionEnumAll
)

// A ManifestHandler gets and puts manifests of a particular type.
type ManifestHandler interface {
	// Unmarshal unmarshals the manifest from a byte slice.
	Unmarshal(ctx context.Context, dgst digest.Digest, content []byte) (distribution.Manifest, error)

	// Put creates or updates the given manifest returning the manifest digest.
	Put(ctx context.Context, manifest distribution.Manifest, skipDependencyVerification bool) (digest.Digest, error)
}

// EnumerateOrphanedManifestRevisions makes manifest service enumerate orphaned
// (unlinked, deleted) manifest revisions instead of valid ones. Orphaned
// manifest revision is returned as a manifest schema2 with
// Manifest.Config.MediaType set to MediaTypeOrphanedManifestRevision.
func EnumerateOrphanedManifestRevisions() distribution.ManifestServiceOption {
	return enumerateManifestRevisionKindOption{manifestRevisionEnumOrphaned}
}

// EnumerateAllManifestRevisions makes manifest service enumerate both valid
// and orphaned manifest revisions instead of just valid ones.
func EnumerateAllManifestRevisions() distribution.ManifestServiceOption {
	return enumerateManifestRevisionKindOption{manifestRevisionEnumAll}
}

type enumerateManifestRevisionKindOption struct {
	kind manifestRevisionEnumKind
}

func (o enumerateManifestRevisionKindOption) Apply(m distribution.ManifestService) error {
	if ms, ok := m.(*manifestStore); ok {
		ms.enumerateKind = o.kind
		return nil
	}
	return fmt.Errorf("enumerate manifest revision kind only valid for manifestStore")
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

// EnumerateManifestRevisions is an utility function for enumerating manifest
// revisions of given manifest store. fn callback will be called for any
// manifest digest found. Kind of digests yielded can be controlled with
// enumerateManifestRevisionKindOption. If a digest refers to valid blob,
// callback will be called with manifest object. If it cannot be read, callback
// will be called with an error. Any error returned from callback will stop the
// enumeration. If all the digests are processed, io.EOF will be returned.
func EnumerateManifestRevisions(ms distribution.ManifestService, ctx context.Context, fn func(dgst digest.Digest, manifest distribution.Manifest, err error) error) error {
	store, ok := ms.(*manifestStore)
	if !ok {
		return fmt.Errorf("enumerate manifest revisions only valid for manifestStore")
	}

	return store.blobStore.Enumerate(ctx, func(dgst digest.Digest) error {
		m, err := store.Get(ctx, dgst)
		if err == nil && store.enumerateKind != manifestRevisionEnumOrphaned {
			if fn(dgst, m, nil) != nil {
				return ErrFinishedWalk
			}
		} else if err != nil && store.enumerateKind != manifestRevisionEnumValid {
			if fn(dgst, nil, err) != nil {
				return ErrFinishedWalk
			}
		}
		return nil
	})
}

type manifestStore struct {
	repository *repository
	blobStore  *linkedBlobStore
	ctx        context.Context

	enumerateKind              manifestRevisionEnumKind
	skipDependencyVerification bool

	schema1Handler      ManifestHandler
	schema2Handler      ManifestHandler
	manifestListHandler ManifestHandler
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

	// TODO(stevvooe): Need to check descriptor from above to ensure that the
	// mediatype is as we expect for the manifest store.

	content, err := ms.blobStore.Get(ctx, dgst)
	if err != nil {
		if err == distribution.ErrBlobUnknown {
			return nil, distribution.ErrManifestUnknownRevision{
				Name:     ms.repository.Name().Name(),
				Revision: dgst,
			}
		}

		return nil, err
	}

	var versioned manifest.Versioned
	if err = json.Unmarshal(content, &versioned); err != nil {
		return nil, err
	}

	switch versioned.SchemaVersion {
	case 1:
		return ms.schema1Handler.Unmarshal(ctx, dgst, content)
	case 2:
		// This can be an image manifest or a manifest list
		switch versioned.MediaType {
		case schema2.MediaTypeManifest:
			return ms.schema2Handler.Unmarshal(ctx, dgst, content)
		case manifestlist.MediaTypeManifestList:
			return ms.manifestListHandler.Unmarshal(ctx, dgst, content)
		default:
			return nil, distribution.ErrManifestVerification{fmt.Errorf("unrecognized manifest content type %s", versioned.MediaType)}
		}
	}

	return nil, fmt.Errorf("unrecognized manifest schema version %d", versioned.SchemaVersion)
}

func (ms *manifestStore) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (digest.Digest, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Put")

	switch manifest.(type) {
	case *schema1.SignedManifest:
		return ms.schema1Handler.Put(ctx, manifest, ms.skipDependencyVerification)
	case *schema2.DeserializedManifest:
		return ms.schema2Handler.Put(ctx, manifest, ms.skipDependencyVerification)
	case *manifestlist.DeserializedManifestList:
		return ms.manifestListHandler.Put(ctx, manifest, ms.skipDependencyVerification)
	}

	return "", fmt.Errorf("unrecognized manifest type %T", manifest)
}

// Delete removes the revision of the specified manfiest.
func (ms *manifestStore) Delete(ctx context.Context, dgst digest.Digest) error {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Delete")
	return ms.blobStore.Delete(ctx, dgst)
}

func (ms *manifestStore) Enumerate(ctx context.Context, ingester func(digest.Digest) error) error {
	context.GetLogger(ms.ctx).Debug("(*manifestStore).Enumerate")
	return EnumerateManifestRevisions(ms, ctx, func(dgst digest.Digest, m distribution.Manifest, err error) error {
		return ingester(dgst)
	})
}

package middleware

import (
	"fmt"

	middlewareErrors "github.com/docker/dhe-deploy/registry/middleware/errors"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/registry/handlers"
	"github.com/docker/libtrust"
)

// manifestStore provides an alternative backing mechanism for manifests.
// It must implement the ManifestService to store manifests and
// ManifestEnumerator for garbage collection and listing
type manifestStore struct {
	// useFilesystemStore is a flag which determines whether to use the default
	// filesystem service for all read actions. We need to fall back to the
	// filesystem for checking whether manifests exist if the metadata store
	// is still syncing.
	//
	// TODO (tonyhb) Determine whether the metadata store is faster; if it's
	// not we can remove this flag and always use distribution's filesystem
	// store for read operations
	useFilesystemStore bool

	app        *handlers.App
	ctx        context.Context
	store      Store
	signingKey libtrust.PrivateKey

	repo        distribution.Repository
	blobService distribution.ManifestService
}

func (m *manifestStore) Exists(ctx context.Context, dgst digest.Digest) (bool, error) {
	return m.blobService.Exists(ctx, dgst)
}

// Get retrieves the manifest specified by the given digest for a repo.
//
// Note that the middleware itself verifies that the manifest is valid;
// the storage backend should only marshal and unmarshal into the correct type.
func (m *manifestStore) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	return m.blobService.Get(ctx, dgst, options...)
}

// Put creates or updates the given manifest returning the manifest digest
func (m *manifestStore) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (d digest.Digest, err error) {
	// First, ensure we write the manifest to the filesystem as per standard
	// distribution code.
	if d, err = m.blobService.Put(ctx, manifest, options...); err != nil {
		context.GetLoggerWithField(ctx, "err", err).Error("error savng manifest to blobstore")
		return d, err
	}

	// NOTE: we're not allowing skipDependencyVerification here.
	//
	// skipDependencyVerification is ONLY used when registry is set up as a
	// pull-through cache (proxy). In these circumstances this middleware
	// should not be used, therefore this verification implementation always
	// verifies blobs.
	//
	// This is the only difference in implementation with storage's
	// manifestStore{}
	switch manifest.(type) {
	case *schema1.SignedManifest:
		err = m.VerifyV1(ctx, manifest.(*schema1.SignedManifest))
	case *schema2.DeserializedManifest:
		ctx, err = m.VerifyV2(ctx, manifest.(*schema2.DeserializedManifest))
	case *manifestlist.DeserializedManifestList:
		err = m.VerifyList(ctx, manifest.(*manifestlist.DeserializedManifestList))
	default:
		err = fmt.Errorf("Unknown manifest type: %T", manifest)
	}

	if err != nil {
		return
	}

	// Our storage service needs the digest of the manifest in order to
	// store the manifest under the correct key.
	_, data, err := manifest.Payload()
	if err != nil {
		return
	}

	// NOTE that for v1 manifests .Payload() returns the entire manifest including
	// the randomly generated signature. Digests must always be calculated on the
	// canonical manifest without signatures.
	if man, ok := manifest.(*schema1.SignedManifest); ok {
		data = man.Canonical
	}

	dgst := digest.FromBytes(data)
	err = m.store.PutManifest(ctx, m.repo.Named().String(), string(dgst), manifest)
	return dgst, err
}

// Delete removes the manifest specified by the given digest.
func (m *manifestStore) Delete(ctx context.Context, dgst digest.Digest) error {
	key := m.key(dgst)

	// First delete from the manifest store in rethinkDB. We can silently ignore
	// ErrNotFound issues - when deleting a tag from DTR's API the manifest
	// will already be removed from the tagstore if no tags reference it.
	// Unfortunately, this API call cannot delete manifests from the blobstore
	// so this will be called directly.
	_, err := m.store.GetManifest(ctx, key)
	if err != nil && err != middlewareErrors.ErrNotFound {
		context.GetLoggerWithField(ctx, "err", err).Error("error getting manifest from metadata store")
		return err
	}
	if err := m.store.DeleteManifest(ctx, key); err != nil {
		context.GetLoggerWithField(ctx, "err", err).Error("error deleting manifest from metadata store")
		return err
	}

	// Delete this within the blobService
	return m.blobService.Delete(ctx, dgst)
}

func (m *manifestStore) key(dgst digest.Digest) string {
	return m.repo.Named().String() + "@" + string(dgst)
}

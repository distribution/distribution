package storage

import (
	"net/http"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/uuid"
)

// linkedBlobStore provides a full BlobService that namespaces the blobs to a
// given repository. Effectively, it manages the links in a given repository
// that grant access to the global blob store.
type linkedBlobStore struct {
	*blobStore
	blobServer distribution.BlobServer
	statter    distribution.BlobStatter
	repository distribution.Repository
	ctx        context.Context // only to be used where context can't come through method args

	// linkPath allows one to control the repository blob link set to which
	// the blob store dispatches. This is required because manifest and layer
	// blobs have not yet been fully merged. At some point, this functionality
	// should be removed an the blob links folder should be merged.
	linkPath func(pm *pathMapper, name string, dgst digest.Digest) (string, error)
}

var _ distribution.BlobStore = &linkedBlobStore{}

func (lbs *linkedBlobStore) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	return lbs.statter.Stat(ctx, dgst)
}

func (lbs *linkedBlobStore) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	canonical, err := lbs.Stat(ctx, dgst) // access check
	if err != nil {
		return nil, err
	}

	return lbs.blobStore.Get(ctx, canonical.Digest)
}

func (lbs *linkedBlobStore) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	canonical, err := lbs.Stat(ctx, dgst) // access check
	if err != nil {
		return nil, err
	}

	return lbs.blobStore.Open(ctx, canonical.Digest)
}

func (lbs *linkedBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	canonical, err := lbs.Stat(ctx, dgst) // access check
	if err != nil {
		return err
	}

	if canonical.MediaType != "" {
		// Set the repository local content type.
		w.Header().Set("Content-Type", canonical.MediaType)
	}

	return lbs.blobServer.ServeBlob(ctx, w, r, canonical.Digest)
}

func (lbs *linkedBlobStore) Put(ctx context.Context, mediaType string, p []byte) (distribution.Descriptor, error) {
	// Place the data in the blob store first.
	desc, err := lbs.blobStore.Put(ctx, mediaType, p)
	if err != nil {
		context.GetLogger(ctx).Errorf("error putting into main store: %v", err)
		return distribution.Descriptor{}, err
	}

	// TODO(stevvooe): Write out mediatype if incoming differs from what is
	// returned by Put above. Note that we should allow updates for a given
	// repository.

	return desc, lbs.linkBlob(ctx, desc)
}

// Writer begins a blob write session, returning a handle.
func (lbs *linkedBlobStore) Create(ctx context.Context) (distribution.BlobWriter, error) {
	context.GetLogger(ctx).Debug("(*linkedBlobStore).Writer")

	uuid := uuid.Generate().String()
	startedAt := time.Now().UTC()

	path, err := lbs.blobStore.pm.path(uploadDataPathSpec{
		name: lbs.repository.Name(),
		id:   uuid,
	})

	if err != nil {
		return nil, err
	}

	startedAtPath, err := lbs.blobStore.pm.path(uploadStartedAtPathSpec{
		name: lbs.repository.Name(),
		id:   uuid,
	})

	if err != nil {
		return nil, err
	}

	// Write a startedat file for this upload
	if err := lbs.blobStore.driver.PutContent(ctx, startedAtPath, []byte(startedAt.Format(time.RFC3339))); err != nil {
		return nil, err
	}

	return lbs.newBlobUpload(ctx, uuid, path, startedAt)
}

func (lbs *linkedBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	context.GetLogger(ctx).Debug("(*linkedBlobStore).Resume")

	startedAtPath, err := lbs.blobStore.pm.path(uploadStartedAtPathSpec{
		name: lbs.repository.Name(),
		id:   id,
	})

	if err != nil {
		return nil, err
	}

	startedAtBytes, err := lbs.blobStore.driver.GetContent(ctx, startedAtPath)
	if err != nil {
		switch err := err.(type) {
		case driver.PathNotFoundError:
			return nil, distribution.ErrBlobUploadUnknown
		default:
			return nil, err
		}
	}

	startedAt, err := time.Parse(time.RFC3339, string(startedAtBytes))
	if err != nil {
		return nil, err
	}

	path, err := lbs.pm.path(uploadDataPathSpec{
		name: lbs.repository.Name(),
		id:   id,
	})

	if err != nil {
		return nil, err
	}

	return lbs.newBlobUpload(ctx, id, path, startedAt)
}

// newLayerUpload allocates a new upload controller with the given state.
func (lbs *linkedBlobStore) newBlobUpload(ctx context.Context, uuid, path string, startedAt time.Time) (distribution.BlobWriter, error) {
	fw, err := newFileWriter(ctx, lbs.driver, path)
	if err != nil {
		return nil, err
	}

	bw := &blobWriter{
		blobStore:          lbs,
		id:                 uuid,
		startedAt:          startedAt,
		bufferedFileWriter: *fw,
	}

	bw.setupResumableDigester()

	return bw, nil
}

// linkBlob links a valid, written blob into the registry under the named
// repository for the upload controller.
func (lbs *linkedBlobStore) linkBlob(ctx context.Context, canonical distribution.Descriptor, aliases ...digest.Digest) error {
	dgsts := append([]digest.Digest{canonical.Digest}, aliases...)

	// TODO(stevvooe): Need to write out mediatype for only canonical hash
	// since we don't care about the aliases. They are generally unused except
	// for tarsum but those versions don't care about mediatype.

	// Don't make duplicate links.
	seenDigests := make(map[digest.Digest]struct{}, len(dgsts))

	for _, dgst := range dgsts {
		if _, seen := seenDigests[dgst]; seen {
			continue
		}
		seenDigests[dgst] = struct{}{}

		blobLinkPath, err := lbs.linkPath(lbs.pm, lbs.repository.Name(), dgst)
		if err != nil {
			return err
		}

		if err := lbs.blobStore.link(ctx, blobLinkPath, canonical.Digest); err != nil {
			return err
		}
	}

	return nil
}

type linkedBlobStatter struct {
	*blobStore
	repository distribution.Repository

	// linkPath allows one to control the repository blob link set to which
	// the blob store dispatches. This is required because manifest and layer
	// blobs have not yet been fully merged. At some point, this functionality
	// should be removed an the blob links folder should be merged.
	linkPath func(pm *pathMapper, name string, dgst digest.Digest) (string, error)
}

var _ distribution.BlobStatter = &linkedBlobStatter{}

func (lbs *linkedBlobStatter) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	blobLinkPath, err := lbs.linkPath(lbs.pm, lbs.repository.Name(), dgst)
	if err != nil {
		return distribution.Descriptor{}, err
	}

	target, err := lbs.blobStore.readlink(ctx, blobLinkPath)
	if err != nil {
		switch err := err.(type) {
		case driver.PathNotFoundError:
			return distribution.Descriptor{}, distribution.ErrBlobUnknown
		default:
			return distribution.Descriptor{}, err
		}

		// TODO(stevvooe): For backwards compatibility with data in "_layers", we
		// need to hit layerLinkPath, as well. Or, somehow migrate to the new path
		// layout.
	}

	if target != dgst {
		// Track when we are doing cross-digest domain lookups. ie, tarsum to sha256.
		context.GetLogger(ctx).Warnf("looking up blob with canonical target: %v -> %v", dgst, target)
	}

	// TODO(stevvooe): Look up repository local mediatype and replace that on
	// the returned descriptor.

	return lbs.blobStore.statter.Stat(ctx, target)
}

// blobLinkPath provides the path to the blob link, also known as layers.
func blobLinkPath(pm *pathMapper, name string, dgst digest.Digest) (string, error) {
	return pm.path(layerLinkPathSpec{name: name, digest: dgst})
}

// manifestRevisionLinkPath provides the path to the manifest revision link.
func manifestRevisionLinkPath(pm *pathMapper, name string, dgst digest.Digest) (string, error) {
	return pm.path(layerLinkPathSpec{name: name, digest: dgst})
}

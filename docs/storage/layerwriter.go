package storage

import (
	"fmt"
	"io"
	"path"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

var _ distribution.LayerUpload = &layerWriter{}

// layerWriter is used to control the various aspects of resumable
// layer upload. It implements the LayerUpload interface.
type layerWriter struct {
	layerStore *layerStore

	uuid      string
	startedAt time.Time

	// implementes io.WriteSeeker, io.ReaderFrom and io.Closer to satisy
	// LayerUpload Interface
	bufferedFileWriter
}

var _ distribution.LayerUpload = &layerWriter{}

// UUID returns the identifier for this upload.
func (lw *layerWriter) UUID() string {
	return lw.uuid
}

func (lw *layerWriter) StartedAt() time.Time {
	return lw.startedAt
}

// Finish marks the upload as completed, returning a valid handle to the
// uploaded layer. The final size and checksum are validated against the
// contents of the uploaded layer. The checksum should be provided in the
// format <algorithm>:<hex digest>.
func (lw *layerWriter) Finish(digest digest.Digest) (distribution.Layer, error) {
	ctxu.GetLogger(lw.layerStore.repository.ctx).Debug("(*layerWriter).Finish")

	if err := lw.bufferedFileWriter.Close(); err != nil {
		return nil, err
	}

	canonical, err := lw.validateLayer(digest)
	if err != nil {
		return nil, err
	}

	if err := lw.moveLayer(canonical); err != nil {
		// TODO(stevvooe): Cleanup?
		return nil, err
	}

	// Link the layer blob into the repository.
	if err := lw.linkLayer(canonical, digest); err != nil {
		return nil, err
	}

	if err := lw.removeResources(); err != nil {
		return nil, err
	}

	return lw.layerStore.Fetch(canonical)
}

// Cancel the layer upload process.
func (lw *layerWriter) Cancel() error {
	ctxu.GetLogger(lw.layerStore.repository.ctx).Debug("(*layerWriter).Cancel")
	if err := lw.removeResources(); err != nil {
		return err
	}

	lw.Close()
	return nil
}

// validateLayer checks the layer data against the digest, returning an error
// if it does not match. The canonical digest is returned.
func (lw *layerWriter) validateLayer(dgst digest.Digest) (digest.Digest, error) {
	digestVerifier, err := digest.NewDigestVerifier(dgst)
	if err != nil {
		return "", err
	}

	// TODO(stevvooe): Store resumable hash calculations in upload directory
	// in driver. Something like a file at path <uuid>/resumablehash/<offest>
	// with the hash state up to that point would be perfect. The hasher would
	// then only have to fetch the difference.

	// Read the file from the backend driver and validate it.
	fr, err := newFileReader(lw.bufferedFileWriter.driver, lw.path)
	if err != nil {
		return "", err
	}

	tr := io.TeeReader(fr, digestVerifier)

	// TODO(stevvooe): This is one of the places we need a Digester write
	// sink. Instead, its read driven. This might be okay.

	// Calculate an updated digest with the latest version.
	canonical, err := digest.FromReader(tr)
	if err != nil {
		return "", err
	}

	if !digestVerifier.Verified() {
		return "", distribution.ErrLayerInvalidDigest{
			Digest: dgst,
			Reason: fmt.Errorf("content does not match digest"),
		}
	}

	return canonical, nil
}

// moveLayer moves the data into its final, hash-qualified destination,
// identified by dgst. The layer should be validated before commencing the
// move.
func (lw *layerWriter) moveLayer(dgst digest.Digest) error {
	blobPath, err := lw.layerStore.repository.registry.pm.path(blobDataPathSpec{
		digest: dgst,
	})

	if err != nil {
		return err
	}

	// Check for existence
	if _, err := lw.driver.Stat(blobPath); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			break // ensure that it doesn't exist.
		default:
			return err
		}
	} else {
		// If the path exists, we can assume that the content has already
		// been uploaded, since the blob storage is content-addressable.
		// While it may be corrupted, detection of such corruption belongs
		// elsewhere.
		return nil
	}

	// If no data was received, we may not actually have a file on disk. Check
	// the size here and write a zero-length file to blobPath if this is the
	// case. For the most part, this should only ever happen with zero-length
	// tars.
	if _, err := lw.driver.Stat(lw.path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// HACK(stevvooe): This is slightly dangerous: if we verify above,
			// get a hash, then the underlying file is deleted, we risk moving
			// a zero-length blob into a nonzero-length blob location. To
			// prevent this horrid thing, we employ the hack of only allowing
			// to this happen for the zero tarsum.
			if dgst == digest.DigestSha256EmptyTar {
				return lw.driver.PutContent(blobPath, []byte{})
			}

			// We let this fail during the move below.
			logrus.
				WithField("upload.uuid", lw.UUID()).
				WithField("digest", dgst).Warnf("attempted to move zero-length content with non-zero digest")
		default:
			return err // unrelated error
		}
	}

	return lw.driver.Move(lw.path, blobPath)
}

// linkLayer links a valid, written layer blob into the registry under the
// named repository for the upload controller.
func (lw *layerWriter) linkLayer(canonical digest.Digest, aliases ...digest.Digest) error {
	dgsts := append([]digest.Digest{canonical}, aliases...)

	// Don't make duplicate links.
	seenDigests := make(map[digest.Digest]struct{}, len(dgsts))

	for _, dgst := range dgsts {
		if _, seen := seenDigests[dgst]; seen {
			continue
		}
		seenDigests[dgst] = struct{}{}

		layerLinkPath, err := lw.layerStore.repository.registry.pm.path(layerLinkPathSpec{
			name:   lw.layerStore.repository.Name(),
			digest: dgst,
		})

		if err != nil {
			return err
		}

		if err := lw.layerStore.repository.registry.driver.PutContent(layerLinkPath, []byte(canonical)); err != nil {
			return err
		}
	}

	return nil
}

// removeResources should clean up all resources associated with the upload
// instance. An error will be returned if the clean up cannot proceed. If the
// resources are already not present, no error will be returned.
func (lw *layerWriter) removeResources() error {
	dataPath, err := lw.layerStore.repository.registry.pm.path(uploadDataPathSpec{
		name: lw.layerStore.repository.Name(),
		uuid: lw.uuid,
	})

	if err != nil {
		return err
	}

	// Resolve and delete the containing directory, which should include any
	// upload related files.
	dirPath := path.Dir(dataPath)

	if err := lw.driver.Delete(dirPath); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			break // already gone!
		default:
			// This should be uncommon enough such that returning an error
			// should be okay. At this point, the upload should be mostly
			// complete, but perhaps the backend became unaccessible.
			logrus.Errorf("unable to delete layer upload resources %q: %v", dirPath, err)
			return err
		}
	}

	return nil
}

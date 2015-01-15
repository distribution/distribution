package storage

import (
	"io"
	"path"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storagedriver"
	"github.com/docker/docker/pkg/tarsum"
)

// layerUploadController is used to control the various aspects of resumable
// layer upload. It implements the LayerUpload interface.
type layerUploadController struct {
	layerStore *layerStore

	name      string
	uuid      string
	startedAt time.Time

	fileWriter
}

var _ LayerUpload = &layerUploadController{}

// Name of the repository under which the layer will be linked.
func (luc *layerUploadController) Name() string {
	return luc.name
}

// UUID returns the identifier for this upload.
func (luc *layerUploadController) UUID() string {
	return luc.uuid
}

func (luc *layerUploadController) StartedAt() time.Time {
	return luc.startedAt
}

// Finish marks the upload as completed, returning a valid handle to the
// uploaded layer. The final size and checksum are validated against the
// contents of the uploaded layer. The checksum should be provided in the
// format <algorithm>:<hex digest>.
func (luc *layerUploadController) Finish(digest digest.Digest) (Layer, error) {
	canonical, err := luc.validateLayer(digest)
	if err != nil {
		return nil, err
	}

	if err := luc.moveLayer(canonical); err != nil {
		// TODO(stevvooe): Cleanup?
		return nil, err
	}

	// Link the layer blob into the repository.
	if err := luc.linkLayer(canonical); err != nil {
		return nil, err
	}

	if err := luc.removeResources(); err != nil {
		return nil, err
	}

	return luc.layerStore.Fetch(luc.Name(), canonical)
}

// Cancel the layer upload process.
func (luc *layerUploadController) Cancel() error {
	if err := luc.removeResources(); err != nil {
		return err
	}

	luc.Close()
	return nil
}

// validateLayer checks the layer data against the digest, returning an error
// if it does not match. The canonical digest is returned.
func (luc *layerUploadController) validateLayer(dgst digest.Digest) (digest.Digest, error) {
	// First, check the incoming tarsum version of the digest.
	version, err := tarsum.GetVersionFromTarsum(dgst.String())
	if err != nil {
		return "", err
	}

	// TODO(stevvooe): Should we push this down into the digest type?
	switch version {
	case tarsum.Version1:
	default:
		// version 0 and dev, for now.
		return "", ErrLayerTarSumVersionUnsupported
	}

	digestVerifier := digest.NewDigestVerifier(dgst)

	// TODO(stevvooe): Store resumable hash calculations in upload directory
	// in driver. Something like a file at path <uuid>/resumablehash/<offest>
	// with the hash state up to that point would be perfect. The hasher would
	// then only have to fetch the difference.

	// Read the file from the backend driver and validate it.
	fr, err := newFileReader(luc.fileWriter.driver, luc.path)
	if err != nil {
		return "", err
	}

	tr := io.TeeReader(fr, digestVerifier)

	// TODO(stevvooe): This is one of the places we need a Digester write
	// sink. Instead, its read driven. This might be okay.

	// Calculate an updated digest with the latest version.
	canonical, err := digest.FromTarArchive(tr)
	if err != nil {
		return "", err
	}

	if !digestVerifier.Verified() {
		return "", ErrLayerInvalidDigest{Digest: dgst}
	}

	return canonical, nil
}

// moveLayer moves the data into its final, hash-qualified destination,
// identified by dgst. The layer should be validated before commencing the
// move.
func (luc *layerUploadController) moveLayer(dgst digest.Digest) error {
	blobPath, err := luc.layerStore.pathMapper.path(blobDataPathSpec{
		digest: dgst,
	})

	if err != nil {
		return err
	}

	// Check for existence
	if _, err := luc.layerStore.driver.Stat(blobPath); err != nil {
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

	return luc.driver.Move(luc.path, blobPath)
}

// linkLayer links a valid, written layer blob into the registry under the
// named repository for the upload controller.
func (luc *layerUploadController) linkLayer(digest digest.Digest) error {
	layerLinkPath, err := luc.layerStore.pathMapper.path(layerLinkPathSpec{
		name:   luc.Name(),
		digest: digest,
	})

	if err != nil {
		return err
	}

	return luc.layerStore.driver.PutContent(layerLinkPath, []byte(digest))
}

// removeResources should clean up all resources associated with the upload
// instance. An error will be returned if the clean up cannot proceed. If the
// resources are already not present, no error will be returned.
func (luc *layerUploadController) removeResources() error {
	dataPath, err := luc.layerStore.pathMapper.path(uploadDataPathSpec{
		name: luc.name,
		uuid: luc.uuid,
	})

	if err != nil {
		return err
	}

	// Resolve and delete the containing directory, which should include any
	// upload related files.
	dirPath := path.Dir(dataPath)

	if err := luc.driver.Delete(dirPath); err != nil {
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

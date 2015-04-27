package storage

import (
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
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

	uuid              string
	startedAt         time.Time
	resumableDigester digest.ResumableDigester

	// implementes io.WriteSeeker, io.ReaderFrom and io.Closer to satisfy
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
func (lw *layerWriter) Finish(dgst digest.Digest) (distribution.Layer, error) {
	ctxu.GetLogger(lw.layerStore.repository.ctx).Debug("(*layerWriter).Finish")

	if err := lw.bufferedFileWriter.Close(); err != nil {
		return nil, err
	}

	var (
		canonical digest.Digest
		err       error
	)

	// HACK(stevvooe): To deal with s3's lack of consistency, attempt to retry
	// validation on failure. Three attempts are made, backing off
	// retries*100ms each time.
	for retries := 0; ; retries++ {
		canonical, err = lw.validateLayer(dgst)
		if err == nil {
			break
		}

		ctxu.GetLoggerWithField(lw.layerStore.repository.ctx, "retries", retries).
			Errorf("error validating layer: %v", err)

		if retries < 3 {
			time.Sleep(100 * time.Millisecond * time.Duration(retries+1))
			continue
		}

		return nil, err

	}

	if err := lw.moveLayer(canonical); err != nil {
		// TODO(stevvooe): Cleanup?
		return nil, err
	}

	// Link the layer blob into the repository.
	if err := lw.linkLayer(canonical, dgst); err != nil {
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

func (lw *layerWriter) Write(p []byte) (int, error) {
	if lw.resumableDigester == nil {
		return lw.bufferedFileWriter.Write(p)
	}

	// Ensure that the current write offset matches how many bytes have been
	// written to the digester. If not, we need to update the digest state to
	// match the current write position.
	if err := lw.resumeHashAt(lw.offset); err != nil {
		return 0, err
	}

	return io.MultiWriter(&lw.bufferedFileWriter, lw.resumableDigester).Write(p)
}

func (lw *layerWriter) ReadFrom(r io.Reader) (n int64, err error) {
	if lw.resumableDigester == nil {
		return lw.bufferedFileWriter.ReadFrom(r)
	}

	// Ensure that the current write offset matches how many bytes have been
	// written to the digester. If not, we need to update the digest state to
	// match the current write position.
	if err := lw.resumeHashAt(lw.offset); err != nil {
		return 0, err
	}

	return lw.bufferedFileWriter.ReadFrom(io.TeeReader(r, lw.resumableDigester))
}

func (lw *layerWriter) Close() error {
	if lw.err != nil {
		return lw.err
	}

	if lw.resumableDigester != nil {
		if err := lw.storeHashState(); err != nil {
			return err
		}
	}

	return lw.bufferedFileWriter.Close()
}

type hashStateEntry struct {
	offset int64
	path   string
}

// getStoredHashStates returns a slice of hashStateEntries for this upload.
func (lw *layerWriter) getStoredHashStates() ([]hashStateEntry, error) {
	uploadHashStatePathPrefix, err := lw.layerStore.repository.pm.path(uploadHashStatePathSpec{
		name: lw.layerStore.repository.Name(),
		uuid: lw.uuid,
		alg:  lw.resumableDigester.Digest().Algorithm(),
		list: true,
	})
	if err != nil {
		return nil, err
	}

	paths, err := lw.driver.List(uploadHashStatePathPrefix)
	if err != nil {
		if _, ok := err.(storagedriver.PathNotFoundError); !ok {
			return nil, err
		}
		// Treat PathNotFoundError as no entries.
		paths = nil
	}

	hashStateEntries := make([]hashStateEntry, 0, len(paths))

	for _, p := range paths {
		pathSuffix := path.Base(p)
		// The suffix should be the offset.
		offset, err := strconv.ParseInt(pathSuffix, 0, 64)
		if err != nil {
			logrus.Errorf("unable to parse offset from upload state path %q: %s", p, err)
		}

		hashStateEntries = append(hashStateEntries, hashStateEntry{offset: offset, path: p})
	}

	return hashStateEntries, nil
}

// resumeHashAt attempts to restore the state of the internal hash function
// by loading the most recent saved hash state less than or equal to the given
// offset. Any unhashed bytes remaining less than the given offset are hashed
// from the content uploaded so far.
func (lw *layerWriter) resumeHashAt(offset int64) error {
	if offset < 0 {
		return fmt.Errorf("cannot resume hash at negative offset: %d", offset)
	}

	if offset == int64(lw.resumableDigester.Len()) {
		// State of digester is already at the requested offset.
		return nil
	}

	// List hash states from storage backend.
	var hashStateMatch hashStateEntry
	hashStates, err := lw.getStoredHashStates()
	if err != nil {
		return fmt.Errorf("unable to get stored hash states with offset %d: %s", offset, err)
	}

	// Find the highest stored hashState with offset less than or equal to
	// the requested offset.
	for _, hashState := range hashStates {
		if hashState.offset == offset {
			hashStateMatch = hashState
			break // Found an exact offset match.
		} else if hashState.offset < offset && hashState.offset > hashStateMatch.offset {
			// This offset is closer to the requested offset.
			hashStateMatch = hashState
		} else if hashState.offset > offset {
			// Remove any stored hash state with offsets higher than this one
			// as writes to this resumed hasher will make those invalid. This
			// is probably okay to skip for now since we don't expect anyone to
			// use the API in this way. For that reason, we don't treat an
			// an error here as a fatal error, but only log it.
			if err := lw.driver.Delete(hashState.path); err != nil {
				logrus.Errorf("unable to delete stale hash state %q: %s", hashState.path, err)
			}
		}
	}

	if hashStateMatch.offset == 0 {
		// No need to load any state, just reset the hasher.
		lw.resumableDigester.Reset()
	} else {
		storedState, err := lw.driver.GetContent(hashStateMatch.path)
		if err != nil {
			return err
		}

		if err = lw.resumableDigester.Restore(storedState); err != nil {
			return err
		}
	}

	// Mind the gap.
	if gapLen := offset - int64(lw.resumableDigester.Len()); gapLen > 0 {
		// Need to read content from the upload to catch up to the desired
		// offset.
		fr, err := newFileReader(lw.driver, lw.path)
		if err != nil {
			return err
		}

		if _, err = fr.Seek(int64(lw.resumableDigester.Len()), os.SEEK_SET); err != nil {
			return fmt.Errorf("unable to seek to layer reader offset %d: %s", lw.resumableDigester.Len(), err)
		}

		if _, err := io.CopyN(lw.resumableDigester, fr, gapLen); err != nil {
			return err
		}
	}

	return nil
}

func (lw *layerWriter) storeHashState() error {
	uploadHashStatePath, err := lw.layerStore.repository.pm.path(uploadHashStatePathSpec{
		name:   lw.layerStore.repository.Name(),
		uuid:   lw.uuid,
		alg:    lw.resumableDigester.Digest().Algorithm(),
		offset: int64(lw.resumableDigester.Len()),
	})
	if err != nil {
		return err
	}

	hashState, err := lw.resumableDigester.State()
	if err != nil {
		return err
	}

	return lw.driver.PutContent(uploadHashStatePath, hashState)
}

// validateLayer checks the layer data against the digest, returning an error
// if it does not match. The canonical digest is returned.
func (lw *layerWriter) validateLayer(dgst digest.Digest) (digest.Digest, error) {
	var (
		verified, fullHash bool
		canonical          digest.Digest
	)

	if lw.resumableDigester != nil {
		// Restore the hasher state to the end of the upload.
		if err := lw.resumeHashAt(lw.size); err != nil {
			return "", err
		}

		canonical = lw.resumableDigester.Digest()

		if canonical.Algorithm() == dgst.Algorithm() {
			// Common case: client and server prefer the same canonical digest
			// algorithm - currently SHA256.
			verified = dgst == canonical
		} else {
			// The client wants to use a different digest algorithm. They'll just
			// have to be patient and wait for us to download and re-hash the
			// uploaded content using that digest algorithm.
			fullHash = true
		}
	} else {
		// Not using resumable digests, so we need to hash the entire layer.
		fullHash = true
	}

	if fullHash {
		digester := digest.NewCanonicalDigester()

		digestVerifier, err := digest.NewDigestVerifier(dgst)
		if err != nil {
			return "", err
		}

		// Read the file from the backend driver and validate it.
		fr, err := newFileReader(lw.bufferedFileWriter.driver, lw.path)
		if err != nil {
			return "", err
		}

		tr := io.TeeReader(fr, digester)

		if _, err = io.Copy(digestVerifier, tr); err != nil {
			return "", err
		}

		canonical = digester.Digest()
		verified = digestVerifier.Verified()
	}

	if !verified {
		ctxu.GetLoggerWithField(lw.layerStore.repository.ctx, "canonical", dgst).
			Errorf("canonical digest does match provided digest")
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
	blobPath, err := lw.layerStore.repository.pm.path(blobDataPathSpec{
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

		layerLinkPath, err := lw.layerStore.repository.pm.path(layerLinkPathSpec{
			name:   lw.layerStore.repository.Name(),
			digest: dgst,
		})

		if err != nil {
			return err
		}

		if err := lw.layerStore.repository.driver.PutContent(layerLinkPath, []byte(canonical)); err != nil {
			return err
		}
	}

	return nil
}

// removeResources should clean up all resources associated with the upload
// instance. An error will be returned if the clean up cannot proceed. If the
// resources are already not present, no error will be returned.
func (lw *layerWriter) removeResources() error {
	dataPath, err := lw.layerStore.repository.pm.path(uploadDataPathSpec{
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

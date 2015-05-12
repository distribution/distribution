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
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

// layerWriter is used to control the various aspects of resumable
// layer upload. It implements the LayerUpload interface.
type blobWriter struct {
	blobStore *linkedBlobStore

	id                string
	startedAt         time.Time
	resumableDigester digest.ResumableDigester

	// implementes io.WriteSeeker, io.ReaderFrom and io.Closer to satisfy
	// LayerUpload Interface
	bufferedFileWriter
}

var _ distribution.BlobWriter = &blobWriter{}

// ID returns the identifier for this upload.
func (bw *blobWriter) ID() string {
	return bw.id
}

func (bw *blobWriter) StartedAt() time.Time {
	return bw.startedAt
}

// Commit marks the upload as completed, returning a valid descriptor. The
// final size and digest are checked against the first descriptor provided.
func (bw *blobWriter) Commit(ctx context.Context, desc distribution.Descriptor) (distribution.Descriptor, error) {
	context.GetLogger(ctx).Debug("(*blobWriter).Commit")

	if err := bw.bufferedFileWriter.Close(); err != nil {
		return distribution.Descriptor{}, err
	}

	canonical, err := bw.validateBlob(ctx, desc)
	if err != nil {
		return distribution.Descriptor{}, err
	}

	if err := bw.moveBlob(ctx, canonical); err != nil {
		return distribution.Descriptor{}, err
	}

	if err := bw.blobStore.linkBlob(ctx, canonical, desc.Digest); err != nil {
		return distribution.Descriptor{}, err
	}

	if err := bw.removeResources(ctx); err != nil {
		return distribution.Descriptor{}, err
	}

	return canonical, nil
}

// Rollback the blob upload process, releasing any resources associated with
// the writer and canceling the operation.
func (bw *blobWriter) Cancel(ctx context.Context) error {
	context.GetLogger(ctx).Debug("(*blobWriter).Rollback")
	if err := bw.removeResources(ctx); err != nil {
		return err
	}

	bw.Close()
	return nil
}

func (bw *blobWriter) Write(p []byte) (int, error) {
	if bw.resumableDigester == nil {
		return bw.bufferedFileWriter.Write(p)
	}

	// Ensure that the current write offset matches how many bytes have been
	// written to the digester. If not, we need to update the digest state to
	// match the current write position.
	if err := bw.resumeHashAt(bw.blobStore.ctx, bw.offset); err != nil {
		return 0, err
	}

	return io.MultiWriter(&bw.bufferedFileWriter, bw.resumableDigester).Write(p)
}

func (bw *blobWriter) ReadFrom(r io.Reader) (n int64, err error) {
	if bw.resumableDigester == nil {
		return bw.bufferedFileWriter.ReadFrom(r)
	}

	// Ensure that the current write offset matches how many bytes have been
	// written to the digester. If not, we need to update the digest state to
	// match the current write position.
	if err := bw.resumeHashAt(bw.blobStore.ctx, bw.offset); err != nil {
		return 0, err
	}

	return bw.bufferedFileWriter.ReadFrom(io.TeeReader(r, bw.resumableDigester))
}

func (bw *blobWriter) Close() error {
	if bw.err != nil {
		return bw.err
	}

	if bw.resumableDigester != nil {
		if err := bw.storeHashState(bw.blobStore.ctx); err != nil {
			return err
		}
	}

	return bw.bufferedFileWriter.Close()
}

// validateBlob checks the data against the digest, returning an error if it
// does not match. The canonical descriptor is returned.
func (bw *blobWriter) validateBlob(ctx context.Context, desc distribution.Descriptor) (distribution.Descriptor, error) {
	var (
		verified, fullHash bool
		canonical          digest.Digest
	)

	if desc.Digest == "" {
		// if no descriptors are provided, we have nothing to validate
		// against. We don't really want to support this for the registry.
		return distribution.Descriptor{}, distribution.ErrBlobInvalidDigest{
			Reason: fmt.Errorf("cannot validate against empty digest"),
		}
	}

	// Stat the on disk file
	if fi, err := bw.bufferedFileWriter.driver.Stat(ctx, bw.path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// NOTE(stevvooe): We really don't care if the file is
			// not actually present for the reader. We now assume
			// that the desc length is zero.
			desc.Length = 0
		default:
			// Any other error we want propagated up the stack.
			return distribution.Descriptor{}, err
		}
	} else {
		if fi.IsDir() {
			return distribution.Descriptor{}, fmt.Errorf("unexpected directory at upload location %q", bw.path)
		}

		bw.size = fi.Size()
	}

	if desc.Length > 0 {
		if desc.Length != bw.size {
			return distribution.Descriptor{}, distribution.ErrBlobInvalidLength
		}
	} else {
		// if provided 0 or negative length, we can assume caller doesn't know or
		// care about length.
		desc.Length = bw.size
	}

	if bw.resumableDigester != nil {
		// Restore the hasher state to the end of the upload.
		if err := bw.resumeHashAt(ctx, bw.size); err != nil {
			return distribution.Descriptor{}, err
		}

		canonical = bw.resumableDigester.Digest()

		if canonical.Algorithm() == desc.Digest.Algorithm() {
			// Common case: client and server prefer the same canonical digest
			// algorithm - currently SHA256.
			verified = desc.Digest == canonical
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

		digestVerifier, err := digest.NewDigestVerifier(desc.Digest)
		if err != nil {
			return distribution.Descriptor{}, err
		}

		// Read the file from the backend driver and validate it.
		fr, err := newFileReader(ctx, bw.bufferedFileWriter.driver, bw.path, desc.Length)
		if err != nil {
			return distribution.Descriptor{}, err
		}

		tr := io.TeeReader(fr, digester)

		if _, err := io.Copy(digestVerifier, tr); err != nil {
			return distribution.Descriptor{}, err
		}

		canonical = digester.Digest()
		verified = digestVerifier.Verified()
	}

	if !verified {
		context.GetLoggerWithFields(ctx,
			map[string]interface{}{
				"canonical": canonical,
				"provided":  desc.Digest,
			}, "canonical", "provided").
			Errorf("canonical digest does match provided digest")
		return distribution.Descriptor{}, distribution.ErrBlobInvalidDigest{
			Digest: desc.Digest,
			Reason: fmt.Errorf("content does not match digest"),
		}
	}

	// update desc with canonical hash
	desc.Digest = canonical

	if desc.MediaType == "" {
		desc.MediaType = "application/octet-stream"
	}

	return desc, nil
}

// moveBlob moves the data into its final, hash-qualified destination,
// identified by dgst. The layer should be validated before commencing the
// move.
func (bw *blobWriter) moveBlob(ctx context.Context, desc distribution.Descriptor) error {
	blobPath, err := bw.blobStore.pm.path(blobDataPathSpec{
		digest: desc.Digest,
	})

	if err != nil {
		return err
	}

	// Check for existence
	if _, err := bw.blobStore.driver.Stat(ctx, blobPath); err != nil {
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
	if _, err := bw.blobStore.driver.Stat(ctx, bw.path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// HACK(stevvooe): This is slightly dangerous: if we verify above,
			// get a hash, then the underlying file is deleted, we risk moving
			// a zero-length blob into a nonzero-length blob location. To
			// prevent this horrid thing, we employ the hack of only allowing
			// to this happen for the zero tarsum.
			if desc.Digest == digest.DigestSha256EmptyTar {
				return bw.blobStore.driver.PutContent(ctx, blobPath, []byte{})
			}

			// We let this fail during the move below.
			logrus.
				WithField("upload.id", bw.ID()).
				WithField("digest", desc.Digest).Warnf("attempted to move zero-length content with non-zero digest")
		default:
			return err // unrelated error
		}
	}

	// TODO(stevvooe): We should also write the mediatype when executing this move.

	return bw.blobStore.driver.Move(ctx, bw.path, blobPath)
}

type hashStateEntry struct {
	offset int64
	path   string
}

// getStoredHashStates returns a slice of hashStateEntries for this upload.
func (bw *blobWriter) getStoredHashStates(ctx context.Context) ([]hashStateEntry, error) {
	uploadHashStatePathPrefix, err := bw.blobStore.pm.path(uploadHashStatePathSpec{
		name: bw.blobStore.repository.Name(),
		id:   bw.id,
		alg:  bw.resumableDigester.Digest().Algorithm(),
		list: true,
	})
	if err != nil {
		return nil, err
	}

	paths, err := bw.blobStore.driver.List(ctx, uploadHashStatePathPrefix)
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
func (bw *blobWriter) resumeHashAt(ctx context.Context, offset int64) error {
	if offset < 0 {
		return fmt.Errorf("cannot resume hash at negative offset: %d", offset)
	}

	if offset == int64(bw.resumableDigester.Len()) {
		// State of digester is already at the requested offset.
		return nil
	}

	// List hash states from storage backend.
	var hashStateMatch hashStateEntry
	hashStates, err := bw.getStoredHashStates(ctx)
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
			if err := bw.driver.Delete(ctx, hashState.path); err != nil {
				logrus.Errorf("unable to delete stale hash state %q: %s", hashState.path, err)
			}
		}
	}

	if hashStateMatch.offset == 0 {
		// No need to load any state, just reset the hasher.
		bw.resumableDigester.Reset()
	} else {
		storedState, err := bw.driver.GetContent(ctx, hashStateMatch.path)
		if err != nil {
			return err
		}

		if err = bw.resumableDigester.Restore(storedState); err != nil {
			return err
		}
	}

	// Mind the gap.
	if gapLen := offset - int64(bw.resumableDigester.Len()); gapLen > 0 {
		// Need to read content from the upload to catch up to the desired offset.
		fr, err := newFileReader(ctx, bw.driver, bw.path, bw.size)
		if err != nil {
			return err
		}

		if _, err = fr.Seek(int64(bw.resumableDigester.Len()), os.SEEK_SET); err != nil {
			return fmt.Errorf("unable to seek to layer reader offset %d: %s", bw.resumableDigester.Len(), err)
		}

		if _, err := io.CopyN(bw.resumableDigester, fr, gapLen); err != nil {
			return err
		}
	}

	return nil
}

func (bw *blobWriter) storeHashState(ctx context.Context) error {
	uploadHashStatePath, err := bw.blobStore.pm.path(uploadHashStatePathSpec{
		name:   bw.blobStore.repository.Name(),
		id:     bw.id,
		alg:    bw.resumableDigester.Digest().Algorithm(),
		offset: int64(bw.resumableDigester.Len()),
	})
	if err != nil {
		return err
	}

	hashState, err := bw.resumableDigester.State()
	if err != nil {
		return err
	}

	return bw.driver.PutContent(ctx, uploadHashStatePath, hashState)
}

// removeResources should clean up all resources associated with the upload
// instance. An error will be returned if the clean up cannot proceed. If the
// resources are already not present, no error will be returned.
func (bw *blobWriter) removeResources(ctx context.Context) error {
	dataPath, err := bw.blobStore.pm.path(uploadDataPathSpec{
		name: bw.blobStore.repository.Name(),
		id:   bw.id,
	})

	if err != nil {
		return err
	}

	// Resolve and delete the containing directory, which should include any
	// upload related files.
	dirPath := path.Dir(dataPath)
	if err := bw.blobStore.driver.Delete(ctx, dirPath); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			break // already gone!
		default:
			// This should be uncommon enough such that returning an error
			// should be okay. At this point, the upload should be mostly
			// complete, but perhaps the backend became unaccessible.
			context.GetLogger(ctx).Errorf("unable to delete layer upload resources %q: %v", dirPath, err)
			return err
		}
	}

	return nil
}

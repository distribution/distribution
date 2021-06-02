package storage

import (
	"context"
	"errors"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/distribution/distribution/v3"
	dcontext "github.com/distribution/distribution/v3/context"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/opencontainers/go-digest"
	"github.com/sirupsen/logrus"
)

var (
	errResumableDigestNotAvailable = errors.New("resumable digest not available")
)

const (
	// digestSha256Empty is the canonical sha256 digest of empty data
	digestSha256Empty = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// blobWriter is used to control the various aspects of resumable
// blob upload.
type blobWriter struct {
	ctx       context.Context
	blobStore *linkedBlobStore

	id        string
	startedAt time.Time
	digester  digest.Digester
	written   int64 // track the write to digester

	fileWriter storagedriver.FileWriter
	driver     storagedriver.StorageDriver
	path       string

	resumableDigestEnabled bool
	committed              bool
	cancelled              bool
	mutex                  *sync.Mutex
	finished               chan struct{}
	refCount               int32
	lastError              error
	finishedClosed         bool
	desc                   *distribution.Descriptor
	descNotify             *sync.Cond
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
	bw.mutex.Lock()
	defer bw.mutex.Unlock()
	defer bw.closeChannel()
	res, err := bw.doCommit(ctx, desc)
	if err != nil {
		bw.lastError = err
	}
	return res, err
}

func (bw *blobWriter) closeChannel() {
	if bw.finishedClosed {
		close(bw.finished)
		bw.finishedClosed = true
	}
}

func (bw *blobWriter) doCommit(ctx context.Context, desc distribution.Descriptor) (distribution.Descriptor, error) {
	dcontext.GetLogger(ctx).Debug("(*blobWriter).Commit")

	if err := bw.fileWriter.Commit(); err != nil {
		return distribution.Descriptor{}, err
	}

	bw.Close()
	desc.Size = bw.Size()

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

	if err := bw.ReleaseResources(); err != nil {
		return distribution.Descriptor{}, err
	}

	err = bw.blobStore.blobAccessController.SetDescriptor(ctx, canonical.Digest, canonical)
	if err != nil {
		return distribution.Descriptor{}, err
	}

	bw.committed = true

	return canonical, nil
}

// Cancel the blob upload process, releasing any resources associated with
// the writer and canceling the operation.
func (bw *blobWriter) Cancel(ctx context.Context) error {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()
	defer bw.closeChannel()
	defer bw.descNotify.Broadcast()

	bw.cancelled = true
	dcontext.GetLogger(ctx).Debug("(*blobWriter).Cancel")
	if err := bw.fileWriter.Cancel(); err != nil {
		return err
	}

	if err := bw.Close(); err != nil {
		dcontext.GetLogger(ctx).Errorf("error closing blobwriter: %s", err)
	}

	return bw.ReleaseResources()
}

func (bw *blobWriter) CancelWithError(ctx context.Context, err error) error {
	bw.lastError = err
	return bw.Cancel(ctx)
}

func (bw *blobWriter) Size() int64 {
	return bw.fileWriter.Size()
}

func (bw *blobWriter) Write(p []byte) (int, error) {
	// Ensure that the current write offset matches how many bytes have been
	// written to the digester. If not, we need to update the digest state to
	// match the current write position.
	if err := bw.resumeDigest(bw.blobStore.ctx); err != nil && err != errResumableDigestNotAvailable {
		return 0, err
	}

	_, err := bw.fileWriter.Write(p)
	if err != nil {
		return 0, err
	}

	n, err := bw.digester.Hash().Write(p)
	bw.written += int64(n)

	return n, err
}

func (bw *blobWriter) ReadFrom(r io.Reader) (n int64, err error) {
	// Ensure that the current write offset matches how many bytes have been
	// written to the digester. If not, we need to update the digest state to
	// match the current write position.
	if err := bw.resumeDigest(bw.blobStore.ctx); err != nil && err != errResumableDigestNotAvailable {
		return 0, err
	}

	// Using a TeeReader instead of MultiWriter ensures Copy returns
	// the amount written to the digester as well as ensuring that we
	// write to the fileWriter first
	tee := io.TeeReader(r, bw.fileWriter)
	nn, err := io.Copy(bw.digester.Hash(), tee)
	bw.written += nn

	return nn, err
}

func (bw *blobWriter) Close() error {
	if bw.committed {
		return errors.New("blobwriter close after commit")
	}

	if err := bw.storeHashState(bw.blobStore.ctx); err != nil && err != errResumableDigestNotAvailable {
		return err
	}

	return bw.fileWriter.Close()
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

	var size int64

	// Stat the on disk file
	if fi, err := bw.driver.Stat(ctx, bw.path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// NOTE(stevvooe): We really don't care if the file is
			// not actually present for the reader. We now assume
			// that the desc length is zero.
			desc.Size = 0
		default:
			// Any other error we want propagated up the stack.
			return distribution.Descriptor{}, err
		}
	} else {
		if fi.IsDir() {
			return distribution.Descriptor{}, fmt.Errorf("unexpected directory at upload location %q", bw.path)
		}

		size = fi.Size()
	}

	if desc.Size > 0 {
		if desc.Size != size {
			return distribution.Descriptor{}, distribution.ErrBlobInvalidLength
		}
	} else {
		// if provided 0 or negative length, we can assume caller doesn't know or
		// care about length.
		desc.Size = size
	}

	// TODO(stevvooe): This section is very meandering. Need to be broken down
	// to be a lot more clear.

	if err := bw.resumeDigest(ctx); err == nil {
		canonical = bw.digester.Digest()

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
	} else if err == errResumableDigestNotAvailable {
		// Not using resumable digests, so we need to hash the entire layer.
		fullHash = true
	} else {
		return distribution.Descriptor{}, err
	}

	if fullHash {
		// a fantastic optimization: if the the written data and the size are
		// the same, we don't need to read the data from the backend. This is
		// because we've written the entire file in the lifecycle of the
		// current instance.
		if bw.written == size && digest.Canonical == desc.Digest.Algorithm() {
			canonical = bw.digester.Digest()
			verified = desc.Digest == canonical
		}

		// If the check based on size fails, we fall back to the slowest of
		// paths. We may be able to make the size-based check a stronger
		// guarantee, so this may be defensive.
		if !verified {
			digester := digest.Canonical.Digester()
			verifier := desc.Digest.Verifier()

			// Read the file from the backend driver and validate it.
			fr, err := newFileReader(ctx, bw.driver, bw.path, desc.Size)
			if err != nil {
				return distribution.Descriptor{}, err
			}
			defer fr.Close()

			tr := io.TeeReader(fr, digester.Hash())

			if _, err := io.Copy(verifier, tr); err != nil {
				return distribution.Descriptor{}, err
			}

			canonical = digester.Digest()
			verified = verifier.Verified()
		}
	}

	if !verified {
		dcontext.GetLoggerWithFields(ctx,
			map[interface{}]interface{}{
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
	blobPath, err := pathFor(blobDataPathSpec{
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
	// blobs.
	if _, err := bw.blobStore.driver.Stat(ctx, bw.path); err != nil {
		switch err := err.(type) {
		case storagedriver.PathNotFoundError:
			// HACK(stevvooe): This is slightly dangerous: if we verify above,
			// get a hash, then the underlying file is deleted, we risk moving
			// a zero-length blob into a nonzero-length blob location. To
			// prevent this horrid thing, we employ the hack of only allowing
			// to this happen for the digest of an empty blob.
			if desc.Digest == digestSha256Empty {
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

func (bw *blobWriter) ReleaseResources() error {
	if atomic.AddInt32(&bw.refCount, -1) == 0 {
		return bw.removeResources(bw.ctx)
	}
	return nil
}

// removeResources should clean up all resources associated with the upload
// instance. An error will be returned if the clean up cannot proceed. If the
// resources are already not present, no error will be returned.
func (bw *blobWriter) removeResources(ctx context.Context) error {
	dataPath, err := pathFor(uploadDataPathSpec{
		name: bw.blobStore.repository.Named().Name(),
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
			dcontext.GetLogger(ctx).Errorf("unable to delete layer upload resources %q: %v", dirPath, err)
			return err
		}
	}

	return nil
}

type blobWriterReader struct {
	path       string
	events     chan fsnotify.Event
	blobWriter *blobWriter
	driver     storagedriver.StorageDriver
	ctx        context.Context
	reader     io.ReadCloser
	watcher    io.Closer
	finished   chan struct{}
}

func (reader *blobWriterReader) GetDescriptor() (distribution.Descriptor, error) {
	reader.blobWriter.mutex.Lock()
	defer reader.blobWriter.mutex.Unlock()
	if reader.blobWriter.desc == nil {
		reader.blobWriter.descNotify.Wait()
	}
	if reader.blobWriter.lastError != nil {
		return distribution.Descriptor{}, reader.blobWriter.lastError
	}
	return *reader.blobWriter.desc, nil
}

func (reader *blobWriterReader) Read(buff []byte) (int, error) {
	reader.blobWriter.mutex.Lock()
	defer reader.blobWriter.mutex.Unlock()
	for true {
		inProgress := reader.blobWriter.IsInProgress()
		reader.blobWriter.mutex.Unlock()
		count, err := reader.reader.Read(buff)
		reader.blobWriter.mutex.Lock()
		if err != nil && err != io.EOF {
			return count, err
		}
		if count != 0 {
			return count, nil
		}
		if err == io.EOF && !inProgress {
			if reader.blobWriter.lastError != nil {
				err = reader.blobWriter.lastError
			}
			return count, err
		}
		// At this point we have returned all the data we have available, we need to wait for
		// further data to arrive or the download to be interrupted.
		reader.blobWriter.mutex.Unlock()
		select {
		case <-reader.events:
		case <-reader.finished:
		case <-time.After(60 * time.Second):
			logrus.Debugf("Timed out waiting for events ...")
		}
		reader.blobWriter.mutex.Lock()
	}
	return 0, io.EOF

}

func (reader *blobWriterReader) Close() error {
	return reader.blobWriter.ReleaseResources()
}

var _ io.ReadCloser = &blobWriterReader{}
var _ ReadableWriter = &blobWriter{}

type BlobReader interface {
	io.ReadCloser
	GetDescriptor() (distribution.Descriptor, error)
}

type ReadableWriter interface {
	distribution.BlobWriter
	Reader() (BlobReader, error)

	// CancelWithError does the same as Cancel plus delegates the error to any readers that are reading the writer
	CancelWithError(ctx context.Context, err error) error
	IsInProgress() bool
	SetDescriptor(desc distribution.Descriptor)
}

func (bw *blobWriter) IsInProgress() bool {
	return !bw.cancelled && !bw.committed
}

func (bw *blobWriter) SetDescriptor(desc distribution.Descriptor) {
	bw.mutex.Lock()
	bw.desc = &desc
	bw.descNotify.Broadcast()
	bw.mutex.Unlock()
}

func (bw *blobWriter) Reader() (BlobReader, error) {
	bw.mutex.Lock()
	defer bw.mutex.Unlock()
	if bw.refCount == 0 {
		return nil, io.EOF //just finished
	}
	atomic.AddInt32(&bw.refCount, +1)
	watcher, events, err := bw.driver.Watch(bw.ctx, bw.path)
	if err != nil {
		return nil, err
	}

	reader, err := bw.driver.Reader(bw.ctx, bw.path, 0)
	if err != nil {
		return nil, err
	}
	wReader := blobWriterReader{
		driver:     bw.driver,
		ctx:        bw.ctx,
		path:       bw.path,
		blobWriter: bw,
		watcher:    watcher,
		finished:   bw.finished,
		events:     events,
		reader:     reader,
	}

	return &wReader, nil
}

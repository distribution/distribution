package proxy

import (
	"context"
	"errors"
	"github.com/distribution/distribution/v3/registry/storage"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/distribution/distribution/v3"
	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/proxy/scheduler"
	"github.com/opencontainers/go-digest"
)

type proxyBlobStore struct {
	localStore     distribution.BlobStore
	remoteStore    distribution.BlobService
	scheduler      *scheduler.TTLExpirationScheduler
	repositoryName reference.Named
	authChallenger authChallenger
}

var _ distribution.BlobStore = &proxyBlobStore{}

type BlobFetch struct {
	completeCond   *sync.Cond
	readableWriter storage.ReadableWriter
	mutex          sync.Mutex
}

func newBlobFetch(w storage.ReadableWriter) *BlobFetch {
	res := &BlobFetch{
		readableWriter: w,
	}
	res.completeCond = sync.NewCond(&res.mutex)
	return res
}

// inflight tracks currently downloading blobs
var inflight = make(map[digest.Digest]*BlobFetch)

// mu protects inflight
var mu sync.Mutex

func setResponseHeaders(w http.ResponseWriter, length int64, mediaType string, digest digest.Digest) {
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest.String())
	w.Header().Set("Etag", digest.String())
}

func (pbs *proxyBlobStore) copyContent(ctx context.Context, dgst digest.Digest, writer io.Writer, desc distribution.Descriptor) error {
	remoteReader, err := pbs.remoteStore.Open(ctx, dgst)
	if err != nil {
		return err
	}

	defer remoteReader.Close()

	_, err = io.CopyN(writer, remoteReader, desc.Size)
	if err != nil {
		return err
	}

	proxyMetrics.BlobPush(uint64(desc.Size))

	return nil
}

func (pbs *proxyBlobStore) doServeFromLocalStore(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	_, err := pbs.localStore.Stat(ctx, dgst)
	if err != nil {
		return err
	}

	return pbs.localStore.ServeBlob(ctx, w, r, dgst)

}

func (pbs *proxyBlobStore) IsPresentLocally(ctx context.Context, dgst digest.Digest) bool {
	_, err := pbs.localStore.Stat(ctx, dgst)
	// Stat can report a zero sized file here if it's checked between creation
	// and population. Continue as if it did not exist.
	return err == nil
}

func (pbs *proxyBlobStore) fetchFromRemote(dgst digest.Digest, ctx context.Context, fetcher *BlobFetch) {
	err := pbs.doFetchFromRemote(dgst, context.Background(), fetcher.readableWriter)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("Failed to fetch layer %s, error: %s", dgst, err)
	}
	mu.Lock()
	blobFetch := inflight[dgst]
	if blobFetch != nil && blobFetch.completeCond != nil {
		cond := blobFetch.completeCond
		cond.Broadcast()
	}
	delete(inflight, dgst)
	mu.Unlock()
}

func (pbs *proxyBlobStore) doFetchFromRemote(dgst digest.Digest, ctx context.Context, bw storage.ReadableWriter) error {
	if err := pbs.authChallenger.tryEstablishChallenges(ctx); err != nil {
		bw.CancelWithError(ctx, err)
		return err
	}
	desc, err := pbs.remoteStore.Stat(ctx, dgst)
	if err != nil {
		bw.CancelWithError(ctx, err)
		return err
	}
	bw.SetDescriptor(desc)
	err = pbs.copyContent(ctx, dgst, bw, desc)
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		bw.CancelWithError(ctx, err)
		dcontext.GetLogger(ctx).Errorf("Error copying to storage: %s", err.Error())
		return err
	}

	_, err = bw.Commit(ctx, desc)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("Error committing to storage: %s", err.Error())
		return err
	}

	blobRef, err := reference.WithDigest(pbs.repositoryName, dgst)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("Error creating reference: %s", err)
		return err
	}

	pbs.scheduler.AddBlob(blobRef, repositoryTTL)
	return nil
}

func (pbs *proxyBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	mu.Lock()
	fetch, ok := inflight[dgst]
	isPresent := pbs.IsPresentLocally(ctx, dgst)

	dcontext.GetLogger(ctx).Debugf("digest %s present: %t, in flight: %t, in flight object: %v", dgst, isPresent, ok, fetch)
	isNew := !isPresent && (!ok || ok && !fetch.readableWriter.IsInProgress())
	if isNew {
		writer, err := pbs.localStore.Create(ctx)
		if err != nil {
			mu.Unlock()
			return err
		}
		rWriter, _ := writer.(storage.ReadableWriter)
		fetch = newBlobFetch(rWriter)
		inflight[dgst] = fetch
		go pbs.fetchFromRemote(dgst, ctx, fetch)
	}

	inflightReader := storage.BlobReader(nil)
	var err error
	if fetch != nil {
		inflightReader, err = fetch.readableWriter.Reader()
	}
	mu.Unlock()
	switch err.(type) {
	case storagedriver.ErrUnsupportedMethod:
		if fetch != nil && fetch.readableWriter.IsInProgress() {
			fetch.mutex.Lock()
			fetch.completeCond.Wait()
			fetch.mutex.Unlock()
			// propagate error
			_, err = fetch.readableWriter.GetDescriptor()
			isPresent = pbs.IsPresentLocally(ctx, dgst)
		}
	}
	if err != nil {
		return err
	}
	if isPresent {
		err := pbs.doServeFromLocalStore(ctx, w, r, dgst)
		if err != nil {
			dcontext.GetLogger(ctx).Errorf("Error serving blob from local storage: %s", err.Error())
			return err
		}
		dcontext.GetLogger(ctx).Debugf("Served %s from local storage, repo: %s", dgst, pbs.repositoryName)
	} else {
		if inflightReader == nil {
			// this should not be reached, local stream should always be readable
			return errors.New("unable to read blob being written")
		}
		desc, err := inflightReader.GetDescriptor()
		if err != nil {
			dcontext.GetLogger(ctx).Errorf("Error occurred while waiting descriptor from in-flight reader: %s", err)
			return err
		}
		setResponseHeaders(w, desc.Size, desc.MediaType, dgst)
		size, err := io.Copy(w, inflightReader)
		if err != nil {
			dcontext.GetLogger(ctx).Errorf("Error occurred while fetching from in-flight reader: %s", err)
			return err
		}
		desc, err = pbs.localStore.Stat(ctx, dgst)
		if err != nil {
			return err
		}
		if desc.Size != size {
			dcontext.GetLogger(ctx).Errorf("Size did not match for %s. Expected: %d, downloaded: %d", dgst, desc.Size, size)
			return io.EOF
		}
	}
	return nil
}

func (pbs *proxyBlobStore) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	desc, err := pbs.localStore.Stat(ctx, dgst)
	if err == nil {
		return desc, err
	}

	if err != distribution.ErrBlobUnknown {
		return distribution.Descriptor{}, err
	}

	if err := pbs.authChallenger.tryEstablishChallenges(ctx); err != nil {
		return distribution.Descriptor{}, err
	}

	return pbs.remoteStore.Stat(ctx, dgst)
}

func (pbs *proxyBlobStore) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	blob, err := pbs.localStore.Get(ctx, dgst)
	if err == nil {
		return blob, nil
	}

	if err := pbs.authChallenger.tryEstablishChallenges(ctx); err != nil {
		return []byte{}, err
	}

	blob, err = pbs.remoteStore.Get(ctx, dgst)
	if err != nil {
		return []byte{}, err
	}

	_, err = pbs.localStore.Put(ctx, "", blob)
	if err != nil {
		return []byte{}, err
	}
	return blob, nil
}

// Unsupported functions
func (pbs *proxyBlobStore) Put(ctx context.Context, mediaType string, p []byte) (distribution.Descriptor, error) {
	return distribution.Descriptor{}, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Create(ctx context.Context, options ...distribution.BlobCreateOption) (distribution.BlobWriter, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Mount(ctx context.Context, sourceRepo reference.Named, dgst digest.Digest) (distribution.Descriptor, error) {
	return distribution.Descriptor{}, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Delete(ctx context.Context, dgst digest.Digest) error {
	return distribution.ErrUnsupported
}

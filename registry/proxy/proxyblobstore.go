package proxy

import (
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/proxy/scheduler"
)

// todo(richardscothern): from cache control header or config file
const blobTTL = time.Duration(24 * 7 * time.Hour)

type proxyBlobStore struct {
	localStore  distribution.BlobStore
	remoteStore distribution.BlobService
	scheduler   *scheduler.TTLExpirationScheduler
}

var _ distribution.BlobStore = proxyBlobStore{}

type inflightBlob struct {
	refCount int
	bw       distribution.BlobWriter
}

// inflight tracks currently downloading blobs
var inflight = make(map[digest.Digest]*inflightBlob)

// mu protects inflight
var mu sync.Mutex

func setResponseHeaders(w http.ResponseWriter, length int64, mediaType string, digest digest.Digest) {
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest.String())
	w.Header().Set("Etag", digest.String())
}

func (pbs proxyBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	desc, err := pbs.localStore.Stat(ctx, dgst)
	if err != nil && err != distribution.ErrBlobUnknown {
		return err
	}

	if err == nil {
		proxyMetrics.BlobPush(uint64(desc.Size))
		return pbs.localStore.ServeBlob(ctx, w, r, dgst)
	}

	desc, err = pbs.remoteStore.Stat(ctx, dgst)
	if err != nil {
		return err
	}

	remoteReader, err := pbs.remoteStore.Open(ctx, dgst)
	if err != nil {
		return err
	}

	bw, isNew, cleanup, err := getOrCreateBlobWriter(ctx, pbs.localStore, desc)
	if err != nil {
		return err
	}
	defer cleanup()

	if isNew {
		go func() {
			err := streamToStorage(ctx, remoteReader, desc, bw)
			if err != nil {
				context.GetLogger(ctx).Error(err)
			}

			proxyMetrics.BlobPull(uint64(desc.Size))
		}()
		err := streamToClient(ctx, w, desc, bw)
		if err != nil {
			return err
		}

		proxyMetrics.BlobPush(uint64(desc.Size))
		pbs.scheduler.AddBlob(dgst.String(), blobTTL)
		return nil
	}

	err = streamToClient(ctx, w, desc, bw)
	if err != nil {
		return err
	}
	proxyMetrics.BlobPush(uint64(desc.Size))
	return nil
}

type cleanupFunc func()

// getOrCreateBlobWriter will track which blobs are currently being downloaded and enable client requesting
// the same blob concurrently to read from the existing stream.
func getOrCreateBlobWriter(ctx context.Context, blobs distribution.BlobService, desc distribution.Descriptor) (distribution.BlobWriter, bool, cleanupFunc, error) {
	mu.Lock()
	defer mu.Unlock()
	dgst := desc.Digest

	cleanup := func() {
		mu.Lock()
		defer mu.Unlock()
		inflight[dgst].refCount--

		if inflight[dgst].refCount == 0 {
			defer delete(inflight, dgst)
			_, err := inflight[dgst].bw.Commit(ctx, desc)
			if err != nil {
				// There is a narrow race here where Commit can be called while this blob's TTL is expiring
				// and its being removed from storage.  In that case, the client stream will continue
				// uninterruped and the blob will be pulled through on the next request, so just log it
				context.GetLogger(ctx).Errorf("Error committing blob: %q", err)
			}

		}
	}

	var bw distribution.BlobWriter
	_, ok := inflight[dgst]
	if ok {
		bw = inflight[dgst].bw
		inflight[dgst].refCount++
		return bw, false, cleanup, nil
	}

	var err error
	bw, err = blobs.Create(ctx)
	if err != nil {
		return nil, false, nil, err
	}

	inflight[dgst] = &inflightBlob{refCount: 1, bw: bw}
	return bw, true, cleanup, nil
}

func streamToStorage(ctx context.Context, remoteReader distribution.ReadSeekCloser, desc distribution.Descriptor, bw distribution.BlobWriter) error {
	_, err := io.CopyN(bw, remoteReader, desc.Size)
	if err != nil {
		return err
	}

	return nil
}

func streamToClient(ctx context.Context, w http.ResponseWriter, desc distribution.Descriptor, bw distribution.BlobWriter) error {
	setResponseHeaders(w, desc.Size, desc.MediaType, desc.Digest)

	reader, err := bw.Reader()
	if err != nil {
		return err
	}
	defer reader.Close()
	teeReader := io.TeeReader(reader, w)
	buf := make([]byte, 32768, 32786)
	var soFar int64
	for {
		rd, err := teeReader.Read(buf)
		if err == nil || err == io.EOF {
			soFar += int64(rd)
			if soFar < desc.Size {
				// buffer underflow, keep trying
				continue
			}
			return nil
		}
		return err
	}
}

func (pbs proxyBlobStore) Stat(ctx context.Context, dgst digest.Digest) (distribution.Descriptor, error) {
	desc, err := pbs.localStore.Stat(ctx, dgst)
	if err == nil {
		return desc, err
	}

	if err != distribution.ErrBlobUnknown {
		return distribution.Descriptor{}, err
	}

	return pbs.remoteStore.Stat(ctx, dgst)
}

// Unsupported functions
func (pbs proxyBlobStore) Put(ctx context.Context, mediaType string, p []byte) (distribution.Descriptor, error) {
	return distribution.Descriptor{}, distribution.ErrUnsupported
}

func (pbs proxyBlobStore) Create(ctx context.Context) (distribution.BlobWriter, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs proxyBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs proxyBlobStore) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs proxyBlobStore) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs proxyBlobStore) Delete(ctx context.Context, dgst digest.Digest) error {
	return distribution.ErrUnsupported
}

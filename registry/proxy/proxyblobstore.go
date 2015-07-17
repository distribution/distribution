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
	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/registry/proxy/scheduler"
)

// todo(richardscothern): decide what to do on local disk failure - for now
// propogate failures back up the stack

// todo(richardscothern): from cache control header
const blobTTL = time.Duration(10 * time.Minute)

type proxyBlobStore struct {
	localStore  distribution.BlobService
	remoteStore distribution.BlobService
}

var _ distribution.BlobStore = proxyBlobStore{}

// inflight tracks currently downloading blobs
var inflight = make(map[digest.Digest]distribution.BlobWriter)

// mu protects inflight
var mu sync.Mutex

func setResponseHeaders(w http.ResponseWriter, length int64, mediaType string, digest digest.Digest) {
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Docker-Content-Digest", digest.String())
	w.Header().Set("Etag", digest.String())
}

// todo(richardscothern): Support Content-Range
func (pbs proxyBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	desc, err := pbs.localStore.Stat(ctx, dgst)
	if err != nil && err != distribution.ErrBlobUnknown {
		return err
	}

	if err == nil {
		proxyMetrics.BlobPush(uint64(desc.Length))
		return pbs.serveLocalBlob(ctx, dgst, w, r)
	}

	desc, err = pbs.remoteStore.Stat(ctx, dgst)
	if err != nil {
		return err
	}

	remoteReader, err := pbs.remoteStore.Open(ctx, dgst)
	if err != nil {
		return err
	}

	bw, isNew, cleanup, err := getOrCreateBlobWriter(ctx, pbs.localStore, dgst)
	if err != nil {
		return err
	}
	defer cleanup()

	if isNew {
		doneChan := make(chan bool)
		go func() {
			err := streamToStorage(ctx, remoteReader, desc, bw, doneChan)
			if err != nil {
				context.GetLogger(ctx).Error(err)
			}

			proxyMetrics.BlobPull(uint64(desc.Length))
		}()
		err := streamToClient(ctx, w, desc, bw)
		if err != nil {
			return err
		}

		doneChan <- true

		proxyMetrics.BlobPush(uint64(desc.Length))
		scheduler.AddBlob(dgst.String(), blobTTL)
		return nil
	}

	err = streamToClient(ctx, w, desc, bw)
	if err != nil {
		return err
	}
	proxyMetrics.BlobPush(uint64(desc.Length))
	return nil
}

type cleanupFunc func()

func getOrCreateBlobWriter(ctx context.Context, blobs distribution.BlobService, dgst digest.Digest) (distribution.BlobWriter, bool, cleanupFunc, error) {
	var bw distribution.BlobWriter
	mu.Lock()
	defer mu.Unlock()

	cleanup := func() {
		// Intentionally blank
	}

	_, isInflight := inflight[dgst]
	if isInflight {
		bw = inflight[dgst]
		cleanup = func() {
			mu.Lock()
			delete(inflight, dgst)
			mu.Unlock()
		}
	} else {
		var err error
		bw, err = blobs.Create(ctx)
		if err != nil {
			return nil, false, nil, err
		}
		inflight[dgst] = bw
	}
	return bw, !isInflight, cleanup, nil
}

func streamToStorage(ctx context.Context, remoteReader distribution.ReadSeekCloser, desc distribution.Descriptor, bw distribution.BlobWriter, readyChan chan bool) error {
	_, err := io.CopyN(bw, remoteReader, desc.Length)
	if err != nil {
		return err
	}

	// Wait for the data to to be streamed to the client before
	// Commiting the blob
	<-readyChan

	_, err = bw.Commit(ctx, desc)
	if err != nil {
		return err
	}

	return nil
}

func streamToClient(ctx context.Context, w http.ResponseWriter, desc distribution.Descriptor, bw distribution.BlobWriter) error {
	setResponseHeaders(w, desc.Length, desc.MediaType, desc.Digest)

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
			if soFar < desc.Length {
				// buffer underflow, keep trying
				continue
			}
			return nil
		}
		return err
	}
}

func (pbs proxyBlobStore) serveLocalBlob(ctx context.Context, dgst digest.Digest, w http.ResponseWriter, r *http.Request) error {
	localReader, err := pbs.localStore.Open(ctx, dgst)
	if err != nil {
		return err
	}
	defer localReader.Close()

	http.ServeContent(w, r, "", time.Time{}, localReader)
	return nil
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
	return distribution.Descriptor{}, v2.ErrorCodeUnsupported
}

func (pbs proxyBlobStore) Create(ctx context.Context) (distribution.BlobWriter, error) {
	return nil, v2.ErrorCodeUnsupported
}

func (pbs proxyBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	return nil, v2.ErrorCodeUnsupported
}

func (pbs proxyBlobStore) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	return nil, v2.ErrorCodeUnsupported
}

func (pbs proxyBlobStore) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	return nil, v2.ErrorCodeUnsupported
}

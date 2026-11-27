package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/proxy/scheduler"
	"github.com/distribution/reference"
)

type proxyBlobStore struct {
	localStore        distribution.BlobStore
	remoteStore       distribution.BlobStore
	scheduler         *scheduler.TTLExpirationScheduler
	ttl               *time.Duration
	cacheWriteTimeout time.Duration
	rangePassthrough  bool
	repositoryName    reference.Named
	authChallenger    authChallenger
}

var _ distribution.BlobStore = &proxyBlobStore{}

// inflight tracks currently downloading blobs
var inflight = make(map[digest.Digest]struct{})

// mu protects inflight
var mu sync.Mutex

func setResponseHeaders(h http.Header, desc v1.Descriptor, digest digest.Digest) {
	h.Set("Content-Length", strconv.FormatInt(desc.Size, 10))
	h.Set("Content-Type", desc.MediaType)
	h.Set("Docker-Content-Digest", digest.String())
	h.Set("Etag", fmt.Sprintf(`"%s"`, digest))
}

// blobResponseWriter sets the blob headers on the first body write. Until then
// the handler can still send an error response with its own headers.
type blobResponseWriter struct {
	dst    io.Writer
	header http.Header
	desc   v1.Descriptor
	digest digest.Digest
}

func (w *blobResponseWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if w.header != nil {
		setResponseHeaders(w.header, w.desc, w.digest)
		w.header = nil
	}
	return w.dst.Write(p)
}

func (pbs *proxyBlobStore) copyContent(ctx context.Context, dgst digest.Digest, writer io.Writer, h http.Header) (v1.Descriptor, error) {
	desc, err := pbs.remoteStore.Stat(ctx, dgst)
	if err != nil {
		return v1.Descriptor{}, err
	}

	remoteReader, err := pbs.remoteStore.Open(ctx, dgst)
	if err != nil {
		return v1.Descriptor{}, err
	}

	defer remoteReader.Close()

	dst := &blobResponseWriter{dst: writer, header: h, desc: desc, digest: dgst}
	if _, err := io.CopyN(dst, remoteReader, desc.Size); err != nil {
		return v1.Descriptor{}, err
	}

	proxyMetrics.BlobPull(uint64(desc.Size))
	proxyMetrics.BlobPush(uint64(desc.Size), false)

	return desc, nil
}

func (pbs *proxyBlobStore) serveLocal(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) (bool, error) {
	localDesc, err := pbs.localStore.Stat(ctx, dgst)
	if err != nil {
		// Stat can report a zero sized file here if it's checked between creation
		// and population.  Return nil error, and continue
		return false, nil
	}

	proxyMetrics.BlobPush(uint64(localDesc.Size), true)
	return true, pbs.localStore.ServeBlob(ctx, w, r, dgst)
}

func (pbs *proxyBlobStore) serveBlobRange(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) (bool, error) {
	if !pbs.rangePassthrough || r.Method != http.MethodGet || r.Header.Get("Range") == "" {
		return false, nil
	}

	// Range responses, including a full 200 fallback, never enter the cache
	// writer or the digest-wide inflight path.
	ctx, w = dcontext.WithResponseWriter(ctx, w)
	err := pbs.remoteStore.ServeBlob(ctx, w, r, dgst)
	status := ctx.Value("http.response.status")
	if err == nil && (status == http.StatusOK || status == http.StatusPartialContent) {
		written := ctx.Value("http.response.written").(int64)
		proxyMetrics.BlobPull(uint64(written))
		proxyMetrics.BlobPush(uint64(written), false)
	}
	return true, err
}

func (pbs *proxyBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	served, err := pbs.serveLocal(ctx, w, r, dgst)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("Error serving blob from local storage: %s", err.Error())
		return err
	}

	if served {
		return nil
	}

	if err := pbs.authChallenger.tryEstablishChallenges(ctx); err != nil {
		return err
	}

	if served, err := pbs.serveBlobRange(ctx, w, r, dgst); served || err != nil {
		return err
	}

	mu.Lock()
	_, ok := inflight[dgst]
	if ok {
		// If the blob has been serving in other requests.
		// Will return the blob from the remote store directly.
		// TODO Maybe we could reuse the these blobs are serving remotely and caching locally.
		mu.Unlock()
		desc, err := pbs.copyContent(ctx, dgst, w, w.Header())
		if err == nil && desc.Size == 0 {
			setResponseHeaders(w.Header(), desc, dgst)
		}
		return err
	}
	inflight[dgst] = struct{}{}
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(inflight, dgst)
		mu.Unlock()
	}()

	// Create a detached context for the blob writer that won't be canceled
	// when the HTTP request context is canceled. This allows the cache write
	// to complete even if the client disconnects.
	// Use the configured timeout to prevent hanging operations.
	writerCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pbs.cacheWriteTimeout)
	defer cancel()

	bw, err := pbs.localStore.Create(writerCtx)
	if err != nil {
		return err
	}

	committed := false
	// Ensure the writer is canceled if we return early with an error
	defer func() {
		if !committed {
			if err := bw.Cancel(writerCtx); err != nil {
				dcontext.GetLogger(ctx).WithError(err).Errorf("Error canceling blob writer")
			}
		}
	}()

	// Serving client and storing locally over same fetching request.
	// This can prevent a redundant blob fetching.
	multiWriter := io.MultiWriter(w, bw)
	desc, err := pbs.copyContent(ctx, dgst, multiWriter, w.Header())
	if err != nil {
		return err
	}

	_, err = bw.Commit(writerCtx, desc)
	if err != nil {
		return err
	}

	committed = true

	blobRef, err := reference.WithDigest(pbs.repositoryName, dgst)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("Error creating reference: %s", err)
		return err
	}

	if pbs.scheduler != nil && pbs.ttl != nil {
		if err := pbs.scheduler.AddBlob(blobRef, *pbs.ttl); err != nil {
			dcontext.GetLogger(ctx).Errorf("Error adding blob: %s", err)
			return err
		}
	}

	// An empty blob has no body write. Its headers can go out once the cache
	// has accepted it and the request has succeeded.
	if desc.Size == 0 {
		setResponseHeaders(w.Header(), desc, dgst)
	}
	return nil
}

func (pbs *proxyBlobStore) Stat(ctx context.Context, dgst digest.Digest) (v1.Descriptor, error) {
	desc, err := pbs.localStore.Stat(ctx, dgst)
	if err == nil {
		return desc, err
	}

	if err != distribution.ErrBlobUnknown {
		return v1.Descriptor{}, err
	}

	if err := pbs.authChallenger.tryEstablishChallenges(ctx); err != nil {
		return v1.Descriptor{}, err
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
func (pbs *proxyBlobStore) Put(ctx context.Context, mediaType string, p []byte) (v1.Descriptor, error) {
	return v1.Descriptor{}, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Create(ctx context.Context, options ...distribution.BlobCreateOption) (distribution.BlobWriter, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Mount(ctx context.Context, sourceRepo reference.Named, dgst digest.Digest) (v1.Descriptor, error) {
	return v1.Descriptor{}, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Open(ctx context.Context, dgst digest.Digest) (io.ReadSeekCloser, error) {
	return nil, distribution.ErrUnsupported
}

func (pbs *proxyBlobStore) Delete(ctx context.Context, dgst digest.Digest) error {
	return distribution.ErrUnsupported
}

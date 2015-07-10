package proxy

import (
	"io"
	"net/http"
	"strconv"
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

func (pbs proxyBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	desc, err := pbs.localStore.Stat(ctx, dgst)
	if err != nil && err != distribution.ErrBlobUnknown {
		return err
	}

	if err == nil { // have it locally
		localReader, err := pbs.localStore.Open(ctx, dgst)
		if err != nil {
			return err
		}
		defer localReader.Close()

		http.ServeContent(w, r, "", time.Time{}, localReader)
		return nil
	}

	desc, err = pbs.remoteStore.Stat(ctx, dgst)
	if err != nil {
		return err
	}

	remoteReader, err := pbs.remoteStore.Open(ctx, dgst)
	if err != nil {
		return err
	}

	// http.ServeContent is too restrictive here - it requires a Seek to get the size which we already have
	// therefore todo(richardscothern): support If-Modified-Since

	bw, err := pbs.localStore.Create(ctx)
	if err != nil {
		context.GetLogger(ctx).Errorf("%s", err)
		return err
	}
	defer bw.Close()

	blobSize := desc.Length

	w.Header().Set("Content-Length", strconv.FormatInt(blobSize, 10))
	w.Header().Set("Content-Type", desc.MediaType)

	mw := io.MultiWriter(bw, w)
	written, err := io.CopyN(mw, remoteReader, desc.Length)
	if err != nil {
		context.GetLogger(ctx).Errorf("copy failed: %q", err)
		return err
	}
	context.GetLogger(ctx).Infof("Wrote %d bytes", written)
	_, err = bw.Commit(ctx, desc)
	if err != nil {
		return err
	}

	scheduler.AddBlob(dgst.String(), blobTTL)
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

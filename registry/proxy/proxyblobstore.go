package proxy

import (
	"bytes"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/registry/proxy/scheduler"
)

// todo(richardscothern): make configurable
const blobTTL = time.Duration(10 * time.Second)

type proxyBlobStore struct {
	localStore  distribution.BlobStore
	remoteStore distribution.BlobStore
}

var _ distribution.BlobStore = proxyBlobStore{}

func (pbs proxyBlobStore) Put(ctx context.Context, mediaType string, p []byte) (distribution.Descriptor, error) {
	return distribution.Descriptor{}, fmt.Errorf("Not Allowed")
}

func (pbs proxyBlobStore) Create(ctx context.Context) (distribution.BlobWriter, error) {
	return nil, fmt.Errorf("Not Allowed")
}

func (pbs proxyBlobStore) Resume(ctx context.Context, id string) (distribution.BlobWriter, error) {
	return nil, fmt.Errorf("Not Allowed")
}

func (pbs proxyBlobStore) Get(ctx context.Context, dgst digest.Digest) ([]byte, error) {
	blob, err := pbs.localStore.Get(ctx, dgst)
	if err == nil {
		return blob, nil
	}

	if err != distribution.ErrBlobUnknown {
		return []byte{}, err
	}

	// todo(richardscothern): this Get can be called by multiple goroutines which results
	// in correct behavior but can result in date being fetched more than once.  It's not
	// sufficient to block on the download as this can trigger idle socket timeouts.  This
	// needs to be able to handle streaming the data to multiple requests

	blob, err = pbs.remoteStore.Get(ctx, dgst)
	if err != nil {
		return []byte{}, err
	}

	go func() {
		_, err = pbs.localStore.Put(ctx, "application/octet-stream", blob)
		if err != nil {
			context.GetLogger(ctx).Errorf("Unable to write blob to local storage: %s", err)
		}
	}()

	scheduler.AddBlob(dgst.String(), blobTTL)

	return blob, nil
}

func (pbs proxyBlobStore) Open(ctx context.Context, dgst digest.Digest) (distribution.ReadSeekCloser, error) {
	return nil, fmt.Errorf("Not implemented yet")
}

func (pbs proxyBlobStore) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	blob, err := pbs.Get(ctx, dgst)
	if err != nil {
		return err
	}

	// todo(richardscothern): ensure descriptor cache has been updated

	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(blob))
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

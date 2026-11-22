package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/opencontainers/go-digest"
)

// TODO(stevvooe): This should configurable in the future.
const blobCacheControlMaxAge = 365 * 24 * time.Hour

type seekableReaderDriver interface {
	// ReadSeeker opens path at offset zero. Seeking to the end reports the
	// content size. The caller must close the reader.
	ReadSeeker(ctx context.Context, path string) (io.ReadSeekCloser, error)
}

// blobServer simply serves blobs from a driver instance using a path function
// to identify paths and a descriptor service to fill in metadata.
type blobServer struct {
	driver   driver.StorageDriver
	statter  distribution.BlobStatter
	pathFn   func(dgst digest.Digest) (string, error)
	redirect bool // allows disabling RedirectURL redirects
}

func (bs *blobServer) ServeBlob(ctx context.Context, w http.ResponseWriter, r *http.Request, dgst digest.Digest) error {
	desc, err := bs.statter.Stat(ctx, dgst)
	if err != nil {
		return err
	}

	path, err := bs.pathFn(desc.Digest)
	if err != nil {
		return err
	}

	if bs.redirect {
		redirectURL, err := bs.driver.RedirectURL(r, path)
		if err != nil {
			return err
		}
		if redirectURL != "" {
			// Redirect to storage URL.
			http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
			return nil
		}
		// Fallback to serving the content directly.
	}

	var br io.ReadSeekCloser
	if seekable, ok := bs.driver.(seekableReaderDriver); ok {
		br, err = seekable.ReadSeeker(ctx, path)
		if _, ok := err.(driver.PathNotFoundError); ok {
			return distribution.ErrBlobUnknown
		}
	} else {
		br, err = newFileReader(ctx, bs.driver, path, desc.Size)
	}
	if err != nil {
		return err
	}
	defer br.Close()

	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, desc.Digest)) // If-None-Match handled by ServeContent
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%.f", blobCacheControlMaxAge.Seconds()))

	if w.Header().Get("Docker-Content-Digest") == "" {
		w.Header().Set("Docker-Content-Digest", desc.Digest.String())
	}

	if w.Header().Get("Content-Type") == "" {
		// Set the content type if not already set.
		w.Header().Set("Content-Type", desc.MediaType)
	}

	// ServeContent sets Content-Length. A value set here would be sent with a 412.
	http.ServeContent(w, r, desc.Digest.String(), time.Time{}, br)
	return nil
}

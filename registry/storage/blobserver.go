package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/opencontainers/go-digest"
)

// TODO(stevvooe): This should configurable in the future.
const blobCacheControlMaxAge = 365 * 24 * time.Hour

// filesystemDriverName is the name that the filesystem storage driver reports.
const filesystemDriverName = "filesystem"

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

	br, err := newFileReader(ctx, bs.driver, path, desc.Size)
	if err != nil {
		return err
	}
	defer br.Close()

	var content io.ReadSeeker = br
	// On the filesystem driver, serve a GET from the blob file itself.
	// fileReader would allocate a buffer of up to fileReaderBufferSize per
	// GET and report desc.Size as the length, which can come from a stale
	// cache entry. Opening the file here reports a missing or unreadable
	// file before any header is written. For HEAD, ServeBlob keeps
	// fileReader, which then never opens the file. The name check spares
	// other drivers a second Reader call.
	if r.Method == http.MethodGet && bs.driver.Name() == filesystemDriverName {
		rc, err := bs.driver.Reader(ctx, path, 0)
		if err != nil {
			if _, ok := err.(driver.PathNotFoundError); ok {
				// The blob statter returns the same error when it finds the file missing.
				return distribution.ErrBlobUnknown
			}
			return err
		}
		defer rc.Close()
		// A storage middleware may wrap the reader; ServeBlob then keeps fileReader.
		if f, ok := rc.(*os.File); ok {
			content = f
		}
	}

	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, desc.Digest)) // If-None-Match handled by ServeContent
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%.f", blobCacheControlMaxAge.Seconds()))

	if w.Header().Get("Docker-Content-Digest") == "" {
		w.Header().Set("Docker-Content-Digest", desc.Digest.String())
	}

	if w.Header().Get("Content-Type") == "" {
		// Set the content type if not already set.
		w.Header().Set("Content-Type", desc.MediaType)
	}

	if w.Header().Get("Content-Length") == "" {
		// Set the content length if not already set.
		w.Header().Set("Content-Length", fmt.Sprint(desc.Size))
	}

	http.ServeContent(w, r, desc.Digest.String(), time.Time{}, content)
	return nil
}

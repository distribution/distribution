package testutil

import (
	"context"
	"fmt"
	"io"

	"github.com/distribution/distribution/v3"
	"github.com/opencontainers/go-digest"
)

// PushBlob pushes a blob with the given digest to the given repository.
func PushBlob(ctx context.Context, repository distribution.Repository, blobReader io.ReadSeeker, dgst digest.Digest) error {
	blobs := repository.Blobs(ctx)

	wr, err := blobs.Create(ctx)
	if err != nil {
		return fmt.Errorf("error creating layer upload: %v", err)
	}

	// Use the resumes, as well!
	wr, err = blobs.Resume(ctx, wr.ID())
	if err != nil {
		return fmt.Errorf("error resuming layer upload: %v", err)
	}

	if _, err := io.Copy(wr, blobReader); err != nil {
		return fmt.Errorf("unexpected error uploading: %v", err)
	}

	if _, err := wr.Commit(ctx, distribution.Descriptor{Digest: dgst}); err != nil {
		return fmt.Errorf("unexpected error finishing upload: %v", err)
	}

	return nil
}

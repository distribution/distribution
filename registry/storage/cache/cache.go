// Package cache provides facilities to speed up access to the storage
// backend.
package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var (
	ErrNoLastAccessedTime = errors.New("no last accessed time available")
)

// BlobDescriptorCacheProvider provides repository scoped
// BlobDescriptorService cache instances and a global descriptor cache.
type BlobDescriptorCacheProvider interface {
	distribution.BlobDescriptorService

	RepositoryScoped(repo string) (distribution.BlobDescriptorService, error)
	GetLastAccessed(ctx context.Context, dgst digest.Digest) (*time.Time, error)
}

// ValidateDescriptor provides a helper function to ensure that caches have
// common criteria for admitting descriptors.
func ValidateDescriptor(desc v1.Descriptor) error {
	if err := desc.Digest.Validate(); err != nil {
		return err
	}

	if desc.Size < 0 {
		return fmt.Errorf("cache: invalid length in descriptor: %v < 0", desc.Size)
	}

	if desc.MediaType == "" {
		return fmt.Errorf("cache: empty mediatype on descriptor: %v", desc)
	}

	return nil
}

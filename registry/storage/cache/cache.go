// Package cache provides facilities to speed up access to the storage
// backend.
package cache

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3"
)

// InitFunc is the type of a CacheProvider factory function and is
// used to register the constructor for different CacheProvider backends.
type InitFunc func(ctx context.Context, options map[string]interface{}) (BlobDescriptorCacheProvider, error)

var cacheProviders map[string]InitFunc

// BlobDescriptorCacheProvider provides repository scoped
// BlobDescriptorService cache instances and a global descriptor cache.
type BlobDescriptorCacheProvider interface {
	distribution.BlobDescriptorService

	RepositoryScoped(repo string) (distribution.BlobDescriptorService, error)
}

// ValidateDescriptor provides a helper function to ensure that caches have
// common criteria for admitting descriptors.
func ValidateDescriptor(desc distribution.Descriptor) error {
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

// Register is used to register an InitFunc for
// a BlobDescriptorCacheProvider with the given name.
// It's meant to be called from init() function.
func Register(name string, initFunc InitFunc) {
	if cacheProviders == nil {
		cacheProviders = make(map[string]InitFunc)
	}
	if _, exists := cacheProviders[name]; exists {
		panic(fmt.Sprintf("cache provider already registered with the name %q", name))
	}

	cacheProviders[name] = initFunc
}

// Get constructs a CacheProvider with the given options using the named backend.
func Get(ctx context.Context, name string, options map[string]interface{}) (BlobDescriptorCacheProvider, error) {
	if initFunc, exists := cacheProviders[name]; exists {
		return initFunc(ctx, options)
	}
	return nil, fmt.Errorf("no cache provider registered with name: %s", name)
}

package cacheprovider

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3/registry/storage/cache"
)

// InitFunc is the type of a CacheProvider factory function and is
// used to register the constructor for different CacheProvider backends.
type InitFunc func(ctx context.Context, options map[string]interface{}) (cache.BlobDescriptorCacheProvider, error)

var cacheProviders map[string]InitFunc

// Register is used to register an InitFunc for
// a CacheProvider backend with the given name.
func Register(name string, initFunc InitFunc) error {
	if cacheProviders == nil {
		cacheProviders = make(map[string]InitFunc)
	}
	if _, exists := cacheProviders[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	cacheProviders[name] = initFunc

	return nil
}

// Get constructs a CacheProvider with the given options using the named backend.
func Get(ctx context.Context, name string, options map[string]interface{}) (cache.BlobDescriptorCacheProvider, error) {
	if initFunc, exists := cacheProviders[name]; exists {
		return initFunc(ctx, options)
	}
	return nil, fmt.Errorf("no cache Provider registered with name: %s", name)
}

package extension

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3/registry/storage/extension"
)

type RepositoryExtension interface {
	extension.Extension
	RepositoryExtension(ctx context.Context) (interface{}, error)
}

type InitFunc func(ctx context.Context, options map[string]interface{}) (RepositoryExtension, error)

var extensions map[string]InitFunc

// Register is used to register an InitFunc for
// a storage extension backend with the given name.
func Register(name string, initFunc InitFunc) error {
	if extensions == nil {
		extensions = make(map[string]InitFunc)
	}

	if _, exists := extensions[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	extensions[name] = initFunc

	return nil
}

// Get constructs a storage extension with the given options using the named backend.
func Get(ctx context.Context, name string, options map[string]interface{}) (RepositoryExtension, error) {
	if extensions != nil {
		if initFunc, exists := extensions[name]; exists {
			return initFunc(ctx, options)
		}
	}

	return nil, fmt.Errorf("no storage repository extension registered with name: %s", name)
}

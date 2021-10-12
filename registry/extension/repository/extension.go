package extension

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3/registry/extension"
)

var extensions map[string]extension.InitFunc

// Register is used to register an InitFunc for
// a server extension backend with the given name.
func Register(name string, initFunc extension.InitFunc) error {
	if extensions == nil {
		extensions = make(map[string]extension.InitFunc)
	}

	if _, exists := extensions[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	extensions[name] = initFunc

	return nil
}

// Get constructs a server extension with the given options using the named backend.
func Get(ctx context.Context, name string, options map[string]interface{}) ([]extension.Route, error) {
	if extensions != nil {
		if initFunc, exists := extensions[name]; exists {
			return initFunc(ctx, options)
		}
	}

	return nil, fmt.Errorf("no server repository extension registered with name: %s", name)
}

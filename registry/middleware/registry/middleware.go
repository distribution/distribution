package middleware

import (
	"fmt"

	"github.com/docker/distribution"
)

// InitFunc is the type of a RegistryMiddleware factory function and is
// used to register the contsructor for different RegistryMiddleware backends.
type InitFunc func(registry distribution.Registry, options map[string]interface{}) (distribution.Registry, error)

var middlewares map[string]InitFunc

// Register is used to register an InitFunc for
// a RegistryMiddleware backend with the given name.
func Register(name string, initFunc InitFunc) error {
	if middlewares == nil {
		middlewares = make(map[string]InitFunc)
	}
	if _, exists := middlewares[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	middlewares[name] = initFunc

	return nil
}

// Get constructs a RegistryMiddleware with the given options using the named backend.
func Get(name string, options map[string]interface{}, registry distribution.Registry) (distribution.Registry, error) {
	if middlewares != nil {
		if initFunc, exists := middlewares[name]; exists {
			return initFunc(registry, options)
		}
	}

	return nil, fmt.Errorf("no registry middleware registered with name: %s", name)
}

package middleware

import (
	"fmt"

	"github.com/gorilla/mux"
)

// InitFunc is the type of a RegistryMiddleware factory function and is
// used to register the constructor for different RegistryMiddleware backends.
type InitFunc func(options map[string]interface{}) (mux.MiddlewareFunc, error)

var (
	middlewares map[string]InitFunc
)

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
func Get(name string, options map[string]interface{}) (mux.MiddlewareFunc, error) {
	if middlewares != nil {
		if initFunc, exists := middlewares[name]; exists {
			return initFunc(options)
		}
	}

	return nil, fmt.Errorf("no registry middleware registered with name: %s", name)
}

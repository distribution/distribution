package handlers

import (
	"fmt"

	"github.com/docker/distribution"
)

// RegistryHandlerInitFunc is the type of a RegistryHandler factory function.
type RegistryHandlerInitFunc func(registry distribution.Registry, options map[string]interface{}) (distribution.Registry, error)

var registryHandlers map[string]RegistryHandlerInitFunc

// RegisterRegistryHandler registers a RegistryHandlerInitFunc for a RegistryHandler
// with the given name.
func RegisterRegistryHandler(name string, initFunc RegistryHandlerInitFunc) error {
	if registryHandlers == nil {
		registryHandlers = make(map[string]RegistryHandlerInitFunc)
	}
	if _, exists := registryHandlers[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	registryHandlers[name] = initFunc

	return nil
}

// GetRegistryHandler constructs a RegistryHandler with the given options using the named handler.
func GetRegistryHandler(name string, registry distribution.Registry, options map[string]interface{}) (distribution.Registry, error) {
	if registryHandlers != nil {
		if initFunc, exists := registryHandlers[name]; exists {
			return initFunc(registry, options)
		}
	}

	return nil, fmt.Errorf("no registry handler registered with name: %s", name)
}

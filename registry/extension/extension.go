package extension

import (
	c "context"
	"fmt"
	"net/http"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/storage/driver"
)

// Context contains the request specific context for use in across handlers.
type Context struct {
	c.Context

	// Registry is the base namespace that is used by all extension namespaces
	Registry distribution.Namespace
	// Repository is a reference to a named repository
	Repository distribution.Repository
	// Errors are the set of errors that occurred within this request context
	Errors errcode.Errors
}

// RouteDispatchFunc is the http route dispatcher used by the extension route handlers
type RouteDispatchFunc func(extContext *Context, r *http.Request) http.Handler

// Route describes an extension route.
type Route struct {
	// Namespace is the name of the extension namespace
	Namespace string
	// Extension is the name of the extension under the namespace
	Extension string
	// Component is the name of the component under the extension
	Component string
	// Descriptor is the route descriptor that gives its path
	Descriptor v2.RouteDescriptor
	// Dispatcher if present signifies that the route is http route with a dispatcher
	Dispatcher RouteDispatchFunc
}

// Namespace is the namespace that is used to define extensions to the distribution.
type Namespace interface {
	// GetRepositoryRoutes returns a list of extension routes scoped at a repository level
	GetRepositoryRoutes() []Route
	// GetRegistryRoutes returns a list of extension routes scoped at a registry level
	GetRegistryRoutes() []Route
}

// InitExtensionNamespace is the initialize function for creating the extension namespace
type InitExtensionNamespace func(ctx c.Context, storageDriver driver.StorageDriver, options configuration.ExtensionConfig) (Namespace, error)

var extensions map[string]InitExtensionNamespace

// Register is used to register an InitExtensionNamespace for
// an extension namespace with the given name.
func Register(name string, initFunc InitExtensionNamespace) error {
	if extensions == nil {
		extensions = make(map[string]InitExtensionNamespace)
	}

	if _, exists := extensions[name]; exists {
		return fmt.Errorf("namespace name already registered: %s", name)
	}

	extensions[name] = initFunc

	return nil
}

// Get constructs an extension namespace with the given options using the given named.
func Get(ctx c.Context, name string, storageDriver driver.StorageDriver, options configuration.ExtensionConfig) (Namespace, error) {
	if extensions != nil {
		if initFunc, exists := extensions[name]; exists {
			return initFunc(ctx, storageDriver, options)
		}
	}

	return nil, fmt.Errorf("no extension registered with name: %s", name)
}

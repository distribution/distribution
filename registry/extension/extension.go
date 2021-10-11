package extension

import (
	"context"
	"fmt"
	"net/http"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
)

// Context contains the request specific context for use in across handlers.
type Context struct {
	context.Context

	Repository        distribution.Repository
	RepositoryRemover distribution.RepositoryRemover
	Errors            errcode.Errors
}

type DispatchFunc func(ctx *Context, r *http.Request) http.Handler

// Route describes an extended route.
type Route struct {
	Namespace    string
	Extension    string
	Component    string
	NameRequired bool
	Descriptor   v2.RouteDescriptor
	Dispatcher   DispatchFunc
}

// InitFunc is the type of a server extension factory function and is
// used to register the constructor for different server extension backends.
type InitFunc func(ctx context.Context, options map[string]interface{}) ([]Route, error)

var extensions map[string]InitFunc

// Register is used to register an InitFunc for
// a server extension backend with the given name.
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

// Get constructs a server extension with the given options using the named backend.
func Get(ctx context.Context, name string, options map[string]interface{}) ([]Route, error) {
	if extensions != nil {
		if initFunc, exists := extensions[name]; exists {
			return initFunc(ctx, options)
		}
	}

	return nil, fmt.Errorf("no server extension registered with name: %s", name)
}

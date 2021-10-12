package extension

import (
	"context"
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
	Namespace  string
	Extension  string
	Component  string
	Descriptor v2.RouteDescriptor
	Dispatcher DispatchFunc
}

// InitFunc is the type of a server extension factory function and is
// used to register the constructor for different server extension backends.
type InitFunc func(ctx context.Context, options map[string]interface{}) ([]Route, error)

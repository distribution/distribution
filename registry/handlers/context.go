package handlers

import (
	"context"
	"fmt"
	"net/http"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/auth"
	"github.com/opencontainers/go-digest"
)

// Context should contain the request specific context for use in across
// handlers. Resources that don't need to be shared across handlers should not
// be on this object.
type Context struct {
	context.Context

	// Repository is the repository for the current request. All requests
	// should be scoped to a single repository. This field may be nil.
	Repository distribution.Repository

	// Errors is a collection of errors encountered during the request to be
	// returned to the client API. If errors are added to the collection, the
	// handler *must not* start the response via http.ResponseWriter.
	Errors errcode.Errors

	urlBuilder *v2.URLBuilder

	// TODO(stevvooe): The goal is too completely factor this context and
	// dispatching out of the web application. Ideally, we should lean on
	// context.Context for injection of these resources.
}

// Value overrides context.Context.Value to ensure that calls are routed to
// correct context.
func (ctx *Context) Value(key interface{}) interface{} {
	return ctx.Context.Value(key)
}

func getName(ctx context.Context) (name string) {
	return dcontext.GetStringValue(ctx, "vars.name")
}

func getReference(ctx context.Context) (reference string) {
	return dcontext.GetStringValue(ctx, "vars.reference")
}

var errDigestNotAvailable = fmt.Errorf("digest not available in context")

func getDigest(ctx context.Context) (dgst digest.Digest, err error) {
	dgstStr := dcontext.GetStringValue(ctx, "vars.digest")

	if dgstStr == "" {
		dcontext.GetLogger(ctx).Errorf("digest not available")
		return "", errDigestNotAvailable
	}

	d, err := digest.Parse(dgstStr)
	if err != nil {
		dcontext.GetLogger(ctx).Errorf("error parsing digest=%q: %v", dgstStr, err)
		return "", err
	}

	return d, nil
}

func getUploadUUID(ctx context.Context) (uuid string) {
	return dcontext.GetStringValue(ctx, "vars.uuid")
}

const (
	// userKey is used to get the user object from
	// a user context
	userKey = "auth.user"

	// userNameKey is used to get the user name from
	// a user context
	userNameKey = "auth.user.name"
)

// getUserName attempts to resolve a username from the context and request. If
// a username cannot be resolved, the empty string is returned.
func getUserName(ctx context.Context, r *http.Request) string {
	username := dcontext.GetStringValue(ctx, userNameKey)

	// Fallback to request user with basic auth
	if username == "" {
		var ok bool
		uname, _, ok := basicAuth(r)
		if ok {
			username = uname
		}
	}

	return username
}

// withUser returns a context with the authorized user info.
func withUser(ctx context.Context, user auth.UserInfo) context.Context {
	return userInfoContext{
		Context: ctx,
		user:    user,
	}
}

type userInfoContext struct {
	context.Context
	user auth.UserInfo
}

func (uic userInfoContext) Value(key interface{}) interface{} {
	switch key {
	case userKey:
		return uic.user
	case userNameKey:
		return uic.user.Name
	}

	return uic.Context.Value(key)
}

// withResources returns a context with the authorized resources.
func withResources(ctx context.Context, resources []auth.Resource) context.Context {
	return resourceContext{
		Context:   ctx,
		resources: resources,
	}
}

type resourceContext struct {
	context.Context
	resources []auth.Resource
}

type resourceKey struct{}

func (rc resourceContext) Value(key interface{}) interface{} {
	if key == (resourceKey{}) {
		return rc.resources
	}

	return rc.Context.Value(key)
}

// authorizedResources returns the list of resources which have
// been authorized for this request.
func authorizedResources(ctx context.Context) []auth.Resource {
	if resources, ok := ctx.Value(resourceKey{}).([]auth.Resource); ok {
		return resources
	}

	return nil
}

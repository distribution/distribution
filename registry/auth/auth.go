// Package auth defines a standard interface for request access controllers.
//
// An access controller has a simple interface with a single `Authorized`
// method which checks that a given context is authorized to perform one or
// more actions on a resource. This method should return a non-nil
// error if the context is not authorized.
//
// An implementation registers its access controller by name with a constructor
// which accepts an options map for configuring the access controller.
//
//		options := map[string]interface{}{"sillySecret": "whysosilly?"}
// 		accessController, _ := auth.GetAccessController("silly", options)
//
// This `accessController` can then be used in a request handler like so:
//
// 		func updateOrder(w http.ResponseWriter, r *http.Request) {
//			orderNumber := r.FormValue("orderNumber")
//			order := auth.Resource{Type: "customerOrder", Name: orderNumber}
//
//			// Is the client authorized to update the order?
//			if ctx, err := accessController.Authorized(ctx, order, "update"); err != nil {
//				switch err := err.(type) {
//				case auth.AuthenticationError:
//					// Let the error set a challenge header.
//					err.SetChallengeHeaders(w.Header())
//					w.WriteHeader(http.StatusUnauthorized)
//					w.Write([]byte(err.AuthenticationErrorDetails()))
//					return
//				case auth.AuthorizationError:
//					if err.ResourceHidden() {
//						w.WriteHeader(http.StatusNotFound)
//						return
//					}
//					w.WriteHeader(http.StatusForbidden)
//					w.Write([]byte(err.AuthorizationErrorDetails()))
//					return
//				default:
//					// Some other error.
//				}
//			}
// 		}
//
package auth

import (
	"fmt"
	"net/http"

	"github.com/docker/distribution/context"
)

// UserInfo carries information about
// an autenticated/authorized client.
type UserInfo struct {
	Name string
}

// Resource describes a resource by type and name.
type Resource struct {
	Type string
	Name string
}

// AuthenticationError is a special error type which is used to indicate that a
// client either has invalid authentication credentials or, if the client is
// not authenticated, should attempt to authenticate in order to access the
// requested resource. A type which implements this interface is able to set
// HTTP WWW-Authenticate challenge response header values based on the error.
// Note: HTTP status code "401 Unauthorized" is used semantically to indicate
// that the client is "Unauthenticated". To indicate that the client is in fact
// unauthorized (despite being authenticated), an access controller should
// return an error implementing the AuthorizationError interface.
type AuthenticationError interface {
	error

	// AuthenticationErrorDetails should return a JSON-serializable object
	// detailing the authentication error, e.g., authentication required,
	// invalid username/password, expired token, invalid signature, etc.
	AuthenticationErrorDetails() interface{}

	// SetChallengeHeaders prepares an authentication challenge response by
	// setting one or more HTTP WWW-Authenticate challenge headers.
	// Callers are expected to set the appropriate HTTP status code (i.e.,
	// 401) themselves.
	SetChallengeHeaders(h http.Header)
}

// AuthorizationError is a special error type which is used to indicate that a
// client is not authorized to access the requested resource.
type AuthorizationError interface {
	error

	// AuthorizationErrorDetails should return a JSON-serializable object
	// detailing the authorization error, e.g., user is not authorized to
	// push, etc. It is the caller's responsibility to include these
	// details in an HTTP 403 Forbidden response.
	AuthorizationErrorDetails() interface{}

	// ResourceHidden should return whether the existence of the requested
	// resource should be exposed to the client. If true, the caller MUST
	// return an HTTP 404 Not Found response in lieu of a 403 Forbidden or
	// 401 Unauthorized response so as to not leak the existince of the
	// resource to the client.
	ResourceHidden() bool
}

// AccessController controls access to a registry resource based on a request
// context and the attempted actions being performed on that resource.
// Implementations must validate complete authorization or indicate
// authentication or authorization errors through the AuthenticationError and
// AuthorizationError interfaces.
type AccessController interface {
	// Authorized returns a nil error if the context is granted access and
	// returns a new authorized context. If one or more action strings are
	// provided, the requested access will be compared with what is
	// available to the context. The given context will contain a
	// "http.request" key with a `*http.Request` value. If the error is
	// non-nil, access should always be denied. The returned error may
	// implement the AuthenticationError or AuthorizationError interface in
	// which case the caller should take the appropriate action based on
	// the error. The returned context object should have a "auth.user"
	// value set to a UserInfo struct.
	Authorized(ctx context.Context, resource Resource, actions ...string) (context.Context, error)
}

// WithUser returns a context with the authorized user info.
func WithUser(ctx context.Context, user UserInfo) context.Context {
	return userInfoContext{
		Context: ctx,
		user:    user,
	}
}

type userInfoContext struct {
	context.Context
	user UserInfo
}

func (uic userInfoContext) Value(key interface{}) interface{} {
	switch key {
	case "auth.user":
		return uic.user
	case "auth.user.name":
		return uic.user.Name
	}

	return uic.Context.Value(key)
}

// InitFunc is the type of an AccessController factory function and is used
// to register the constructor for different AccesController backends.
type InitFunc func(options map[string]interface{}) (AccessController, error)

var accessControllers map[string]InitFunc

func init() {
	accessControllers = make(map[string]InitFunc)
}

// Register is used to register an InitFunc for
// an AccessController backend with the given name.
func Register(name string, initFunc InitFunc) error {
	if _, exists := accessControllers[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	accessControllers[name] = initFunc

	return nil
}

// GetAccessController constructs an AccessController
// with the given options using the named backend.
func GetAccessController(name string, options map[string]interface{}) (AccessController, error) {
	if initFunc, exists := accessControllers[name]; exists {
		return initFunc(options)
	}

	return nil, fmt.Errorf("no access controller registered with name: %s", name)
}

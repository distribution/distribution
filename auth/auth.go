package auth

import (
	"fmt"
	"net/http"
)

// Resource describes a resource by type and name.
type Resource struct {
	Type string
	Name string
}

// Access describes a specific action that is
// requested or allowed for a given recource.
type Access struct {
	Resource
	Action string
}

// Challenge is a special error type which is used for HTTP 401 Unauthorized
// responses and is able to write the response with WWW-Authenticate challenge
// header values based on the error.
type Challenge interface {
	error
	Status() int
	SetHeader(header http.Header)
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// AccessController controls access to registry resources based on a request
// and required access levels for a request. Implementations can support both
// complete denial and http authorization challenges.
type AccessController interface {
	// Authorized returns non-nil if the request is granted the request
	// access. If the error is non-nil, access should always be denied. The
	// error may be of type Challenge, in which case the caller may have the
	// Challenge handle the request or choose what action to take based on the
	// Challenge header or response status.
	//
	// In the future, other error types, besides Challenge, may be added to
	// support more complex authorization flows.
	Authorized(req *http.Request, access ...Access) error
}

// InitFunc is the type of an AccessController factory function and is used
// to register the contsructor for different AccesController backends.
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

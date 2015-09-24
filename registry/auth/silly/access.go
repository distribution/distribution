// Package silly provides a simple authentication scheme that checks for the
// existence of an Authorization header and issues access if is present and
// non-empty.
//
// This package is present as an example implementation of a minimal
// auth.AccessController and for testing. This is not suitable for any kind of
// production security.
package silly

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"
)

// accessController provides a simple implementation of auth.AccessController
// that simply checks for a non-empty Authorization header. It is useful for
// demonstration and testing.
type accessController struct {
	realm   string
	service string
}

var _ auth.AccessController = &accessController{}

func newAccessController(options map[string]interface{}) (auth.AccessController, error) {
	realm, present := options["realm"]
	if _, ok := realm.(string); !present || !ok {
		return nil, fmt.Errorf(`"realm" must be set for silly access controller`)
	}

	service, present := options["service"]
	if _, ok := service.(string); !present || !ok {
		return nil, fmt.Errorf(`"service" must be set for silly access controller`)
	}

	return &accessController{realm: realm.(string), service: service.(string)}, nil
}

// Authorized simply checks for the existence of the authorization header,
// responding with a bearer challenge if it doesn't exist.
func (ac *accessController) Authorized(ctx context.Context, resource auth.Resource, actions ...string) (context.Context, error) {
	req, err := context.GetRequest(ctx)
	if err != nil {
		return nil, err
	}

	if req.Header.Get("Authorization") == "" {
		authnErr := &authenticationError{
			realm:   ac.realm,
			service: ac.service,
		}

		if len(actions) > 0 {
			combinedActions := strings.Join(actions, ",")
			authnErr.scope = fmt.Sprintf("%s:%s:%s", resource.Type, resource.Name, combinedActions)
		}

		return nil, authnErr
	}

	return auth.WithUser(ctx, auth.UserInfo{Name: "silly"}), nil
}

type authenticationError struct {
	realm   string
	service string
	scope   string
}

var _ auth.AuthenticationError = &authenticationError{}

// SetChallengeHeaders sets a simple bearer challenge on the response header.
func (ae *authenticationError) SetChallengeHeaders(h http.Header) {
	header := fmt.Sprintf("Bearer realm=%q,service=%q", ae.realm, ae.service)

	if ae.scope != "" {
		header = fmt.Sprintf("%s,scope=%q", header, ae.scope)
	}

	h.Set("WWW-Authenticate", header)
}

// AuthenticationErrorDetails is no different than the regular Error method.
func (ae *authenticationError) AuthenticationErrorDetails() interface{} {
	return ae.Error()
}

func (ae *authenticationError) Error() string {
	return fmt.Sprintf("silly authentication error: %#v", ae)
}

// init registers the silly auth backend.
func init() {
	auth.Register("silly", auth.InitFunc(newAccessController))
}

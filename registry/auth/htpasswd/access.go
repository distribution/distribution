// Package htpasswd provides a simple authentication scheme that checks for the
// user credential hash in an htpasswd formatted file in a configuration-determined
// location.
//
// This authentication method MUST be used under TLS, as simple token-replay attack is possible.
package htpasswd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"
)

var (
	// ErrAuthenticationRequired is returned when credentials are not
	// provided.
	ErrAuthenticationRequired = errors.New("authentication required")
	// ErrInvalidCredentials is returned when the provided credentials are
	// not valid.
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type accessController struct {
	realm    string
	htpasswd *htpasswd
}

var _ auth.AccessController = &accessController{}

func newAccessController(options map[string]interface{}) (auth.AccessController, error) {
	realm, present := options["realm"]
	if _, ok := realm.(string); !present || !ok {
		return nil, fmt.Errorf(`"realm" must be set for htpasswd access controller`)
	}

	path, present := options["path"]
	if _, ok := path.(string); !present || !ok {
		return nil, fmt.Errorf(`"path" must be set for htpasswd access controller`)
	}

	f, err := os.Open(path.(string))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h, err := newHTPasswd(f)
	if err != nil {
		return nil, err
	}

	return &accessController{realm: realm.(string), htpasswd: h}, nil
}

func (ac *accessController) Authorized(ctx context.Context, resource auth.Resource, actions ...string) (context.Context, error) {
	req, err := context.GetRequest(ctx)
	if err != nil {
		return nil, err
	}

	username, password, ok := req.BasicAuth()
	if !ok {
		return nil, &authenticationError{
			realm: ac.realm,
			err:   ErrAuthenticationRequired,
		}
	}

	if err := ac.htpasswd.authenticateUser(username, password); err != nil {
		context.GetLogger(ctx).Errorf("error authenticating user %q: %v", username, err)
		return nil, &authenticationError{
			realm: ac.realm,
			err:   ErrInvalidCredentials,
		}
	}

	return auth.WithUser(ctx, auth.UserInfo{Name: username}), nil
}

// authenticationError implements the auth.Challenge interface.
type authenticationError struct {
	realm string
	err   error
}

var _ auth.AuthenticationError = &authenticationError{}

// SetChallengeHeaders sets the basic challenge header..
func (ae *authenticationError) SetChallengeHeaders(h http.Header) {
	h.Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", ae.realm))
}

// AuthenticationErrorDetails is no different than the regular Error method.
func (ae *authenticationError) AuthenticationErrorDetails() interface{} {
	return ae.Error()
}

func (ae *authenticationError) Error() string {
	return ae.err.Error()
}

func init() {
	auth.Register("htpasswd", auth.InitFunc(newAccessController))
}

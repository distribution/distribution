// Package basic provides a simple authentication scheme that checks for the
// user credential hash in an htpasswd formatted file in a configuration-determined
// location.
//
// The use of SHA hashes (htpasswd -s) is enforced since MD5 is insecure and simple
// system crypt() may be as well.
//
// This authentication method MUST be used under TLS, as simple token-replay attack is possible.
package basic

import (
	"errors"
	"fmt"
	"net/http"

	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"
	"golang.org/x/net/context"
)

type accessController struct {
	realm    string
	htpasswd *HTPasswd
}

type challenge struct {
	realm string
	err   error
}

var _ auth.AccessController = &accessController{}
var (
	// ErrPasswordRequired - returned when no auth token is given.
	ErrPasswordRequired = errors.New("authorization credential required")
	// ErrInvalidCredential - returned when the auth token does not authenticate correctly.
	ErrInvalidCredential = errors.New("invalid authorization credential")
)

func newAccessController(options map[string]interface{}) (auth.AccessController, error) {
	realm, present := options["realm"]
	if _, ok := realm.(string); !present || !ok {
		return nil, fmt.Errorf(`"realm" must be set for basic access controller`)
	}

	path, present := options["path"]
	if _, ok := path.(string); !present || !ok {
		return nil, fmt.Errorf(`"path" must be set for basic access controller`)
	}

	return &accessController{realm: realm.(string), htpasswd: NewHTPasswd(path.(string))}, nil
}

func (ac *accessController) Authorized(ctx context.Context, accessRecords ...auth.Access) (context.Context, error) {
	req, err := ctxu.GetRequest(ctx)
	if err != nil {
		return nil, err
	}

	authHeader := req.Header.Get("Authorization") 
	if authHeader == "" {
		challenge := challenge{
			realm: ac.realm,
		}
		return nil, &challenge
	}

	user, pass, ok := req.BasicAuth()
	if !ok {
		return nil, errors.New("Invalid Authorization header")
	}
		
	if res, _ := ac.htpasswd.AuthenticateUser(user, pass); !res {
		challenge := challenge{
			realm: ac.realm,
		}
		challenge.err = ErrInvalidCredential
		return nil, &challenge
	}

	return auth.WithUser(ctx, auth.UserInfo{Name: user}), nil
}

func (ch *challenge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := fmt.Sprintf("Basic realm=%q", ch.realm)
	w.Header().Set("WWW-Authenticate", header)
	w.WriteHeader(http.StatusUnauthorized)
}

func (ch *challenge) Error() string {
	return fmt.Sprintf("basic authentication challenge: %#v", ch)
}

func init() {
	auth.Register("basic", auth.InitFunc(newAccessController))
}

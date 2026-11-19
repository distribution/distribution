package rbac

import (
	"fmt"
	"net/http"

	"github.com/distribution/distribution/v3/registry/auth"
	"github.com/sirupsen/logrus"
)

func init() {
	if err := auth.Register("rbac", auth.InitFunc(newAccessController)); err != nil {
		logrus.Errorf("failed to register rbac auth: %v", err)
	}
}

// accessController authenticates users against the RBAC policy file and
// authorizes every requested repository access against the user's role.
//
//	auth:
//	  rbac:
//	    realm: Registry Realm
//	    policy: /auth/rbac_policy.yml
type accessController struct {
	realm string
	store *Store
}

var _ auth.AccessController = &accessController{}

func newAccessController(options map[string]any) (auth.AccessController, error) {
	realm, ok := options["realm"].(string)
	if !ok || realm == "" {
		return nil, fmt.Errorf(`"realm" must be set for rbac access controller`)
	}
	path, ok := options["policy"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf(`"policy" must be set for rbac access controller`)
	}
	store, err := SharedStore(path)
	if err != nil {
		return nil, err
	}
	return &accessController{realm: realm, store: store}, nil
}

// Authorized implements auth.AccessController. Denials are returned as
// challenges because the registry turns every other error type into a bare 400.
func (ac *accessController) Authorized(req *http.Request, access ...auth.Access) (*auth.Grant, error) {
	username, password, ok := req.BasicAuth()
	if !ok {
		return nil, &challenge{realm: ac.realm, err: auth.ErrInvalidCredential}
	}

	policy, err := ac.store.Get()
	if err != nil {
		return nil, err // not a Challenge: registry answers 400 and logs the cause
	}

	if !policy.Authenticate(username, password) {
		return nil, &challenge{realm: ac.realm, err: auth.ErrAuthenticationFailure}
	}

	for _, a := range access {
		switch a.Type {
		case "repository":
			if !policy.IsAllowed(username, a.Name, Action(a.Action)) {
				return nil, &challenge{realm: ac.realm, err: fmt.Errorf("%w: %s %s denied for %q", auth.ErrAuthenticationFailure, a.Action, a.Name, username)}
			}
		case "registry":
			// Catalog: any authenticated user may call it; the registry
			// middleware filters the result down to what the user may pull.
		default:
			return nil, &challenge{realm: ac.realm, err: fmt.Errorf("%w: unsupported resource type %q", auth.ErrAuthenticationFailure, a.Type)}
		}
	}

	return &auth.Grant{User: auth.UserInfo{Name: username}}, nil
}

type challenge struct {
	realm string
	err   error
}

var _ auth.Challenge = challenge{}

func (ch challenge) SetHeaders(r *http.Request, w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf("Basic realm=%q", ch.realm))
}

func (ch challenge) Error() string {
	return fmt.Sprintf("basic authentication challenge for realm %q: %s", ch.realm, ch.err)
}

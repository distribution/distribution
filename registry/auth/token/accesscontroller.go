package token

import (
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/distribution/distribution/v3/registry/auth"
	"github.com/go-jose/go-jose/v3"
	"github.com/sirupsen/logrus"
)

// init handles registering the token auth backend.
func init() {
	if err := auth.Register("token", auth.InitFunc(newAccessController)); err != nil {
		logrus.Errorf("tailed to register token auth: %v", err)
	}
}

// accessSet maps a typed, named resource to
// a set of actions requested or authorized.
type accessSet map[auth.Resource]actionSet

// newAccessSet constructs an accessSet from
// a variable number of auth.Access items.
func newAccessSet(accessItems ...auth.Access) accessSet {
	accessSet := make(accessSet, len(accessItems))

	for _, access := range accessItems {
		resource := auth.Resource{
			Type: access.Type,
			Name: access.Name,
		}

		set, exists := accessSet[resource]
		if !exists {
			set = newActionSet()
			accessSet[resource] = set
		}

		set.add(access.Action)
	}

	return accessSet
}

// contains returns whether or not the given access is in this accessSet.
func (s accessSet) contains(access auth.Access) bool {
	actionSet, ok := s[access.Resource]
	if ok {
		return actionSet.contains(access.Action)
	}

	return false
}

// scopeParam returns a collection of scopes which can
// be used for a WWW-Authenticate challenge parameter.
// See https://tools.ietf.org/html/rfc6750#section-3
func (s accessSet) scopeParam() string {
	scopes := make([]string, 0, len(s))

	for resource, actionSet := range s {
		actions := strings.Join(actionSet.keys(), ",")
		scopes = append(scopes, fmt.Sprintf("%s:%s:%s", resource.Type, resource.Name, actions))
	}

	return strings.Join(scopes, " ")
}

// Errors used and exported by this package.
var (
	ErrInsufficientScope = errors.New("insufficient scope")
	ErrTokenRequired     = errors.New("authorization token required")
)

// authChallenge implements the auth.Challenge interface.
type authChallenge struct {
	err          error
	realm        string
	autoRedirect bool
	service      string
	accessSet    accessSet
}

var _ auth.Challenge = authChallenge{}

// Error returns the internal error string for this authChallenge.
func (ac authChallenge) Error() string {
	return ac.err.Error()
}

// Status returns the HTTP Response Status Code for this authChallenge.
func (ac authChallenge) Status() int {
	return http.StatusUnauthorized
}

// challengeParams constructs the value to be used in
// the WWW-Authenticate response challenge header.
// See https://tools.ietf.org/html/rfc6750#section-3
func (ac authChallenge) challengeParams(r *http.Request) string {
	var realm string
	if ac.autoRedirect {
		realm = fmt.Sprintf("https://%s/auth/token", r.Host)
	} else {
		realm = ac.realm
	}
	str := fmt.Sprintf("Bearer realm=%q,service=%q", realm, ac.service)

	if scope := ac.accessSet.scopeParam(); scope != "" {
		str = fmt.Sprintf("%s,scope=%q", str, scope)
	}

	if ac.err == ErrInvalidToken || ac.err == ErrMalformedToken {
		str = fmt.Sprintf("%s,error=%q", str, "invalid_token")
	} else if ac.err == ErrInsufficientScope {
		str = fmt.Sprintf("%s,error=%q", str, "insufficient_scope")
	}

	return str
}

// SetChallenge sets the WWW-Authenticate value for the response.
func (ac authChallenge) SetHeaders(r *http.Request, w http.ResponseWriter) {
	w.Header().Add("WWW-Authenticate", ac.challengeParams(r))
}

// accessController implements the auth.AccessController interface.
type accessController struct {
	realm        string
	autoRedirect bool
	issuer       string
	service      string
	rootCerts    *x509.CertPool
	trustedKeys  map[string]crypto.PublicKey
}

// tokenAccessOptions is a convenience type for handling
// options to the contstructor of an accessController.
type tokenAccessOptions struct {
	realm          string
	autoRedirect   bool
	issuer         string
	service        string
	rootCertBundle string
	jwks           string
}

// checkOptions gathers the necessary options
// for an accessController from the given map.
func checkOptions(options map[string]interface{}) (tokenAccessOptions, error) {
	var opts tokenAccessOptions

	keys := []string{"realm", "issuer", "service", "rootcertbundle", "jwks"}
	vals := make([]string, 0, len(keys))
	for _, key := range keys {
		val, ok := options[key].(string)
		if !ok {
			// NOTE(milosgajdos): this func makes me intensely sad
			// just like all the other weakly typed config options.
			// Either of these config options may be missing, but
			// at least one must be present: we handle those cases
			// in newAccessController func which consumes this one.
			if key == "rootcertbundle" || key == "jwks" {
				vals = append(vals, "")
				continue
			}
			return opts, fmt.Errorf("token auth requires a valid option string: %q", key)
		}
		vals = append(vals, val)
	}

	opts.realm, opts.issuer, opts.service, opts.rootCertBundle, opts.jwks = vals[0], vals[1], vals[2], vals[3], vals[4]

	autoRedirectVal, ok := options["autoredirect"]
	if ok {
		autoRedirect, ok := autoRedirectVal.(bool)
		if !ok {
			return opts, fmt.Errorf("token auth requires a valid option bool: autoredirect")
		}
		opts.autoRedirect = autoRedirect
	}

	return opts, nil
}

func getRootCerts(path string) ([]*x509.Certificate, error) {
	fp, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open token auth root certificate bundle file %q: %s", path, err)
	}
	defer fp.Close()

	rawCertBundle, err := io.ReadAll(fp)
	if err != nil {
		return nil, fmt.Errorf("unable to read token auth root certificate bundle file %q: %s", path, err)
	}

	var rootCerts []*x509.Certificate
	pemBlock, rawCertBundle := pem.Decode(rawCertBundle)
	for pemBlock != nil {
		if pemBlock.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(pemBlock.Bytes)
			if err != nil {
				return nil, fmt.Errorf("unable to parse token auth root certificate: %s", err)
			}

			rootCerts = append(rootCerts, cert)
		}

		pemBlock, rawCertBundle = pem.Decode(rawCertBundle)
	}

	return rootCerts, nil
}

func getJwks(path string) (*jose.JSONWebKeySet, error) {
	// TODO(milosgajdos): we should consider providing a JWKS
	// URL from which the JWKS could be fetched
	jp, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open jwks file %q: %s", path, err)
	}
	defer jp.Close()

	rawJWKS, err := io.ReadAll(jp)
	if err != nil {
		return nil, fmt.Errorf("unable to read token jwks file %q: %s", path, err)
	}

	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal(rawJWKS, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse jwks: %v", err)
	}

	return &jwks, nil
}

// newAccessController creates an accessController using the given options.
func newAccessController(options map[string]interface{}) (auth.AccessController, error) {
	config, err := checkOptions(options)
	if err != nil {
		return nil, err
	}

	var (
		rootCerts []*x509.Certificate
		jwks      *jose.JSONWebKeySet
	)

	if config.rootCertBundle != "" {
		rootCerts, err = getRootCerts(config.rootCertBundle)
		if err != nil {
			return nil, err
		}
	}

	if config.jwks != "" {
		jwks, err = getJwks(config.jwks)
		if err != nil {
			return nil, err
		}
	}

	if (len(rootCerts) == 0 && jwks == nil) || // no certs bundle and no jwks
		(len(rootCerts) == 0 && jwks != nil && len(jwks.Keys) == 0) { // no certs bundle and empty jwks
		return nil, errors.New("token auth requires at least one token signing key")
	}

	rootPool := x509.NewCertPool()
	for _, rootCert := range rootCerts {
		rootPool.AddCert(rootCert)
	}

	trustedKeys := make(map[string]crypto.PublicKey)
	if jwks != nil {
		for _, key := range jwks.Keys {
			trustedKeys[key.KeyID] = key.Public()
		}
	}

	return &accessController{
		realm:        config.realm,
		autoRedirect: config.autoRedirect,
		issuer:       config.issuer,
		service:      config.service,
		rootCerts:    rootPool,
		trustedKeys:  trustedKeys,
	}, nil
}

// Authorized handles checking whether the given request is authorized
// for actions on resources described by the given access items.
func (ac *accessController) Authorized(req *http.Request, accessItems ...auth.Access) (*auth.Grant, error) {
	challenge := &authChallenge{
		realm:        ac.realm,
		autoRedirect: ac.autoRedirect,
		service:      ac.service,
		accessSet:    newAccessSet(accessItems...),
	}

	prefix, rawToken, ok := strings.Cut(req.Header.Get("Authorization"), " ")
	if !ok || rawToken == "" || !strings.EqualFold(prefix, "bearer") {
		challenge.err = ErrTokenRequired
		return nil, challenge
	}

	token, err := NewToken(rawToken)
	if err != nil {
		challenge.err = err
		return nil, challenge
	}

	verifyOpts := VerifyOptions{
		TrustedIssuers:    []string{ac.issuer},
		AcceptedAudiences: []string{ac.service},
		Roots:             ac.rootCerts,
		TrustedKeys:       ac.trustedKeys,
	}

	claims, err := token.Verify(verifyOpts)
	if err != nil {
		challenge.err = err
		return nil, challenge
	}

	accessSet := claims.accessSet()
	for _, access := range accessItems {
		if !accessSet.contains(access) {
			challenge.err = ErrInsufficientScope
			return nil, challenge
		}
	}

	return &auth.Grant{
		User:      auth.UserInfo{Name: claims.Subject},
		Resources: claims.resources(),
	}, nil
}

package token

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"
	"github.com/docker/libtrust"
)

// scopeParam returns a scope parameter which can be
// used in a WWW-Authenticate challenge header.
// See https://tools.ietf.org/html/rfc6750#section-3
func scopeParam(resource auth.Resource, actions actionSet) string {
	combinedActions := strings.Join(actions.keys(), ",")
	if combinedActions == "" {
		return ""
	}

	return fmt.Sprintf("%s:%s:%s", resource.Type, resource.Name, combinedActions)
}

var errTokenRequired = errors.New("authorization token required")

// authenticationError implements the auth.AuthenticationError interface.
type verificationError struct {
	err      error
	realm    string
	service  string
	resource auth.Resource
	actions  actionSet
}

var _ auth.AuthenticationError = &verificationError{}

// Error returns the internal error string for this verificationError.
func (ve *verificationError) Error() string {
	return ve.err.Error()
}

// challengeParams constructs the value to be used in
// the WWW-Authenticate response challenge header.
// See https://tools.ietf.org/html/rfc6750#section-3
func (ve *verificationError) challengeParams() string {
	str := fmt.Sprintf("Bearer realm=%q,service=%q", ve.realm, ve.service)

	if scope := scopeParam(ve.resource, ve.actions); scope != "" {
		str = fmt.Sprintf("%s,scope=%q", str, scope)
	}

	if ve.err != errTokenRequired {
		str = fmt.Sprintf("%s,error=%q,error_description=%q", str, "invalid token", ve.err.Error())
	}

	return str
}

// AuthenticationErrorDetails is no different than the regular Error method.
func (ve *verificationError) AuthenticationErrorDetails() interface{} {
	return ve.Error()
}

// SetChallengeHeaders sets the WWW-Authenticate value for the response.
func (ve *verificationError) SetChallengeHeaders(h http.Header) {
	h.Add("WWW-Authenticate", ve.challengeParams())
}

// authorizationError implements the auth.AuthorizationError interface.
type authorizationError struct {
	requestedActions actionSet
	grantedActions   actionSet
}

var _ auth.AuthorizationError = &authorizationError{}

// Error returns a string describing this authorizationError.
func (ae *authorizationError) Error() string {
	return fmt.Sprintf("requested actions: %q authorized actions: %q", ae.requestedActions.keys(), ae.grantedActions.keys())
}

// AuthorizationErrorDetails is no different than the regular Error method.
func (ae *authorizationError) AuthorizationErrorDetails() interface{} {
	return ae.Error()
}

// ResourceHidden returns whether the existence of the requested resource
// should be exposed to the client. If the client was granted no actions to the
// requested resource then it should be hidden.
func (ae *authorizationError) ResourceHidden() bool {
	return len(ae.grantedActions.stringSet) == 0
}

// accessController implements the auth.AccessController interface.
type accessController struct {
	realm       string
	issuer      string
	service     string
	rootCerts   *x509.CertPool
	trustedKeys map[string]libtrust.PublicKey
}

// tokenAccessOptions is a convenience type for handling
// options to the contstructor of an accessController.
type tokenAccessOptions struct {
	realm          string
	issuer         string
	service        string
	rootCertBundle string
}

// checkOptions gathers the necessary options
// for an accessController from the given map.
func checkOptions(options map[string]interface{}) (tokenAccessOptions, error) {
	var opts tokenAccessOptions

	keys := []string{"realm", "issuer", "service", "rootcertbundle"}
	vals := make([]string, 0, len(keys))
	for _, key := range keys {
		val, ok := options[key].(string)
		if !ok {
			return opts, fmt.Errorf("token auth requires a valid option string: %q", key)
		}
		vals = append(vals, val)
	}

	opts.realm, opts.issuer, opts.service, opts.rootCertBundle = vals[0], vals[1], vals[2], vals[3]

	return opts, nil
}

// newAccessController creates an accessController using the given options.
func newAccessController(options map[string]interface{}) (auth.AccessController, error) {
	config, err := checkOptions(options)
	if err != nil {
		return nil, err
	}

	fp, err := os.Open(config.rootCertBundle)
	if err != nil {
		return nil, fmt.Errorf("unable to open token auth root certificate bundle file %q: %s", config.rootCertBundle, err)
	}
	defer fp.Close()

	rawCertBundle, err := ioutil.ReadAll(fp)
	if err != nil {
		return nil, fmt.Errorf("unable to read token auth root certificate bundle file %q: %s", config.rootCertBundle, err)
	}

	var rootCerts []*x509.Certificate
	pemBlock, rawCertBundle := pem.Decode(rawCertBundle)
	for pemBlock != nil {
		cert, err := x509.ParseCertificate(pemBlock.Bytes)
		if err != nil {
			return nil, fmt.Errorf("unable to parse token auth root certificate: %s", err)
		}

		rootCerts = append(rootCerts, cert)

		pemBlock, rawCertBundle = pem.Decode(rawCertBundle)
	}

	if len(rootCerts) == 0 {
		return nil, errors.New("token auth requires at least one token signing root certificate")
	}

	rootPool := x509.NewCertPool()
	trustedKeys := make(map[string]libtrust.PublicKey, len(rootCerts))
	for _, rootCert := range rootCerts {
		rootPool.AddCert(rootCert)
		pubKey, err := libtrust.FromCryptoPublicKey(crypto.PublicKey(rootCert.PublicKey))
		if err != nil {
			return nil, fmt.Errorf("unable to get public key from token auth root certificate: %s", err)
		}
		trustedKeys[pubKey.KeyID()] = pubKey
	}

	return &accessController{
		realm:       config.realm,
		issuer:      config.issuer,
		service:     config.service,
		rootCerts:   rootPool,
		trustedKeys: trustedKeys,
	}, nil
}

// Authorized handles checking whether the given request is authorized
// for actions on resources described by the given access items.
func (ac *accessController) Authorized(ctx context.Context, resource auth.Resource, actions ...string) (context.Context, error) {
	var (
		requestedActions = newActionSet(actions...)
		verificationErr  = &verificationError{
			realm:    ac.realm,
			service:  ac.service,
			resource: resource,
			actions:  requestedActions,
		}
	)

	req, err := context.GetRequest(ctx)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(req.Header.Get("Authorization"), " ")

	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		verificationErr.err = errTokenRequired
		return nil, verificationErr
	}

	rawToken := parts[1]

	token, err := NewToken(rawToken)
	if err != nil {
		verificationErr.err = err
		return nil, verificationErr
	}

	verifyOpts := VerifyOptions{
		TrustedIssuers:    newStringSet(ac.issuer),
		AcceptedAudiences: newStringSet(ac.service),
		Roots:             ac.rootCerts,
		TrustedKeys:       ac.trustedKeys,
	}

	if err = token.Verify(verifyOpts); err != nil {
		verificationErr.err = err
		return nil, verificationErr
	}

	grantedActions := token.accessSet()[resource]
	for requestedAction := range requestedActions.stringSet {
		if !grantedActions.contains(requestedAction) {
			// The client is not granted access to perform this
			// action.
			return nil, &authorizationError{
				requestedActions: requestedActions,
				grantedActions:   grantedActions,
			}
		}
	}

	return auth.WithUser(ctx, auth.UserInfo{Name: token.Claims.Subject}), nil
}

// init handles registering the token auth backend.
func init() {
	auth.Register("token", auth.InitFunc(newAccessController))
}

package token

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/distribution/distribution/v3/registry/auth"
	"github.com/go-jose/go-jose/v4"
	"github.com/sirupsen/logrus"
)

// init handles registering the token auth backend.
func init() {
	if err := auth.Register("token", auth.InitFunc(newAccessController)); err != nil {
		logrus.Errorf("failed to register token auth: %v", err)
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
	err              error
	realm            string
	autoRedirect     bool
	autoRedirectPath string
	service          string
	accessSet        accessSet
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

func buildAutoRedirectURL(r *http.Request, autoRedirectPath string) string {
	scheme := "https"

	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); len(forwardedProto) > 0 {
		scheme = forwardedProto
	}

	u := &url.URL{
		Scheme: scheme,
		Host:   r.Host,
		Path:   autoRedirectPath,
	}
	return u.String()
}

// challengeParams constructs the value to be used in
// the WWW-Authenticate response challenge header.
// See https://tools.ietf.org/html/rfc6750#section-3
func (ac authChallenge) challengeParams(r *http.Request) string {
	var realm string
	if ac.autoRedirect {
		realm = buildAutoRedirectURL(r, ac.autoRedirectPath)
	} else {
		realm = ac.realm
	}
	str := fmt.Sprintf("Bearer realm=%q,service=%q", realm, ac.service)

	if scope := ac.accessSet.scopeParam(); scope != "" {
		str = fmt.Sprintf("%s,scope=%q", str, scope)
	}

	switch ac.err {
	case ErrInvalidToken, ErrMalformedToken:
		str = fmt.Sprintf("%s,error=%q", str, "invalid_token")
	case ErrInsufficientScope:
		str = fmt.Sprintf("%s,error=%q", str, "insufficient_scope")
	}

	return str
}

// SetHeaders sets the WWW-Authenticate value for the response.
func (ac authChallenge) SetHeaders(r *http.Request, w http.ResponseWriter) {
	w.Header().Add("WWW-Authenticate", ac.challengeParams(r))
}

// accessController implements the auth.AccessController interface.
type accessController struct {
	realm               string
	autoRedirect        bool
	autoRedirectPath    string
	issuer              string
	service             string
	rootCerts           *x509.CertPool
	mu                  sync.RWMutex
	trustedKeys         map[string]crypto.PublicKey
	signingAlgorithms   []jose.SignatureAlgorithm
	jwksSource          string
	jwksRefreshInterval time.Duration
	cancel              context.CancelFunc
}

const (
	defaultAutoRedirectPath    = "/auth/token"
	defaultJWKSRefreshInterval = 1 * time.Hour
	defaultJWKSRefreshTimeout  = 10 * time.Second
)

// tokenAccessOptions is a convenience type for handling
// options to the constructor of an accessController.
type tokenAccessOptions struct {
	realm                  string
	autoRedirect           bool
	autoRedirectPath       string
	issuer                 string
	service                string
	rootCertBundle         string
	jwks                   string
	jwksRefreshInterval    time.Duration
	jwksRefreshIntervalSet bool // true when jwksrefreshinterval was explicitly configured
	signingAlgorithms      []string
}

// checkOptions gathers the necessary options
// for an accessController from the given map.
func checkOptions(options map[string]any) (tokenAccessOptions, error) {
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
			return tokenAccessOptions{}, fmt.Errorf("token auth requires a valid option string: %q", key)
		}
		vals = append(vals, val)
	}

	opts.realm, opts.issuer, opts.service, opts.rootCertBundle, opts.jwks = vals[0], vals[1], vals[2], vals[3], vals[4]

	if strings.Contains(opts.jwks, "://") {
		if !strings.HasPrefix(opts.jwks, "http://") && !strings.HasPrefix(opts.jwks, "https://") {
			return tokenAccessOptions{}, fmt.Errorf("invalid jwks URL %q: only http and https schemes are supported", opts.jwks)
		}
		u, err := url.ParseRequestURI(opts.jwks)
		if err != nil || u.Host == "" {
			return tokenAccessOptions{}, fmt.Errorf("invalid jwks URL %q: must be a valid http/https URL", opts.jwks)
		}
	}

	autoRedirectVal, ok := options["autoredirect"]
	if ok {
		autoRedirect, ok := autoRedirectVal.(bool)
		if !ok {
			return tokenAccessOptions{}, errors.New("token auth requires a valid option bool: autoredirect")
		}
		opts.autoRedirect = autoRedirect
	}
	if opts.autoRedirect {
		autoRedirectPathVal, ok := options["autoredirectpath"]
		if ok {
			autoRedirectPath, ok := autoRedirectPathVal.(string)
			if !ok {
				return tokenAccessOptions{}, errors.New("token auth requires a valid option string: autoredirectpath")
			}
			opts.autoRedirectPath = autoRedirectPath
		}
		if opts.autoRedirectPath == "" {
			opts.autoRedirectPath = defaultAutoRedirectPath
		}
	}

	if intervalStr, ok := options["jwksrefreshinterval"]; ok {
		interval, err := time.ParseDuration(intervalStr.(string))
		if err != nil {
			return tokenAccessOptions{}, fmt.Errorf("invalid jwksrefreshinterval: %v", err)
		}
		opts.jwksRefreshInterval = interval
		opts.jwksRefreshIntervalSet = true
	}

	signingAlgos, ok := options["signingalgorithms"]
	if ok {
		signingAlgorithmsVals, ok := signingAlgos.([]any)
		if !ok {
			return tokenAccessOptions{}, errors.New("signingalgorithms must be a list of signing algorithms")
		}

		for _, signingAlgorithmVal := range signingAlgorithmsVals {
			signingAlgorithm, ok := signingAlgorithmVal.(string)
			if !ok {
				return tokenAccessOptions{}, errors.New("signingalgorithms must be a list of signing algorithms")
			}

			opts.signingAlgorithms = append(opts.signingAlgorithms, signingAlgorithm)
		}
	}

	return opts, nil
}

var (
	rootCertFetcher func(string) ([]*x509.Certificate, error) = getRootCerts
	jwkFetcher      func(string) (*jose.JSONWebKeySet, error) = getJwks
)

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

func getJwks(source string) (*jose.JSONWebKeySet, error) {
	var rawJWKS []byte

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: defaultJWKSRefreshTimeout}
		resp, err := client.Get(source)
		if err != nil {
			return nil, fmt.Errorf("unable to fetch JWKS from %q: %s", source, err)
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d fetching JWKS from %q", resp.StatusCode, source)
		}

		rawJWKS, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unable to read JWKS response from %q: %s", source, err)
		}
	} else {
		jp, err := os.Open(source)
		if err != nil {
			return nil, fmt.Errorf("unable to open jwks file %q: %s", source, err)
		}

		defer jp.Close()
		rawJWKS, err = io.ReadAll(jp)
		if err != nil {
			return nil, fmt.Errorf("unable to read token jwks file %q: %s", source, err)
		}
	}

	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal(rawJWKS, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse jwks: %v", err)
	}

	return &jwks, nil
}

func getSigningAlgorithms(algos []string) ([]jose.SignatureAlgorithm, error) {
	signAlgVals := make([]jose.SignatureAlgorithm, 0, len(algos))
	for _, alg := range algos {
		signAlg, ok := signingAlgorithms[alg]
		if !ok {
			return nil, fmt.Errorf("unsupported signing algorithm: %s", alg)
		}
		signAlgVals = append(signAlgVals, signAlg)
	}
	return signAlgVals, nil
}

// newAccessController creates an accessController using the given options.
func newAccessController(options map[string]any) (auth.AccessController, error) {
	config, err := checkOptions(options)
	if err != nil {
		return nil, err
	}

	var (
		rootCerts []*x509.Certificate
		jwks      *jose.JSONWebKeySet
		signAlgos []jose.SignatureAlgorithm
	)

	if config.rootCertBundle != "" {
		rootCerts, err = rootCertFetcher(config.rootCertBundle)
		if err != nil {
			return nil, err
		}
	}

	if config.jwks != "" {
		jwks, err = jwkFetcher(config.jwks)
		if err != nil {
			return nil, err
		}
	}

	if (len(rootCerts) == 0 && jwks == nil) || // no certs bundle and no jwks
		(len(rootCerts) == 0 && jwks != nil && len(jwks.Keys) == 0) { // no certs bundle and empty jwks
		return nil, errors.New("token auth requires at least one token signing key")
	}

	trustedKeys := make(map[string]crypto.PublicKey)
	rootPool := x509.NewCertPool()
	for _, rootCert := range rootCerts {
		rootPool.AddCert(rootCert)
		if key := GetJWKThumbprint(rootCert.PublicKey); key != "" {
			trustedKeys[key] = rootCert.PublicKey
		}
	}

	if jwks != nil {
		for _, key := range jwks.Keys {
			trustedKeys[key.KeyID] = key.Public()
		}
	}

	signAlgos, err = getSigningAlgorithms(config.signingAlgorithms)
	if err != nil {
		return nil, err
	}
	if len(signAlgos) == 0 {
		// NOTE: this is to maintain backwards compat
		// with existing registry deployments
		signAlgos = defaultSigningAlgorithms
	}

	refreshInterval := config.jwksRefreshInterval
	if config.jwks != "" && !config.jwksRefreshIntervalSet {
		refreshInterval = defaultJWKSRefreshInterval
	}

	ac := &accessController{
		realm:               config.realm,
		autoRedirect:        config.autoRedirect,
		autoRedirectPath:    config.autoRedirectPath,
		issuer:              config.issuer,
		service:             config.service,
		rootCerts:           rootPool,
		trustedKeys:         trustedKeys,
		signingAlgorithms:   signAlgos,
		jwksSource:          config.jwks,
		jwksRefreshInterval: refreshInterval,
	}

	if ac.jwksSource != "" && ac.jwksRefreshInterval > 0 {
		ctx, cancel := context.WithCancel(context.Background())
		ac.cancel = cancel
		ac.startRefresh(ctx)
	}

	return ac, nil
}

// refreshKeys fetches the JWKS from jwksSource and atomically replaces the
// trusted key set. On failure the existing keys are preserved.
func (ac *accessController) refreshKeys() error {
	jwks, err := jwkFetcher(ac.jwksSource)
	if err != nil {
		return err
	}

	newKeys := make(map[string]crypto.PublicKey, len(jwks.Keys))
	for _, key := range jwks.Keys {
		newKeys[key.KeyID] = key.Public()
	}

	ac.mu.Lock()
	ac.trustedKeys = newKeys
	ac.mu.Unlock()
	logrus.Debugf("token auth: refreshed %d JWKS keys from %q", len(newKeys), ac.jwksSource)
	return nil
}

// startRefresh spawns a goroutine that periodically calls refreshKeys.
// On fetch failure the existing keys are kept and the error is logged.
func (ac *accessController) startRefresh(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(ac.jwksRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := ac.refreshKeys(); err != nil {
					logrus.Errorf("token auth: failed to refresh JWKS from %q: %v", ac.jwksSource, err)
				}
			}
		}
	}()
}

// Close stops the background JWKS refresh goroutine if one was started.
func (ac *accessController) Close() error {
	if ac.cancel != nil {
		ac.cancel()
	}
	return nil
}

// Authorized handles checking whether the given request is authorized
// for actions on resources described by the given access items.
func (ac *accessController) Authorized(req *http.Request, accessItems ...auth.Access) (*auth.Grant, error) {
	challenge := &authChallenge{
		realm:            ac.realm,
		autoRedirect:     ac.autoRedirect,
		autoRedirectPath: ac.autoRedirectPath,
		service:          ac.service,
		accessSet:        newAccessSet(accessItems...),
	}

	prefix, rawToken, ok := strings.Cut(req.Header.Get("Authorization"), " ")
	if !ok || rawToken == "" || !strings.EqualFold(prefix, "bearer") {
		challenge.err = ErrTokenRequired
		return nil, challenge
	}

	token, err := NewToken(rawToken, ac.signingAlgorithms)
	if err != nil {
		challenge.err = err
		return nil, challenge
	}

	ac.mu.RLock()
	trustedKeys := ac.trustedKeys
	ac.mu.RUnlock()

	verifyOpts := VerifyOptions{
		TrustedIssuers:    []string{ac.issuer},
		AcceptedAudiences: []string{ac.service},
		Roots:             ac.rootCerts,
		TrustedKeys:       trustedKeys,
	}

	claims, err := token.Verify(verifyOpts)
	if err != nil {
		// If the key ID is not in our current set and we have a remote source, re-fetch
		if errors.Is(err, ErrUnknownKeyID) && ac.jwksSource != "" {
			if refreshErr := ac.refreshKeys(); refreshErr != nil {
				logrus.Errorf("token auth: on-demand JWKS refresh from %q failed: %v", ac.jwksSource, refreshErr)
			} else {
				ac.mu.RLock()
				verifyOpts.TrustedKeys = ac.trustedKeys
				ac.mu.RUnlock()
				claims, err = token.Verify(verifyOpts)
			}
		}
		if err != nil {
			// Normalize ErrUnknownKeyID so the WWW-Authenticate header
			if errors.Is(err, ErrUnknownKeyID) {
				err = ErrInvalidToken
			}
			challenge.err = err
			return nil, challenge
		}
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

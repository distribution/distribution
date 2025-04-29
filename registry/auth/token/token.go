package token

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	log "github.com/sirupsen/logrus"

	"github.com/distribution/distribution/v3/registry/auth"
)

const (
	// TokenSeparator is the value which separates the header, claims, and
	// signature in the compact serialization of a JSON Web Token.
	TokenSeparator = "."
	// Leeway is the Duration that will be added to NBF and EXP claim
	// checks to account for clock skew as per https://tools.ietf.org/html/rfc7519#section-4.1.5
	Leeway = 60 * time.Second
)

var signingAlgorithms = map[string]jose.SignatureAlgorithm{
	"EdDSA": jose.EdDSA,
	"HS256": jose.HS256,
	"HS384": jose.HS384,
	"HS512": jose.HS512,
	"RS256": jose.RS256,
	"RS384": jose.RS384,
	"RS512": jose.RS512,
	"ES256": jose.ES256,
	"ES384": jose.ES384,
	"ES512": jose.ES512,
	"PS256": jose.PS256,
	"PS384": jose.PS384,
	"PS512": jose.PS512,
}

var defaultSigningAlgorithms = []jose.SignatureAlgorithm{
	jose.EdDSA,
	jose.HS256,
	jose.HS384,
	jose.HS512,
	jose.RS256,
	jose.RS384,
	jose.RS512,
	jose.ES256,
	jose.ES384,
	jose.ES512,
	jose.PS256,
	jose.PS384,
	jose.PS512,
}

// Errors used by token parsing and verification.
var (
	ErrMalformedToken = errors.New("malformed token")
	ErrInvalidToken   = errors.New("invalid token")
)

// ResourceActions stores allowed actions on a named and typed resource.
type ResourceActions struct {
	Type    string   `json:"type"`
	Class   string   `json:"class,omitempty"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// ClaimSet describes the main section of a JSON Web Token.
type ClaimSet struct {
	// Public claims
	Issuer     string       `json:"iss"`
	Subject    string       `json:"sub"`
	Audience   AudienceList `json:"aud"`
	Expiration int64        `json:"exp"`
	NotBefore  int64        `json:"nbf"`
	IssuedAt   int64        `json:"iat"`
	JWTID      string       `json:"jti"`

	// Private claims
	Access []*ResourceActions `json:"access"`
}

// Token is a JSON Web Token.
type Token struct {
	Raw string
	JWT *jwt.JSONWebToken
}

// VerifyOptions is used to specify
// options when verifying a JSON Web Token.
type VerifyOptions struct {
	TrustedIssuers    []string
	AcceptedAudiences []string
	Roots             *x509.CertPool
	TrustedKeys       map[string]crypto.PublicKey
}

// NewToken parses the given raw token string
// and constructs an unverified JSON Web Token.
func NewToken(rawToken string, signingAlgs []jose.SignatureAlgorithm) (*Token, error) {
	token, err := jwt.ParseSigned(rawToken, signingAlgs)
	if err != nil {
		return nil, ErrMalformedToken
	}

	return &Token{
		Raw: rawToken,
		JWT: token,
	}, nil
}

// Verify attempts to verify this token using the given options.
// Returns a nil error if the token is valid.
func (t *Token) Verify(verifyOpts VerifyOptions) (*ClaimSet, error) {
	// Verify that the signing key is trusted.
	signingKey, err := t.VerifySigningKey(verifyOpts)
	if err != nil {
		log.Infof("failed to verify token: %v", err)
		return nil, ErrInvalidToken
	}

	// NOTE(milosgajdos): Claims both verifies the signature
	// and returns the claims within the payload
	var claims ClaimSet
	err = t.JWT.Claims(signingKey, &claims)
	if err != nil {
		return nil, err
	}

	// Verify that the Issuer claim is a trusted authority.
	if !contains(verifyOpts.TrustedIssuers, claims.Issuer) {
		log.Infof("token from untrusted issuer: %q", claims.Issuer)
		return nil, ErrInvalidToken
	}

	// Verify that the Audience claim is allowed.
	if !containsAny(verifyOpts.AcceptedAudiences, claims.Audience) {
		log.Infof("token intended for another audience: %v", claims.Audience)
		return nil, ErrInvalidToken
	}

	// Verify that the token is currently usable and not expired.
	currentTime := time.Now()

	ExpWithLeeway := time.Unix(claims.Expiration, 0).Add(Leeway)
	if currentTime.After(ExpWithLeeway) {
		log.Infof("token not to be used after %s - currently %s", ExpWithLeeway, currentTime)
		return nil, ErrInvalidToken
	}

	NotBeforeWithLeeway := time.Unix(claims.NotBefore, 0).Add(-Leeway)
	if currentTime.Before(NotBeforeWithLeeway) {
		log.Infof("token not to be used before %s - currently %s", NotBeforeWithLeeway, currentTime)
		return nil, ErrInvalidToken
	}

	return &claims, nil
}

// VerifySigningKey attempts to verify and return the signing key which was used to sign the token.
func (t *Token) VerifySigningKey(verifyOpts VerifyOptions) (crypto.PublicKey, error) {
	if len(t.JWT.Headers) == 0 {
		return nil, ErrInvalidToken
	}

	// NOTE(milosgajdos): docker auth spec does not seem to
	// support tokens signed by multiple signatures so we are
	// verifying the first one in the list only at the moment.
	header := t.JWT.Headers[0]

	signingKey, err := verifyCertChain(header, verifyOpts.Roots)
	if err != nil {
		// NOTE(milosgajdos): if the x5c header is missing
		// the token may have been signed by a JWKS.
		if errors.Is(err, jose.ErrMissingX5cHeader) {
			switch {
			case header.JSONWebKey != nil:
				return verifyJWK(header, verifyOpts)
			case header.KeyID != "":
				if signingKey, ok := verifyOpts.TrustedKeys[header.KeyID]; ok {
					return signingKey, nil
				}
				return nil, fmt.Errorf("token signed by untrusted key with ID: %q", header.KeyID)
			default:
				return nil, ErrInvalidToken
			}
		}
		return nil, err
	}

	return signingKey, nil
}

func verifyCertChain(header jose.Header, roots *x509.CertPool) (signingKey crypto.PublicKey, err error) {
	verifyOpts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	// TODO: this call returns certificate chains which we ignore for now, but
	// we should check them for revocations if we have the ability later.
	chains, err := header.Certificates(verifyOpts)
	if err != nil {
		return nil, err
	}
	signingKey = getCertPubKey(chains)

	return
}

func verifyJWK(header jose.Header, verifyOpts VerifyOptions) (crypto.PublicKey, error) {
	jwk := header.JSONWebKey

	// Check to see if the key includes a certificate chain.
	if len(jwk.Certificates) == 0 {
		// The JWK should be one of the trusted root keys.
		key, trusted := verifyOpts.TrustedKeys[jwk.KeyID]
		if !trusted {
			return nil, errors.New("untrusted JWK with no certificate chain")
		}
		// The JWK is one of the trusted keys.
		return key, nil
	}

	opts := x509.VerifyOptions{
		Roots:     verifyOpts.Roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	leaf := jwk.Certificates[0]
	if opts.Intermediates == nil {
		opts.Intermediates = x509.NewCertPool()
		for _, intermediate := range jwk.Certificates[1:] {
			opts.Intermediates.AddCert(intermediate)
		}
	}

	// TODO: this call returns certificate chains which we ignore for now, but
	// we should check them for revocations if we have the ability later.
	chains, err := leaf.Verify(opts)
	if err != nil {
		return nil, err
	}

	return getCertPubKey(chains), nil
}

func getCertPubKey(chains [][]*x509.Certificate) crypto.PublicKey {
	// NOTE(milosgajdos): if there are no certificates
	// header.Certificates call above returns error, so we are
	// guaranteed to get at least one certificate chain.
	// We pick the leaf certificate chain.
	chain := chains[0]

	// NOTE(milosgajdos): header.Certificates call returns the result
	// of leafCert.Verify which is a call to x509.Certificate.Verify.
	// If successful, it returns one or more chains where the first
	// element of the chain is x5c and the last element is from opts.Roots.
	// See: https://pkg.go.dev/crypto/x509#Certificate.Verify
	cert := chain[0]

	// NOTE: we dont have to verify that the public key in the leaf cert
	// *is* the signing key: if it's not the signing then token claims
	// verification with this key fails
	return cert.PublicKey.(crypto.PublicKey)
}

// accessSet returns a set of actions available for the resource
// actions listed in the `access` section of this token.
func (c *ClaimSet) accessSet() accessSet {
	accessSet := make(accessSet, len(c.Access))

	for _, resourceActions := range c.Access {
		resource := auth.Resource{
			Type: resourceActions.Type,
			Name: resourceActions.Name,
		}

		set, exists := accessSet[resource]
		if !exists {
			set = newActionSet()
			accessSet[resource] = set
		}

		for _, action := range resourceActions.Actions {
			set.add(action)
		}
	}

	return accessSet
}

func (c *ClaimSet) resources() []auth.Resource {
	resourceSet := map[auth.Resource]struct{}{}

	for _, resourceActions := range c.Access {
		resource := auth.Resource{
			Type:  resourceActions.Type,
			Class: resourceActions.Class,
			Name:  resourceActions.Name,
		}
		resourceSet[resource] = struct{}{}
	}

	resources := make([]auth.Resource, 0, len(resourceSet))
	for resource := range resourceSet {
		resources = append(resources, resource)
	}

	return resources
}

package token

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/libtrust"

	"github.com/docker/docker-registry/auth"
)

const (
	// TokenSeparator is the value which separates the header, claims, and
	// signature in the compact serialization of a JSON Web Token.
	TokenSeparator = "."
)

// Errors used by token parsing and verification.
var (
	ErrMalformedToken = errors.New("malformed token")
	ErrInvalidToken   = errors.New("invalid token")
)

// ResourceActions stores allowed actions on a named and typed resource.
type ResourceActions struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	Actions []string `json:"actions"`
}

// ClaimSet describes the main section of a JSON Web Token.
type ClaimSet struct {
	// Public claims
	Issuer     string `json:"iss"`
	Subject    string `json:"sub"`
	Audience   string `json:"aud"`
	Expiration int64  `json:"exp"`
	NotBefore  int64  `json:"nbf"`
	IssuedAt   int64  `json:"iat"`
	JWTID      string `json:"jti"`

	// Private claims
	Access []*ResourceActions
}

// Header describes the header section of a JSON Web Token.
type Header struct {
	Type       string             `json:"typ"`
	SigningAlg string             `json:"alg"`
	KeyID      string             `json:"kid,omitempty"`
	RawJWK     json.RawMessage    `json:"jwk"`
	SigningKey libtrust.PublicKey `json:"-"`
}

// CheckSigningKey parses the `jwk` field of a JOSE header and sets the
// SigningKey field if it is valid.
func (h *Header) CheckSigningKey() (err error) {
	if len(h.RawJWK) == 0 {
		// No signing key was specified.
		return
	}

	h.SigningKey, err = libtrust.UnmarshalPublicKeyJWK([]byte(h.RawJWK))
	h.RawJWK = nil // Don't need this anymore!

	return
}

// Token describes a JSON Web Token.
type Token struct {
	Raw       string
	Header    *Header
	Claims    *ClaimSet
	Signature []byte
	Valid     bool
}

// VerifyOptions is used to specify
// options when verifying a JSON Web Token.
type VerifyOptions struct {
	TrustedIssuers    stringSet
	AccpetedAudiences stringSet
	Roots             *x509.CertPool
	TrustedKeys       map[string]libtrust.PublicKey
}

// NewToken parses the given raw token string
// and constructs an unverified JSON Web Token.
func NewToken(rawToken string) (*Token, error) {
	parts := strings.Split(rawToken, TokenSeparator)
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}

	var (
		rawHeader, rawClaims   = parts[0], parts[1]
		headerJSON, claimsJSON []byte
		err                    error
	)

	defer func() {
		if err != nil {
			log.Errorf("error while unmarshalling raw token: %s", err)
		}
	}()

	if headerJSON, err = joseBase64UrlDecode(rawHeader); err != nil {
		err = fmt.Errorf("unable to decode header: %s", err)
		return nil, ErrMalformedToken
	}

	if claimsJSON, err = joseBase64UrlDecode(rawClaims); err != nil {
		err = fmt.Errorf("unable to decode claims: %s", err)
		return nil, ErrMalformedToken
	}

	token := new(Token)
	token.Header = new(Header)
	token.Claims = new(ClaimSet)

	token.Raw = strings.Join(parts[:2], TokenSeparator)
	if token.Signature, err = joseBase64UrlDecode(parts[2]); err != nil {
		err = fmt.Errorf("unable to decode signature: %s", err)
		return nil, ErrMalformedToken
	}

	if err = json.Unmarshal(headerJSON, token.Header); err != nil {
		return nil, ErrMalformedToken
	}

	if err = token.Header.CheckSigningKey(); err != nil {
		return nil, ErrMalformedToken
	}

	if err = json.Unmarshal(claimsJSON, token.Claims); err != nil {
		return nil, ErrMalformedToken
	}

	return token, nil
}

// Verify attempts to verify this token using the given options.
// Returns a nil error if the token is valid.
func (t *Token) Verify(verifyOpts VerifyOptions) error {
	if t.Valid {
		// Token was already verified.
		return nil
	}

	// Verify that the Issuer claim is a trusted authority.
	if !verifyOpts.TrustedIssuers.contains(t.Claims.Issuer) {
		log.Errorf("token from untrusted issuer: %q", t.Claims.Issuer)
		return ErrInvalidToken
	}

	// Verify that the Audience claim is allowed.
	if !verifyOpts.AccpetedAudiences.contains(t.Claims.Audience) {
		log.Errorf("token intended for another audience: %q", t.Claims.Audience)
		return ErrInvalidToken
	}

	// Verify that the token is currently usable and not expired.
	currentUnixTime := time.Now().Unix()
	if !(t.Claims.NotBefore <= currentUnixTime && currentUnixTime <= t.Claims.Expiration) {
		log.Errorf("token not to be used before %d or after %d - currently %d", t.Claims.NotBefore, t.Claims.Expiration, currentUnixTime)
		return ErrInvalidToken
	}

	// Verify the token signature.
	if len(t.Signature) == 0 {
		log.Error("token has no signature")
		return ErrInvalidToken
	}

	// If the token header has a SigningKey field, verify the signature
	// using that key and its included x509 certificate chain if necessary.
	// If the Header's SigningKey field is nil, try using the KeyID field.
	signingKey := t.Header.SigningKey

	if signingKey == nil {
		// Find the key in the given collection of trusted keys.
		trustedKey, ok := verifyOpts.TrustedKeys[t.Header.KeyID]
		if !ok {
			log.Errorf("token signed by untrusted key with ID: %q", t.Header.KeyID)
			return ErrInvalidToken
		}
		signingKey = trustedKey
	}

	// First verify the signature of the token using the key which signed it.
	if err := signingKey.Verify(strings.NewReader(t.Raw), t.Header.SigningAlg, t.Signature); err != nil {
		log.Errorf("unable to verify token signature: %s", err)
		return ErrInvalidToken
	}

	// Next, check if the signing key is one of the trusted keys.
	if _, isTrustedKey := verifyOpts.TrustedKeys[signingKey.KeyID()]; isTrustedKey {
		// We're done! The token was signed by a trusted key and has been verified!
		t.Valid = true
		return nil
	}

	// Otherwise, we need to check the sigining keys included certificate chain.
	return t.verifyCertificateChain(signingKey, verifyOpts.Roots)
}

// verifyCertificateChain attempts to verify the token using the "x5c" field
// of the given leafKey which was used to sign it. Returns a nil error if
// the key's certificate chain is valid and rooted an one of the given roots.
func (t *Token) verifyCertificateChain(leafKey libtrust.PublicKey, roots *x509.CertPool) error {
	// In this case, the token signature is valid, but the key that signed it
	// is not in our set of trusted keys. So, we'll need to check if the
	// token's signing key included an x509 certificate chain that can be
	// verified up to one of our trusted roots.
	x5cVal, ok := leafKey.GetExtendedField("x5c").([]interface{})
	if !ok || x5cVal == nil {
		log.Error("unable to verify token signature: signed by untrusted key with no valid certificate chain")
		return ErrInvalidToken
	}

	// Ensure each item is of the correct type.
	x5c := make([]string, len(x5cVal))
	for i, val := range x5cVal {
		certString, ok := val.(string)
		if !ok || len(certString) == 0 {
			log.Error("unable to verify token signature: signed by untrusted key with malformed certificate chain")
			return ErrInvalidToken
		}
		x5c[i] = certString
	}

	// Ensure the first element is encoded correctly.
	leafCertDer, err := base64.StdEncoding.DecodeString(x5c[0])
	if err != nil {
		log.Errorf("unable to decode signing key leaf cert: %s", err)
		return ErrInvalidToken
	}

	// And that it is a valid x509 certificate.
	leafCert, err := x509.ParseCertificate(leafCertDer)
	if err != nil {
		log.Errorf("unable to parse signing key leaf cert: %s", err)
		return ErrInvalidToken
	}

	// Verify that the public key in the leaf cert *is* the signing key.
	leafCryptoKey, ok := leafCert.PublicKey.(crypto.PublicKey)
	if !ok {
		log.Error("unable to get signing key leaf cert public key value")
		return ErrInvalidToken
	}

	leafPubKey, err := libtrust.FromCryptoPublicKey(leafCryptoKey)
	if err != nil {
		log.Errorf("unable to make libtrust public key from signing key leaf cert: %s", err)
		return ErrInvalidToken
	}

	if leafPubKey.KeyID() != leafKey.KeyID() {
		log.Error("token signing key ID and leaf certificate public key ID do not match")
		return ErrInvalidToken
	}

	// The rest of the x5c array are intermediate certificates.
	intermediates := x509.NewCertPool()
	for i := 1; i < len(x5c); i++ {
		intermediateCertDer, err := base64.StdEncoding.DecodeString(x5c[i])
		if err != nil {
			log.Errorf("unable to decode signing key intermediate cert: %s", err)
			return ErrInvalidToken
		}

		intermediateCert, err := x509.ParseCertificate(intermediateCertDer)
		if err != nil {
			log.Errorf("unable to parse signing key intermediate cert: %s", err)
			return ErrInvalidToken
		}

		intermediates.AddCert(intermediateCert)
	}

	verifyOpts := x509.VerifyOptions{
		Intermediates: intermediates,
		Roots:         roots,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	// TODO: this call returns certificate chains which we ignore for now, but
	// we should check them for revocations if we have the ability later.
	if _, err = leafCert.Verify(verifyOpts); err != nil {
		log.Errorf("unable to verify signing key certificate: %s", err)
		return ErrInvalidToken
	}

	// The signing key's x509 chain is valid!
	t.Valid = true
	return nil
}

// accessSet returns a set of actions available for the resource
// actions listed in the `access` section of this token.
func (t *Token) accessSet() accessSet {
	if t.Claims == nil {
		return nil
	}

	accessSet := make(accessSet, len(t.Claims.Access))

	for _, resourceActions := range t.Claims.Access {
		resource := auth.Resource{
			Type: resourceActions.Type,
			Name: resourceActions.Name,
		}

		set := accessSet[resource]
		if set == nil {
			set = make(actionSet)
			accessSet[resource] = set
		}

		for _, action := range resourceActions.Actions {
			set[action] = struct{}{}
		}
	}

	return accessSet
}

func (t *Token) compactRaw() string {
	return fmt.Sprintf("%s.%s", t.Raw, joseBase64UrlEncode(t.Signature))
}

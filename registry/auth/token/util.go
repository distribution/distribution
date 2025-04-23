package token

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
)

// actionSet is a special type of stringSet.
type actionSet struct {
	stringSet
}

func newActionSet(actions ...string) actionSet {
	return actionSet{newStringSet(actions...)}
}

// Contains calls StringSet.Contains() for
// either "*" or the given action string.
func (s actionSet) contains(action string) bool {
	return s.stringSet.contains("*") || s.stringSet.contains(action)
}

// contains returns true if q is found in ss.
func contains(ss []string, q string) bool {
	for _, s := range ss {
		if s == q {
			return true
		}
	}

	return false
}

// containsAny returns true if any of q is found in ss.
func containsAny(ss []string, q []string) bool {
	for _, s := range ss {
		if contains(q, s) {
			return true
		}
	}

	return false
}

// NOTE: RFC7638 does not prescribe which hashing function to use, but suggests
// sha256 as a sane default as of time of writing
func hashAndEncode(payload string) string {
	shasum := sha256.Sum256([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(shasum[:])
}

// Deprecated: use GetJWKThumbprint instead.
func GetRFC7638Thumbprint(publickey crypto.PublicKey) string {
	return getJWKThumbprint(publickey, true)
}

// GetJWKThumbprint calculates the JWK thumbprint of a public key.
// The current implementation uses SHA256, but this algorithm may change
// in the future as cryptographic best practices evolve. It returns an
// empty string if the public key type is not supported.
func GetJWKThumbprint(publickey crypto.PublicKey) string {
	return getJWKThumbprint(publickey, false)
}

// RFC7638 states in section 3 sub 1 that the keys in the JSON object payload
// are required to be ordered lexicographical order. Golang does not guarantee
// order of keys[0]
// [0]: https://groups.google.com/g/golang-dev/c/zBQwhm3VfvU
//
// The payloads are small enough to create the JSON strings manually
func getJWKThumbprint(publickey crypto.PublicKey, skipED25519 bool) string {
	switch pubkey := publickey.(type) {
	case *rsa.PublicKey:
		e_big := big.NewInt(int64(pubkey.E)).Bytes()

		e := base64.RawURLEncoding.EncodeToString(e_big)
		n := base64.RawURLEncoding.EncodeToString(pubkey.N.Bytes())

		return hashAndEncode(fmt.Sprintf(`{"e":"%s","kty":"RSA","n":"%s"}`, e, n))
	case *ecdsa.PublicKey:
		params := pubkey.Params()
		crv := params.Name
		x := base64.RawURLEncoding.EncodeToString(params.Gx.Bytes())
		y := base64.RawURLEncoding.EncodeToString(params.Gy.Bytes())

		return hashAndEncode(fmt.Sprintf(`{"crv":"%s","kty":"EC","x":"%s","y":"%s"}`, crv, x, y))
	case ed25519.PublicKey:
		if skipED25519 {
			return ""
		}
		x := base64.RawURLEncoding.EncodeToString(pubkey)

		return hashAndEncode(fmt.Sprintf(`{"crv":"Ed25519","kty":"OTP","x":"%s"}`, x))
	default:
		return ""
	}
}

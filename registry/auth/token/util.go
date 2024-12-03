package token

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
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

// RFC7638 states in section 3 sub 1 that the keys in the JSON object payload
// are required to be ordered lexicographical order. Golang does not guarantee
// order of keys[0]
// [0]: https://groups.google.com/g/golang-dev/c/zBQwhm3VfvU
//
// The payloads are small enough to create the JSON strings manually
func GetRFC7638Thumbprint(publickey crypto.PublicKey) string {
	var payload string

	switch pubkey := publickey.(type) {
	case *rsa.PublicKey:
		e_big := big.NewInt(int64(pubkey.E)).Bytes()

		e := base64.RawURLEncoding.EncodeToString(e_big)
		n := base64.RawURLEncoding.EncodeToString(pubkey.N.Bytes())

		payload = fmt.Sprintf(`{"e":"%s","kty":"RSA","n":"%s"}`, e, n)
	case *ecdsa.PublicKey:
		params := pubkey.Params()
		crv := params.Name
		x := base64.RawURLEncoding.EncodeToString(params.Gx.Bytes())
		y := base64.RawURLEncoding.EncodeToString(params.Gy.Bytes())

		payload = fmt.Sprintf(`{"crv":"%s","kty":"EC","x":"%s","y":"%s"}`, crv, x, y)
	default:
		return ""
	}

	return hashAndEncode(payload)
}

// Returns a libtrust-compatible Key ID, for backwards compatibility
// with JWT headers expected by distribution/v2
func GetLibtrustKeyID(publickey crypto.PublicKey) string {
	keyBytes, err := x509.MarshalPKIXPublicKey(publickey)
	if err != nil {
		return ""
	}

	sum := sha256.Sum256(keyBytes)
	b64 := strings.TrimRight(base32.StdEncoding.EncodeToString(sum[:30]), "=")

	var buf bytes.Buffer
	var i int
	for i = 0; i < len(b64)/4-1; i++ {
		start := i * 4
		end := start + 4
		buf.WriteString(b64[start:end] + ":")
	}
	buf.WriteString(b64[i*4:])

	return buf.String()
}

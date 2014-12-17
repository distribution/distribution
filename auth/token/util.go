package token

import (
	"encoding/base64"
	"errors"
	"strings"
)

// joseBase64UrlEncode encodes the given data using the standard base64 url
// encoding format but with all trailing '=' characters ommitted in accordance
// with the jose specification.
// http://tools.ietf.org/html/draft-ietf-jose-json-web-signature-31#section-2
func joseBase64UrlEncode(b []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
}

// joseBase64UrlDecode decodes the given string using the standard base64 url
// decoder but first adds the appropriate number of trailing '=' characters in
// accordance with the jose specification.
// http://tools.ietf.org/html/draft-ietf-jose-json-web-signature-31#section-2
func joseBase64UrlDecode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 0:
	case 2:
		s += "=="
	case 3:
		s += "="
	default:
		return nil, errors.New("illegal base64url string")
	}
	return base64.URLEncoding.DecodeString(s)
}

// stringSet is a useful type for looking up strings.
type stringSet map[string]struct{}

func newStringSet(strs ...string) stringSet {
	set := make(stringSet, len(strs))
	for _, str := range strs {
		set[str] = struct{}{}
	}

	return set
}

// contains returns whether the given key is in this StringSet.
func (ss stringSet) contains(key string) bool {
	_, ok := ss[key]
	return ok
}

// keys returns a slice of all keys in this stringSet.
func (ss stringSet) keys() []string {
	keys := make([]string, 0, len(ss))

	for key := range ss {
		keys = append(keys, key)
	}

	return keys
}

// actionSet is a special type of stringSet.
type actionSet stringSet

// contains calls stringSet.contains() for
// either "*" or the given action string.
func (s actionSet) contains(action string) bool {
	ss := stringSet(s)

	return ss.contains("*") || ss.contains(action)
}

// keys wraps stringSet.keys()
func (s actionSet) keys() []string {
	return stringSet(s).keys()
}

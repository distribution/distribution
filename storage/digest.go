package storage

import (
	"fmt"
	"hash"
	"strings"
)

// Digest allows simple protection of hex formatted digest strings, prefixed
// by their algorithm. Strings of type Digest have some guarantee of being in
// the correct format and it provides quick access to the components of a
// digest string.
//
// The following is an example of the contents of Digest types:
//
// 	sha256:7173b809ca12ec5dee4506cd86be934c4596dd234ee82c0662eac04a8c2c71dc
//
type Digest string

// NewDigest returns a Digest from alg and a hash.Hash object.
func NewDigest(alg string, h hash.Hash) Digest {
	return Digest(fmt.Sprintf("%s:%x", alg, h.Sum(nil)))
}

var (
	// ErrDigestInvalidFormat returned when digest format invalid.
	ErrDigestInvalidFormat = fmt.Errorf("invalid checksum digest format")

	// ErrDigestUnsupported returned when the digest algorithm is unsupported by registry.
	ErrDigestUnsupported = fmt.Errorf("unsupported digest algorithm")
)

// ParseDigest parses s and returns the validated digest object. An error will
// be returned if the format is invalid.
func ParseDigest(s string) (Digest, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return "", ErrDigestInvalidFormat
	}

	switch parts[0] {
	case "sha256":
		break
	default:
		return "", ErrDigestUnsupported
	}

	return Digest(s), nil
}

// Algorithm returns the algorithm portion of the digest.
func (d Digest) Algorithm() string {
	return strings.SplitN(string(d), ":", 2)[0]
}

// Hex returns the hex digest portion of the digest.
func (d Digest) Hex() string {
	return strings.SplitN(string(d), ":", 2)[1]
}

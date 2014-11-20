package digest

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"strings"

	"github.com/docker/docker-registry/common"
	"github.com/docker/docker/pkg/tarsum"
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
// More important for this code base, this type is compatible with tarsum
// digests. For example, the following would be a valid Digest:
//
// 	tarsum+sha256:e58fcf7418d4390dec8e8fb69d88c06ec07039d651fedd3aa72af9972e7d046b
//
// This allows to abstract the digest behind this type and work only in those
// terms.
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
	// Common case will be tarsum
	_, err := common.ParseTarSum(s)
	if err == nil {
		return Digest(s), nil
	}

	// Continue on for general parser

	i := strings.Index(s, ":")
	if i < 0 {
		return "", ErrDigestInvalidFormat
	}

	// case: "sha256:" with no hex.
	if i+1 == len(s) {
		return "", ErrDigestInvalidFormat
	}

	switch s[:i] {
	case "md5", "sha1", "sha256":
		break
	default:
		return "", ErrDigestUnsupported
	}

	return Digest(s), nil
}

// FromReader returns the most valid digest for the underlying content.
func FromReader(rd io.Reader) (Digest, error) {

	// TODO(stevvooe): This is pretty inefficient to always be calculating a
	// sha256 hash to provide fallback, but it provides some nice semantics in
	// that we never worry about getting the right digest for a given reader.
	// For the most part, we can detect tar vs non-tar with only a few bytes,
	// so a scheme that saves those bytes would probably be better here.

	h := sha256.New()
	tr := io.TeeReader(rd, h)

	ts, err := tarsum.NewTarSum(tr, true, tarsum.Version1)
	if err != nil {
		return "", err
	}

	// Try to copy from the tarsum, if we fail, copy the remaining bytes into
	// hash directly.
	if _, err := io.Copy(ioutil.Discard, ts); err != nil {
		if err.Error() != "archive/tar: invalid tar header" {
			return "", err
		}

		if _, err := io.Copy(h, rd); err != nil {
			return "", err
		}

		return NewDigest("sha256", h), nil
	}

	d, err := ParseDigest(ts.Sum(nil))
	if err != nil {
		return "", err
	}

	return d, nil
}

// FromBytes digests the input and returns a Digest.
func FromBytes(p []byte) (Digest, error) {
	return FromReader(bytes.NewReader(p))
}

// Algorithm returns the algorithm portion of the digest. This will panic if
// the underlying digest is not in a valid format.
func (d Digest) Algorithm() string {
	return string(d[:d.sepIndex()])
}

// Hex returns the hex digest portion of the digest. This will panic if the
// underlying digest is not in a valid format.
func (d Digest) Hex() string {
	return string(d[d.sepIndex()+1:])
}

func (d Digest) String() string {
	return string(d)
}

func (d Digest) sepIndex() int {
	i := strings.Index(string(d), ":")

	if i < 0 {
		panic("invalid digest: " + d)
	}

	return i
}

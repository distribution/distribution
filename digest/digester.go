package digest

import (
	"crypto/sha256"
	"fmt"
	"hash"

	"github.com/jlhawn/go-crypto"          // For ResumableHash
	_ "github.com/jlhawn/go-crypto/sha256" // For Resumable SHA256
	_ "github.com/jlhawn/go-crypto/sha512" // For Resumable SHA384, SHA512
)

// Digester calculates the digest of written data. It is functionally
// equivalent to hash.Hash but provides methods for returning the Digest type
// rather than raw bytes.
type Digester struct {
	alg string
	hash.Hash
}

// NewDigester create a new Digester with the given hashing algorithm and instance
// of that algo's hasher.
func NewDigester(alg string, h hash.Hash) Digester {
	return Digester{
		alg:  alg,
		Hash: h,
	}
}

// NewCanonicalDigester is a convenience function to create a new Digester with
// our default settings.
func NewCanonicalDigester() Digester {
	return NewDigester("sha256", sha256.New())
}

// Digest returns the current digest for this digester.
func (d *Digester) Digest() Digest {
	return NewDigest(d.alg, d.Hash)
}

// ResumableDigester is a digester that can export its internal state and be
// restored from saved state.
type ResumableDigester struct {
	alg string
	crypto.ResumableHash
}

var resumableHashAlgs = map[string]crypto.Hash{
	"sha256": crypto.SHA256,
	"sha384": crypto.SHA384,
	"sha512": crypto.SHA512,
}

// NewResumableDigester creates a new ResumableDigester with the given hashing
// algorithm.
func NewResumableDigester(alg string) (ResumableDigester, error) {
	hash, supported := resumableHashAlgs[alg]
	if !supported {
		return ResumableDigester{}, fmt.Errorf("unsupported resumable hash algorithm: %s", alg)
	}

	return ResumableDigester{
		alg:           alg,
		ResumableHash: hash.New(),
	}, nil
}

// NewCanonicalResumableDigester creates a ResumableDigester using the default
// digest algorithm.
func NewCanonicalResumableDigester() ResumableDigester {
	return ResumableDigester{
		alg:           "sha256",
		ResumableHash: crypto.SHA256.New(),
	}
}

// Digest returns the current digest for this resumable digester.
func (d ResumableDigester) Digest() Digest {
	return NewDigest(d.alg, d.ResumableHash)
}

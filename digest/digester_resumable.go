// +build !noresumabledigest

package digest

import (
	"fmt"

	"github.com/jlhawn/go-crypto"
	// For ResumableHash
	_ "github.com/jlhawn/go-crypto/sha256" // For Resumable SHA256
	_ "github.com/jlhawn/go-crypto/sha512" // For Resumable SHA384, SHA512
)

// resumableDigester implements ResumableDigester.
type resumableDigester struct {
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
		return resumableDigester{}, fmt.Errorf("unsupported resumable hash algorithm: %s", alg)
	}

	return resumableDigester{
		alg:           alg,
		ResumableHash: hash.New(),
	}, nil
}

// NewCanonicalResumableDigester creates a ResumableDigester using the default
// digest algorithm.
func NewCanonicalResumableDigester() ResumableDigester {
	return resumableDigester{
		alg:           "sha256",
		ResumableHash: crypto.SHA256.New(),
	}
}

// Digest returns the current digest for this resumable digester.
func (d resumableDigester) Digest() Digest {
	return NewDigest(d.alg, d.ResumableHash)
}

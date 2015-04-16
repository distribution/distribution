package digest

import (
	"crypto/sha256"
	"hash"
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

// ResumableHash is the common interface implemented by all resumable hash
// functions.
type ResumableHash interface {
	// ResumableHash is a superset of hash.Hash
	hash.Hash
	// Len returns the number of bytes written to the Hash so far.
	Len() uint64
	// State returns a snapshot of the state of the Hash.
	State() ([]byte, error)
	// Restore resets the Hash to the given state.
	Restore(state []byte) error
}

// ResumableDigester is a digester that can export its internal state and be
// restored from saved state.
type ResumableDigester interface {
	ResumableHash
	Digest() Digest
}

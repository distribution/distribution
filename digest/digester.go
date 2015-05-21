package digest

import "hash"

// Digester calculates the digest of written data. Writes should go directly
// to the return value of Hash, while calling Digest will return the current
// value of the digest.
type Digester interface {
	Hash() hash.Hash // provides direct access to underlying hash instance.
	Digest() Digest
}

// NewDigester create a new Digester with the given hashing algorithm and
// instance of that algo's hasher.
func NewDigester(alg string, h hash.Hash) Digester {
	return &digester{
		alg:  alg,
		hash: h,
	}
}

// NewCanonicalDigester is a convenience function to create a new Digester with
// our default settings.
func NewCanonicalDigester() Digester {
	return NewDigester(CanonicalAlgorithm, CanonicalHash.New())
}

// digester provides a simple digester definition that embeds a hasher.
type digester struct {
	alg  string
	hash hash.Hash
}

func (d *digester) Hash() hash.Hash {
	return d.hash
}

func (d *digester) Digest() Digest {
	return NewDigest(d.alg, d.Hash())
}

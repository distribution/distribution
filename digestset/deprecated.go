package digestset

import (
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/go-digest/digestset"
)

// ErrDigestNotFound is used when a matching digest
// could not be found in a set.
//
// Deprecated: use [digestset.ErrDigestNotFound].
var ErrDigestNotFound = digestset.ErrDigestNotFound

// ErrDigestAmbiguous is used when multiple digests
// are found in a set. None of the matching digests
// should be considered valid matches.
//
// Deprecated: use [digestset.ErrDigestAmbiguous].
var ErrDigestAmbiguous = digestset.ErrDigestAmbiguous

// Set is used to hold a unique set of digests which
// may be easily referenced by a string
// representation of the digest as well as short representation.
// The uniqueness of the short representation is based on other
// digests in the set. If digests are omitted from this set,
// collisions in a larger set may not be detected, therefore it
// is important to always do short representation lookups on
// the complete set of digests. To mitigate collisions, an
// appropriately long short code should be used.
//
// Deprecated: use [digestset.Set].
type Set = digestset.Set

// NewSet creates an empty set of digests
// which may have digests added.
//
// Deprecated: use [digestset.NewSet].
func NewSet() *digestset.Set {
	return digestset.NewSet()
}

// ShortCodeTable returns a map of Digest to unique short codes. The
// length represents the minimum value, the maximum length may be the
// entire value of digest if uniqueness cannot be achieved without the
// full value. This function will attempt to make short codes as short
// as possible to be unique.
//
// Deprecated: use [digestset.ShortCodeTable].
func ShortCodeTable(dst *digestset.Set, length int) map[digest.Digest]string {
	return digestset.ShortCodeTable(dst, length)
}

/*
	This package has no purpose except to perform registration of multihashes.

	It is meant to be used as a side-effecting import, e.g.

		import (
			_ "github.com/multiformats/go-multihash/register/murmur3"
		)

	This package registers multihashes for murmur3
*/
package murmur3

import (
	"hash"

	multihash "github.com/multiformats/go-multihash/core"
	"github.com/spaolacci/murmur3"
)

func init() {
	multihash.Register(multihash.MURMUR3X64_64, func() hash.Hash { return murmur64{murmur3.New64()} })
}

// A wrapper is needed to export the correct size, because murmur3 incorrectly advertises Hash64 as a 128bit hash.
type murmur64 struct {
	hash.Hash64
}

func (murmur64) BlockSize() int {
	return 1
}

func (x murmur64) Size() int {
	return 8
}

func (x murmur64) Sum(digest []byte) []byte {
	return x.Hash64.Sum(digest)
}

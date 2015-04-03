// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package crypto is a Subset of the Go `crypto` Package with a Resumable Hash
package crypto

import (
	"hash"
	"strconv"
)

// Hash identifies a cryptographic hash function that is implemented in another
// package.
type Hash uint

// HashFunc simply returns the value of h so that Hash implements SignerOpts.
func (h Hash) HashFunc() Hash {
	return h
}

const (
	SHA224 Hash = 1 + iota // import crypto/sha256
	SHA256                 // import crypto/sha256
	SHA384                 // import crypto/sha512
	SHA512                 // import crypto/sha512
	maxHash
)

var digestSizes = []uint8{
	SHA224: 28,
	SHA256: 32,
	SHA384: 48,
	SHA512: 64,
}

// Size returns the length, in bytes, of a digest resulting from the given hash
// function. It doesn't require that the hash function in question be linked
// into the program.
func (h Hash) Size() int {
	if h > 0 && h < maxHash {
		return int(digestSizes[h])
	}
	panic("crypto: Size of unknown hash function")
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

var hashes = make([]func() ResumableHash, maxHash)

// New returns a new ResumableHash calculating the given hash function. New panics
// if the hash function is not linked into the binary.
func (h Hash) New() ResumableHash {
	if h > 0 && h < maxHash {
		f := hashes[h]
		if f != nil {
			return f()
		}
	}
	panic("crypto: requested hash function #" + strconv.Itoa(int(h)) + " is unavailable")
}

// Available reports whether the given hash function is linked into the binary.
func (h Hash) Available() bool {
	return h < maxHash && hashes[h] != nil
}

// RegisterHash registers a function that returns a new instance of the given
// hash function. This is intended to be called from the init function in
// packages that implement hash functions.
func RegisterHash(h Hash, f func() ResumableHash) {
	if h >= maxHash {
		panic("crypto: RegisterHash of unknown hash function")
	}
	hashes[h] = f
}

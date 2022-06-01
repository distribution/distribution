package multihash

import (
	"errors"
	"fmt"
	"hash"
	"io"

	mhreg "github.com/multiformats/go-multihash/core"
)

// ErrSumNotSupported is returned when the Sum function code is not implemented
var ErrSumNotSupported = mhreg.ErrSumNotSupported

var ErrLenTooLarge = errors.New("requested length was too large for digest")

// Sum obtains the cryptographic sum of a given buffer. The length parameter
// indicates the length of the resulting digest. Passing a negative value uses
// default length values for the selected hash function.
func Sum(data []byte, code uint64, length int) (Multihash, error) {
	// Get the algorithm.
	hasher, err := GetHasher(code)
	if err != nil {
		return nil, err
	}

	// Feed data in.
	hasher.Write(data)

	return encodeHash(hasher, code, length)
}

// SumStream obtains the cryptographic sum of a given stream. The length
// parameter indicates the length of the resulting digest. Passing a negative
// value uses default length values for the selected hash function.
func SumStream(r io.Reader, code uint64, length int) (Multihash, error) {
	// Get the algorithm.
	hasher, err := GetHasher(code)
	if err != nil {
		return nil, err
	}

	// Feed data in.
	if _, err = io.Copy(hasher, r); err != nil {
		return nil, err
	}

	return encodeHash(hasher, code, length)
}

func encodeHash(hasher hash.Hash, code uint64, length int) (Multihash, error) {
	// Compute final hash.
	//  A new slice is allocated.  FUTURE: see other comment below about allocation, and review together with this line to try to improve.
	sum := hasher.Sum(nil)

	// Deal with any truncation.
	//  Unless it's an identity multihash.  Those have different rules.
	if length < 0 {
		length = hasher.Size()
	}
	if len(sum) < length {
		return nil, ErrLenTooLarge
	}
	if length >= 0 {
		if code == IDENTITY {
			if length != len(sum) {
				return nil, fmt.Errorf("the length of the identity hash (%d) must be equal to the length of the data (%d)", length, len(sum))
			}
		}
		sum = sum[:length]
	}

	// Put the multihash metainfo bytes at the front of the buffer.
	//  FUTURE: try to improve allocations here.  Encode does several which are probably avoidable, but it's the shape of the Encode method arguments that forces this.
	return Encode(sum, code)
}

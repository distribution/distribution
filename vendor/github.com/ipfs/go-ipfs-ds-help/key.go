// Package dshelp provides utilities for parsing and creating
// datastore keys used by go-ipfs
package dshelp

import (
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/multiformats/go-base32"
	mh "github.com/multiformats/go-multihash"
)

// NewKeyFromBinary creates a new key from a byte slice.
func NewKeyFromBinary(rawKey []byte) datastore.Key {
	buf := make([]byte, 1+base32.RawStdEncoding.EncodedLen(len(rawKey)))
	buf[0] = '/'
	base32.RawStdEncoding.Encode(buf[1:], rawKey)
	return datastore.RawKey(string(buf))
}

// BinaryFromDsKey returns the byte slice corresponding to the given Key.
func BinaryFromDsKey(k datastore.Key) ([]byte, error) {
	return base32.RawStdEncoding.DecodeString(k.String()[1:])
}

// MultihashToDsKey creates a Key from the given Multihash.
// If working with Cids, you can call cid.Hash() to obtain
// the multihash. Note that different CIDs might represent
// the same multihash.
func MultihashToDsKey(k mh.Multihash) datastore.Key {
	return NewKeyFromBinary(k)
}

// DsKeyToMultihash converts a dsKey to the corresponding Multihash.
func DsKeyToMultihash(dsKey datastore.Key) (mh.Multihash, error) {
	kb, err := BinaryFromDsKey(dsKey)
	if err != nil {
		return nil, err
	}
	return mh.Cast(kb)
}

// DsKeyToCidV1Raw converts the given Key (which should be a raw multihash
// key) to a Cid V1 of the given type (see
// https://godoc.org/github.com/ipfs/go-cid#pkg-constants).
func DsKeyToCidV1(dsKey datastore.Key, codecType uint64) (cid.Cid, error) {
	hash, err := DsKeyToMultihash(dsKey)
	if err != nil {
		return cid.Cid{}, err
	}
	return cid.NewCidV1(codecType, hash), nil
}

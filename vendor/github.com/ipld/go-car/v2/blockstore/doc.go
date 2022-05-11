// Package blockstore implements the IPFS blockstore interface backed by a CAR file.
// This package provides two flavours of blockstore: ReadOnly and ReadWrite.
//
// The ReadOnly blockstore provides a read-only random access from a given data payload either in
// unindexed CARv1 format or indexed/unindexed v2 format:
// * ReadOnly.NewReadOnly can be used to instantiate a new read-only blockstore for a given CARv1
//   or CARv2 data payload with an optional index override.
// * ReadOnly.OpenReadOnly can be used to instantiate a new read-only blockstore for a given CARv1
//    or CARv2 file with automatic index generation if the index is not present.
//
// The ReadWrite blockstore allows writing and reading of the blocks concurrently. The user of this
// blockstore is responsible for calling ReadWrite.Finalize when finished writing blocks.
// Upon finalization, the instance can no longer be used for reading or writing blocks and will
// error if used. To continue reading the blocks users are encouraged to use ReadOnly blockstore
// instantiated from the same file path using OpenReadOnly.
// A user may resume reading/writing from files produced by an instance of ReadWrite blockstore. The
// resumption is attempted automatically, if the path passed to OpenReadWrite exists.
//
// Note that the blockstore implementations in this package behave similarly to IPFS IdStore wrapper
// when given CIDs with multihash.IDENTITY code.
// More specifically, for CIDs with multhash.IDENTITY code:
// * blockstore.Has will always return true.
// * blockstore.Get will always succeed, returning the multihash digest of the given CID.
// * blockstore.GetSize will always succeed, returning the multihash digest length of the given CID.
// * blockstore.Put and blockstore.PutMany will always succeed without performing any operation unless car.StoreIdentityCIDs is enabled.
//
// See: https://pkg.go.dev/github.com/ipfs/go-ipfs-blockstore#NewIdStore
package blockstore

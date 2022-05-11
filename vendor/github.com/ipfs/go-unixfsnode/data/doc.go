/*
Package data provides tools for working with the UnixFS data structure that
is encoded in the "Data" field of the larger a DagPB encoded IPLD node.

See https://github.com/ipfs/specs/blob/master/UNIXFS.md for more information
about this data structure.

This package provides an IPLD Prime compatible node interface for this data
structure, as well as methods for serializing and deserializing the data
structure to protobuf
*/
package data

//go:generate go run ./gen

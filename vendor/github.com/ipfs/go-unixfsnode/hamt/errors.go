package hamt

import "fmt"

type errorType string

func (e errorType) Error() string {
	return string(e)
}

const (
	// ErrNotProtobuf indicates an error attempting to load a HAMT from a non-protobuf node
	ErrNotProtobuf errorType = "node was not a protobuf node"
	// ErrNotUnixFSNode indicates an error attempting to load a HAMT from a generic protobuf node
	ErrNotUnixFSNode errorType = "node was not a UnixFS node"
	// ErrInvalidChildIndex indicates there is no link to load for the given child index
	ErrInvalidChildIndex errorType = "invalid index passed to operate children (likely corrupt bitfield)"
	// ErrHAMTTooDeep indicates we attempted to load from a HAMT node that went past the depth of the tree
	ErrHAMTTooDeep errorType = "sharded directory too deep"
	// ErrInvalidHashType indicates the HAMT node's hash function is unsupported (must be Murmur3)
	ErrInvalidHashType errorType = "only murmur3 supported as hash function"
	// ErrNoDataField indicates the HAMT node's UnixFS structure lacked a data field, which is
	// where a bit mask is stored
	ErrNoDataField errorType = "'Data' field not present"
	// ErrNoFanoutField indicates the HAMT node's UnixFS structure lacked a fanout field, which is required
	ErrNoFanoutField errorType = "'Fanout' field not present"
	// ErrHAMTSizeInvalid indicates the HAMT's size property was not an exact power of 2
	ErrHAMTSizeInvalid errorType = "hamt size should be a power of two"
	// ErrMissingLinkName indicates a link in a HAMT had no Name property (required for all HAMTs)
	ErrMissingLinkName errorType = "missing link name"
)

// ErrInvalidLinkName indicates a link's name was too short for a HAMT
type ErrInvalidLinkName struct {
	Name string
}

func (e ErrInvalidLinkName) Error() string {
	return fmt.Sprintf("invalid link name '%s'", e.Name)
}

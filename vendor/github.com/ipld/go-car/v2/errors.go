package car

import (
	"fmt"
)

var _ (error) = (*ErrCidTooLarge)(nil)

// ErrCidTooLarge signals that a CID is too large to include in CARv2 index.
// See: MaxIndexCidSize.
type ErrCidTooLarge struct {
	MaxSize     uint64
	CurrentSize uint64
}

func (e *ErrCidTooLarge) Error() string {
	return fmt.Sprintf("cid size is larger than max allowed (%d > %d)", e.CurrentSize, e.MaxSize)
}

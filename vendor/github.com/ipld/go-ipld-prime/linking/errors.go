package linking

import (
	"fmt"

	"github.com/ipld/go-ipld-prime/datamodel"
)

// ErrLinkingSetup is returned by methods on LinkSystem when some part of the system is not set up correctly,
// or when one of the components refuses to handle a Link or LinkPrototype given.
// (It is not yielded for errors from the storage nor codec systems once they've started; those errors rise without interference.)
type ErrLinkingSetup struct {
	Detail string // Perhaps an enum here as well, which states which internal function was to blame?
	Cause  error
}

func (e ErrLinkingSetup) Error() string { return fmt.Sprintf("%s: %v", e.Detail, e.Cause) }
func (e ErrLinkingSetup) Unwrap() error { return e.Cause }

// ErrHashMismatch is the error returned when loading data and verifying its hash
// and finding that the loaded data doesn't re-hash to the expected value.
// It is typically seen returned by functions like LinkSystem.Load or LinkSystem.Fill.
type ErrHashMismatch struct {
	Actual   datamodel.Link
	Expected datamodel.Link
}

func (e ErrHashMismatch) Error() string {
	return fmt.Sprintf("hash mismatch!  %v (actual) != %v (expected)", e.Actual, e.Expected)
}

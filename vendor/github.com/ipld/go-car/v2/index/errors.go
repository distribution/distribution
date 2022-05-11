package index

import "errors"

// ErrNotFound signals a record is not found in the index.
var ErrNotFound = errors.New("not found")

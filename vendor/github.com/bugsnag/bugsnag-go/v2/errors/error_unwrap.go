// +build go1.13

package errors

import (
	"github.com/pkg/errors"
)

// Unwrap returns the result of calling errors.Unwrap on the underlying error
func (err *Error) Unwrap() error {
	return errors.Unwrap(err.Err)
}

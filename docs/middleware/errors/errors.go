package errors

import (
	"fmt"
)

var ErrNotFound = fmt.Errorf("key not found")

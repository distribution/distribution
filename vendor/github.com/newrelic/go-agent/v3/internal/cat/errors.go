// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cat

import (
	"fmt"
)

type errUnexpectedArraySize struct {
	label    string
	expected int
	actual   int
}

func (e errUnexpectedArraySize) Error() string {
	return fmt.Sprintf("%s: expected %d; got %d", e.label, e.expected, e.actual)
}

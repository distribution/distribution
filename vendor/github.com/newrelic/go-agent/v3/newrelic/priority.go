// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"fmt"
	"strings"
)

// priority allows for a priority sampling of events.  When an event
// is created it is given a priority.  Whenever an event pool is
// full and events need to be dropped, the events with the lowest priority
// are dropped.
type priority float32

// According to spec, Agents SHOULD truncate the value to at most 6
// digits past the decimal point.
const (
	priorityFormat = "%.6f"
)

func newPriorityFromRandom(rnd func() float32) priority {
	for {
		if r := rnd(); 0.0 != r {
			return priority(r)
		}
	}
}

// newPriority returns a new priority.
func newPriority() priority {
	return newPriorityFromRandom(randFloat32)
}

// Float32 returns the priority as a float32.
func (p priority) Float32() float32 {
	return float32(p)
}

func (p priority) isLowerPriority(y priority) bool {
	return p < y
}

// MarshalJSON limits the number of decimals.
func (p priority) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(priorityFormat, p)), nil
}

// WriteJSON limits the number of decimals.
func (p priority) WriteJSON(buf *bytes.Buffer) {
	fmt.Fprintf(buf, priorityFormat, p)
}

func (p priority) traceStateFormat() string {
	s := fmt.Sprintf(priorityFormat, p)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

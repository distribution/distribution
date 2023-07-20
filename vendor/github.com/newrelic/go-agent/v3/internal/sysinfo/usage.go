// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package sysinfo

import (
	"time"
)

// Usage contains process process times.
type Usage struct {
	System time.Duration
	User   time.Duration
}

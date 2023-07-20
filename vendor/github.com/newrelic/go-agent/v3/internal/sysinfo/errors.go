// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package sysinfo

import (
	"errors"
)

var (
	// ErrFeatureUnsupported indicates unsupported platform.
	ErrFeatureUnsupported = errors.New("That feature is not supported on this platform")
)

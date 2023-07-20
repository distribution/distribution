// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// +build !linux

package sysinfo

import "os"

func getHostname() (string, error) {
	return os.Hostname()
}

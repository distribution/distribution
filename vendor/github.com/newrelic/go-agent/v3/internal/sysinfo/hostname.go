// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package sysinfo

// Hostname returns the host name.
func Hostname() (string, error) {
	return getHostname()
}

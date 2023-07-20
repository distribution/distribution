// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"strings"
	"sync"
)

var (
	trackMutex   sync.Mutex
	trackMetrics []string
)

// TrackUsage helps track which integration packages are used.
func TrackUsage(s ...string) {
	trackMutex.Lock()
	defer trackMutex.Unlock()

	m := "Supportability/" + strings.Join(s, "/")
	trackMetrics = append(trackMetrics, m)
}

// GetUsageSupportabilityMetrics returns supportability metric names.
func GetUsageSupportabilityMetrics() []string {
	trackMutex.Lock()
	defer trackMutex.Unlock()

	names := make([]string, 0, len(trackMetrics))
	for _, s := range trackMetrics {
		names = append(names, s)
	}
	return names
}

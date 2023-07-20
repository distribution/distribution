// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import "time"

// apdexZone is a transaction classification.
type apdexZone int

// https://en.wikipedia.org/wiki/Apdex
const (
	apdexNone apdexZone = iota
	apdexSatisfying
	apdexTolerating
	apdexFailing
)

// apdexFailingThreshold calculates the threshold at which the transaction is
// considered a failure.
func apdexFailingThreshold(threshold time.Duration) time.Duration {
	return 4 * threshold
}

// calculateApdexZone calculates the apdex based on the transaction duration and
// threshold.
//
// Note that this does not take into account whether or not the transaction
// had an error.  That is expected to be done by the caller.
func calculateApdexZone(threshold, duration time.Duration) apdexZone {
	if duration <= threshold {
		return apdexSatisfying
	}
	if duration <= apdexFailingThreshold(threshold) {
		return apdexTolerating
	}
	return apdexFailing
}

func (zone apdexZone) label() string {
	switch zone {
	case apdexSatisfying:
		return "S"
	case apdexTolerating:
		return "T"
	case apdexFailing:
		return "F"
	default:
		return ""
	}
}

// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"math"
	"sync"
	"time"
)

type adaptiveSampler struct {
	sync.Mutex
	period time.Duration
	target uint64

	// Transactions with priority higher than this are sampled.
	// This is 1 - sampleRatio.
	priorityMin float32

	currentPeriod struct {
		numSampled uint64
		numSeen    uint64
		end        time.Time
	}
}

// newAdaptiveSampler creates an adaptiveSampler.
func newAdaptiveSampler(period time.Duration, target uint64, now time.Time) *adaptiveSampler {
	as := &adaptiveSampler{}
	as.period = period
	as.target = target
	as.currentPeriod.end = now.Add(period)

	// Sample the first transactions in the first period.
	as.priorityMin = 0.0
	return as
}

// computeSampled calculates if the transaction should be sampled.
func (as *adaptiveSampler) computeSampled(priority float32, now time.Time) bool {
	as.Lock()
	defer as.Unlock()

	// Never sample anything if the target is zero.  This is not an expected
	// connect reply response, but it is used for the placeholder run (app
	// not connected yet), and is used for testing.
	if 0 == as.target {
		return false
	}

	// If the current time is after the end of the "currentPeriod".  This is in
	// a `for`/`while` loop in case there's a harvest where no sampling happened.
	// i.e. for situations where a single call to
	//    as.currentPeriod.end = as.currentPeriod.end.Add(as.period)
	// might not catch us up to the current period
	for now.After(as.currentPeriod.end) {
		as.priorityMin = 0.0
		if as.currentPeriod.numSeen > 0 {
			sampledRatio := float32(as.target) / float32(as.currentPeriod.numSeen)
			as.priorityMin = 1.0 - sampledRatio
		}
		as.currentPeriod.numSampled = 0
		as.currentPeriod.numSeen = 0
		as.currentPeriod.end = as.currentPeriod.end.Add(as.period)
	}

	as.currentPeriod.numSeen++

	// exponential backoff -- if the number of sampled items is greater than our
	// target, we need to apply the exponential backoff
	if as.currentPeriod.numSampled > as.target {
		if as.computeSampledBackoff(as.target, as.currentPeriod.numSeen, as.currentPeriod.numSampled) {
			as.currentPeriod.numSampled++
			return true
		}
		return false
	}

	if priority >= as.priorityMin {
		as.currentPeriod.numSampled++
		return true
	}

	return false
}

func (as *adaptiveSampler) computeSampledBackoff(target uint64, decidedCount uint64, sampledTrueCount uint64) bool {
	return float64(randUint64N(decidedCount)) <
		math.Pow(float64(target), (float64(target)/float64(sampledTrueCount)))-math.Pow(float64(target), 0.5)
}

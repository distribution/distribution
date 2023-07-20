// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"math/rand"
	"sync"
	"time"
)

var (
	seededRand = struct {
		sync.Mutex
		*rand.Rand
	}{
		Rand: rand.New(rand.NewSource(int64(time.Now().UnixNano()))),
	}
)

// randUint64 returns a random uint64.
//
// IMPORTANT! The default rand package functions are not used, since we want to
// minimize the chance that different Go processes duplicate the same
// transaction id.  (Note that the rand top level functions "use a default
// shared Source that produces a deterministic sequence of values each time a
// program is run" (and we don't seed the shared Source to avoid changing
// customer apps' behavior)).
func randUint64() uint64 {
	seededRand.Lock()
	defer seededRand.Unlock()

	u1 := seededRand.Uint32()
	u2 := seededRand.Uint32()
	return (uint64(u1) << 32) | uint64(u2)
}

// randUint32 returns a random uint32.
func randUint32() uint32 {
	seededRand.Lock()
	defer seededRand.Unlock()

	return seededRand.Uint32()
}

// randFloat32 returns a random float32 in [0.0,1.0).
func randFloat32() float32 {
	seededRand.Lock()
	defer seededRand.Unlock()

	return seededRand.Float32()
}

// randUint64N returns a random int64 that's
// between 0 and the passed in max, non-inclusive
func randUint64N(max uint64) uint64 {
	return randUint64() % max
}

// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"encoding/hex"
	"math/rand"
	"sync"
)

// TraceIDGenerator creates identifiers for distributed tracing.
type TraceIDGenerator struct {
	sync.Mutex
	rnd *rand.Rand
}

// NewTraceIDGenerator creates a new trace identifier generator.
func NewTraceIDGenerator(seed int64) *TraceIDGenerator {
	return &TraceIDGenerator{
		rnd: rand.New(rand.NewSource(seed)),
	}
}

// Float32 returns a random float32 from its random source.
func (tg *TraceIDGenerator) Float32() float32 {
	tg.Lock()
	defer tg.Unlock()

	return tg.rnd.Float32()
}

const (
	traceIDByteLen = 16
	// TraceIDHexStringLen is the length of the trace ID when represented
	// as a hex string.
	TraceIDHexStringLen = 32
	spanIDByteLen       = 8
	maxIDByteLen        = 16
)

// GenerateTraceID creates a new trace identifier, which is a 32 character hex string.
func (tg *TraceIDGenerator) GenerateTraceID() string {
	return tg.generateID(traceIDByteLen)
}

// GenerateSpanID creates a new span identifier, which is a 16 character hex string.
func (tg *TraceIDGenerator) GenerateSpanID() string {
	return tg.generateID(spanIDByteLen)
}

func (tg *TraceIDGenerator) generateID(len int) string {
	var bits [maxIDByteLen]byte
	tg.Lock()
	defer tg.Unlock()
	tg.rnd.Read(bits[:len])
	return hex.EncodeToString(bits[:len])
}

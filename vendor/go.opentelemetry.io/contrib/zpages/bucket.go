// Copyright The OpenTelemetry Authors
// Copyright 2017, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zpages // import "go.opentelemetry.io/contrib/zpages"

import (
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	// defaultBucketCapacity is the default capacity for every bucket (latency or error based).
	defaultBucketCapacity = 10
	// samplePeriod is the minimum time between accepting spans in a single bucket.
	samplePeriod = time.Second
)

// bucket is a container for a set of spans for latency buckets or errored spans.
type bucket struct {
	nextTime  time.Time               // next time we can accept a span
	buffer    []sdktrace.ReadOnlySpan // circular buffer of spans
	nextIndex int                     // location next ReadOnlySpan should be placed in buffer
	overflow  bool                    // whether the circular buffer has wrapped around
}

// newBucket returns a new bucket with the given capacity.
func newBucket(capacity uint) *bucket {
	return &bucket{
		buffer: make([]sdktrace.ReadOnlySpan, capacity),
	}
}

// add adds a span to the bucket, if nextTime has been reached.
func (b *bucket) add(s sdktrace.ReadOnlySpan) {
	if s.EndTime().Before(b.nextTime) {
		return
	}
	if len(b.buffer) == 0 {
		return
	}
	b.nextTime = s.EndTime().Add(samplePeriod)
	b.buffer[b.nextIndex] = s
	b.nextIndex++
	if b.nextIndex == len(b.buffer) {
		b.nextIndex = 0
		b.overflow = true
	}
}

// len returns the number of spans in the bucket.
func (b *bucket) len() int {
	if b.overflow {
		return len(b.buffer)
	}
	return b.nextIndex
}

// spans returns the spans in this bucket.
func (b *bucket) spans() []sdktrace.ReadOnlySpan {
	return append([]sdktrace.ReadOnlySpan(nil), b.buffer[0:b.len()]...)
}

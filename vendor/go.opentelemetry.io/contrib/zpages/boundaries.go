// Copyright The OpenTelemetry Authors
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
	"sort"
	"time"
)

const zeroDuration = time.Duration(0)
const maxDuration = time.Duration(1<<63 - 1)

var defaultBoundaries = newBoundaries([]time.Duration{
	10 * time.Microsecond,
	100 * time.Microsecond,
	time.Millisecond,
	10 * time.Millisecond,
	100 * time.Millisecond,
	time.Second,
	10 * time.Second,
	100 * time.Second,
})

// boundaries represents the interval bounds for the latency based samples.
type boundaries struct {
	durations []time.Duration
}

// newBoundaries returns a new boundaries.
func newBoundaries(durations []time.Duration) *boundaries {
	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})
	return &boundaries{durations: durations}
}

// numBuckets returns the number of buckets needed for these boundaries.
func (lb boundaries) numBuckets() int {
	return len(lb.durations) + 1
}

// getBucketIndex returns the appropriate bucket index for a given latency.
func (lb boundaries) getBucketIndex(latency time.Duration) int {
	i := 0
	for i < len(lb.durations) && latency >= lb.durations[i] {
		i++
	}
	return i
}

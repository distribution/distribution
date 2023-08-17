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

package aggregator // import "go.opentelemetry.io/otel/sdk/metric/aggregator"

import (
	"context"
	"fmt"
	"math"

	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	"go.opentelemetry.io/otel/sdk/metric/number"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
)

// Aggregator implements a specific aggregation behavior, e.g., a
// behavior to track a sequence of updates to an instrument.  Counter
// instruments commonly use a simple Sum aggregator, but for the
// distribution instruments (Histogram, GaugeObserver) there are a
// number of possible aggregators with different cost and accuracy
// tradeoffs.
//
// Note that any Aggregator may be attached to any instrument--this is
// the result of the OpenTelemetry API/SDK separation.  It is possible
// to attach a Sum aggregator to a Histogram instrument.
type Aggregator interface {
	// Aggregation returns an Aggregation interface to access the
	// current state of this Aggregator.  The caller is
	// responsible for synchronization and must not call any the
	// other methods in this interface concurrently while using
	// the Aggregation.
	Aggregation() aggregation.Aggregation

	// Update receives a new measured value and incorporates it
	// into the aggregation.  Update() calls may be called
	// concurrently.
	//
	// Descriptor.NumberKind() should be consulted to determine
	// whether the provided number is an int64 or float64.
	//
	// The Context argument comes from user-level code and could be
	// inspected for a `correlation.Map` or `trace.SpanContext`.
	Update(ctx context.Context, n number.Number, descriptor *sdkapi.Descriptor) error

	// SynchronizedMove is called during collection to finish one
	// period of aggregation by atomically saving the
	// currently-updating state into the argument Aggregator AND
	// resetting the current value to the zero state.
	//
	// SynchronizedMove() is called concurrently with Update().  These
	// two methods must be synchronized with respect to each
	// other, for correctness.
	//
	// After saving a synchronized copy, the Aggregator can be converted
	// into one or more of the interfaces in the `aggregation` sub-package,
	// according to kind of Aggregator that was selected.
	//
	// This method will return an InconsistentAggregatorError if
	// this Aggregator cannot be copied into the destination due
	// to an incompatible type.
	//
	// This call has no Context argument because it is expected to
	// perform only computation.
	//
	// When called with a nil `destination`, this Aggregator is reset
	// and the current value is discarded.
	SynchronizedMove(destination Aggregator, descriptor *sdkapi.Descriptor) error

	// Merge combines the checkpointed state from the argument
	// Aggregator into this Aggregator.  Merge is not synchronized
	// with respect to Update or SynchronizedMove.
	//
	// The owner of an Aggregator being merged is responsible for
	// synchronization of both Aggregator states.
	Merge(aggregator Aggregator, descriptor *sdkapi.Descriptor) error
}

// NewInconsistentAggregatorError formats an error describing an attempt to
// Checkpoint or Merge different-type aggregators.  The result can be unwrapped as
// an ErrInconsistentType.
func NewInconsistentAggregatorError(a1, a2 Aggregator) error {
	return fmt.Errorf("%w: %T and %T", aggregation.ErrInconsistentType, a1, a2)
}

// RangeTest is a common routine for testing for valid input values.
// This rejects NaN values.  This rejects negative values when the
// metric instrument does not support negative values, including
// monotonic counter metrics and absolute Histogram metrics.
func RangeTest(num number.Number, descriptor *sdkapi.Descriptor) error {
	numberKind := descriptor.NumberKind()

	if numberKind == number.Float64Kind && math.IsNaN(num.AsFloat64()) {
		return aggregation.ErrNaNInput
	}

	switch descriptor.InstrumentKind() {
	case sdkapi.CounterInstrumentKind, sdkapi.CounterObserverInstrumentKind:
		if num.IsNegative(numberKind) {
			return aggregation.ErrNegativeInput
		}
	}
	return nil
}

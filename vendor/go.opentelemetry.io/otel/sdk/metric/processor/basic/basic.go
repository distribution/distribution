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

package basic // import "go.opentelemetry.io/otel/sdk/metric/processor/basic"

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/aggregator"
	"go.opentelemetry.io/otel/sdk/metric/export"
	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
)

type (
	// Processor is a basic metric processor.
	Processor struct {
		aggregation.TemporalitySelector
		export.AggregatorSelector

		state
	}

	stateKey struct {
		// TODO: This code is organized to support multiple
		// accumulators which could theoretically produce the
		// data for the same instrument, and this code has
		// logic to combine data properly from multiple
		// accumulators.  However, the use of
		// *sdkapi.Descriptor in the stateKey makes such
		// combination impossible, because each accumulator
		// allocates its own instruments.  This can be fixed
		// by using the instrument name and kind instead of
		// the descriptor pointer.  See
		// https://github.com/open-telemetry/opentelemetry-go/issues/862.
		descriptor *sdkapi.Descriptor
		distinct   attribute.Distinct
	}

	stateValue struct {
		// attrs corresponds to the stateKey.distinct field.
		attrs *attribute.Set

		// updated indicates the last sequence number when this value had
		// Process() called by an accumulator.
		updated int64

		// stateful indicates that a cumulative aggregation is
		// being maintained, taken from the process start time.
		stateful bool

		// currentOwned indicates that "current" was allocated
		// by the processor in order to merge results from
		// multiple Accumulators during a single collection
		// round, which may happen either because:
		// (1) multiple Accumulators output the same Accumulation.
		// (2) one Accumulator is configured with dimensionality reduction.
		currentOwned bool

		// current refers to the output from a single Accumulator
		// (if !currentOwned) or it refers to an Aggregator
		// owned by the processor used to accumulate multiple
		// values in a single collection round.
		current aggregator.Aggregator

		// cumulative, if non-nil, refers to an Aggregator owned
		// by the processor used to store the last cumulative
		// value.
		cumulative aggregator.Aggregator
	}

	state struct {
		config config

		// RWMutex implements locking for the `Reader` interface.
		sync.RWMutex
		values map[stateKey]*stateValue

		processStart  time.Time
		intervalStart time.Time
		intervalEnd   time.Time

		// startedCollection and finishedCollection are the
		// number of StartCollection() and FinishCollection()
		// calls, used to ensure that the sequence of starts
		// and finishes are correctly balanced.

		startedCollection  int64
		finishedCollection int64
	}
)

var _ export.Processor = &Processor{}
var _ export.Checkpointer = &Processor{}
var _ export.Reader = &state{}

// ErrInconsistentState is returned when the sequence of collection's starts and finishes are incorrectly balanced.
var ErrInconsistentState = fmt.Errorf("inconsistent processor state")

// ErrInvalidTemporality is returned for unknown metric.Temporality.
var ErrInvalidTemporality = fmt.Errorf("invalid aggregation temporality")

// New returns a basic Processor that is also a Checkpointer using the provided
// AggregatorSelector to select Aggregators.  The TemporalitySelector
// is consulted to determine the kind(s) of exporter that will consume
// data, so that this Processor can prepare to compute Cumulative Aggregations
// as needed.
func New(aselector export.AggregatorSelector, tselector aggregation.TemporalitySelector, opts ...Option) *Processor {
	return NewFactory(aselector, tselector, opts...).NewCheckpointer().(*Processor)
}

type factory struct {
	aselector export.AggregatorSelector
	tselector aggregation.TemporalitySelector
	config    config
}

// NewFactory returns a new basic CheckpointerFactory.
func NewFactory(aselector export.AggregatorSelector, tselector aggregation.TemporalitySelector, opts ...Option) export.CheckpointerFactory {
	var config config
	for _, opt := range opts {
		config = opt.applyProcessor(config)
	}
	return factory{
		aselector: aselector,
		tselector: tselector,
		config:    config,
	}
}

var _ export.CheckpointerFactory = factory{}

func (f factory) NewCheckpointer() export.Checkpointer {
	now := time.Now()
	p := &Processor{
		AggregatorSelector:  f.aselector,
		TemporalitySelector: f.tselector,
		state: state{
			values:        map[stateKey]*stateValue{},
			processStart:  now,
			intervalStart: now,
			config:        f.config,
		},
	}
	return p
}

// Process implements export.Processor.
func (b *Processor) Process(accum export.Accumulation) error {
	if b.startedCollection != b.finishedCollection+1 {
		return ErrInconsistentState
	}
	desc := accum.Descriptor()
	key := stateKey{
		descriptor: desc,
		distinct:   accum.Attributes().Equivalent(),
	}
	agg := accum.Aggregator()

	// Check if there is an existing value.
	value, ok := b.state.values[key]
	if !ok {
		stateful := b.TemporalityFor(desc, agg.Aggregation().Kind()).MemoryRequired(desc.InstrumentKind())

		newValue := &stateValue{
			attrs:    accum.Attributes(),
			updated:  b.state.finishedCollection,
			stateful: stateful,
			current:  agg,
		}
		if stateful {
			if desc.InstrumentKind().PrecomputedSum() {
				// To convert precomputed sums to
				// deltas requires two aggregators to
				// be allocated, one for the prior
				// value and one for the output delta.
				// This functionality was removed from
				// the basic processor in PR #2350.
				return aggregation.ErrNoCumulativeToDelta
			}
			// In this case allocate one aggregator to
			// save the current state.
			b.AggregatorFor(desc, &newValue.cumulative)
		}
		b.state.values[key] = newValue
		return nil
	}

	// Advance the update sequence number.
	sameCollection := b.state.finishedCollection == value.updated
	value.updated = b.state.finishedCollection

	// At this point in the code, we have located an existing
	// value for some stateKey.  This can be because:
	//
	// (a) stateful aggregation is being used, the entry was
	// entered during a prior collection, and this is the first
	// time processing an accumulation for this stateKey in the
	// current collection.  Since this is the first time
	// processing an accumulation for this stateKey during this
	// collection, we don't know yet whether there are multiple
	// accumulators at work.  If there are multiple accumulators,
	// they'll hit case (b) the second time through.
	//
	// (b) multiple accumulators are being used, whether stateful
	// or not.
	//
	// Case (a) occurs when the instrument and the exporter
	// require memory to work correctly, either because the
	// instrument reports a PrecomputedSum to a DeltaExporter or
	// the reverse, a non-PrecomputedSum instrument with a
	// CumulativeExporter.  This logic is encapsulated in
	// Temporality.MemoryRequired(InstrumentKind).
	//
	// Case (b) occurs when the variable `sameCollection` is true,
	// indicating that the stateKey for Accumulation has already
	// been seen in the same collection.  When this happens, it
	// implies that multiple Accumulators are being used, or that
	// a single Accumulator has been configured with a attribute key
	// filter.

	if !sameCollection {
		if !value.currentOwned {
			// This is the first Accumulation we've seen
			// for this stateKey during this collection.
			// Just keep a reference to the Accumulator's
			// Aggregator.  All the other cases copy
			// Aggregator state.
			value.current = agg
			return nil
		}
		return agg.SynchronizedMove(value.current, desc)
	}

	// If the current is not owned, take ownership of a copy
	// before merging below.
	if !value.currentOwned {
		tmp := value.current
		b.AggregatorSelector.AggregatorFor(desc, &value.current)
		value.currentOwned = true
		if err := tmp.SynchronizedMove(value.current, desc); err != nil {
			return err
		}
	}

	// Combine this Accumulation with the prior Accumulation.
	return value.current.Merge(agg, desc)
}

// Reader returns the associated Reader.  Use the
// Reader Locker interface to synchronize access to this
// object.  The Reader.ForEach() method cannot be called
// concurrently with Process().
func (b *Processor) Reader() export.Reader {
	return &b.state
}

// StartCollection signals to the Processor one or more Accumulators
// will begin calling Process() calls during collection.
func (b *Processor) StartCollection() {
	if b.startedCollection != 0 {
		b.intervalStart = b.intervalEnd
	}
	b.startedCollection++
}

// FinishCollection signals to the Processor that a complete
// collection has finished and that ForEach will be called to access
// the Reader.
func (b *Processor) FinishCollection() error {
	b.intervalEnd = time.Now()
	if b.startedCollection != b.finishedCollection+1 {
		return ErrInconsistentState
	}
	defer func() { b.finishedCollection++ }()

	for key, value := range b.values {
		mkind := key.descriptor.InstrumentKind()
		stale := value.updated != b.finishedCollection
		stateless := !value.stateful

		// The following branch updates stateful aggregators.  Skip
		// these updates if the aggregator is not stateful or if the
		// aggregator is stale.
		if stale || stateless {
			// If this processor does not require memeory,
			// stale, stateless entries can be removed.
			// This implies that they were not updated
			// over the previous full collection interval.
			if stale && stateless && !b.config.Memory {
				delete(b.values, key)
			}
			continue
		}

		// The only kind of aggregators that are not stateless
		// are the ones needing delta to cumulative
		// conversion.  Merge aggregator state in this case.
		if !mkind.PrecomputedSum() {
			// This line is equivalent to:
			// value.cumulative = value.cumulative + value.current
			if err := value.cumulative.Merge(value.current, key.descriptor); err != nil {
				return err
			}
		}
	}
	return nil
}

// ForEach iterates through the Reader, passing an
// export.Record with the appropriate Cumulative or Delta aggregation
// to an exporter.
func (b *state) ForEach(exporter aggregation.TemporalitySelector, f func(export.Record) error) error {
	if b.startedCollection != b.finishedCollection {
		return ErrInconsistentState
	}
	for key, value := range b.values {
		mkind := key.descriptor.InstrumentKind()

		var agg aggregation.Aggregation
		var start time.Time

		aggTemp := exporter.TemporalityFor(key.descriptor, value.current.Aggregation().Kind())

		switch aggTemp {
		case aggregation.CumulativeTemporality:
			// If stateful, the sum has been computed.  If stateless, the
			// input was already cumulative.  Either way, use the checkpointed
			// value:
			if value.stateful {
				agg = value.cumulative.Aggregation()
			} else {
				agg = value.current.Aggregation()
			}
			start = b.processStart

		case aggregation.DeltaTemporality:
			// Precomputed sums are a special case.
			if mkind.PrecomputedSum() {
				// This functionality was removed from
				// the basic processor in PR #2350.
				return aggregation.ErrNoCumulativeToDelta
			}
			agg = value.current.Aggregation()
			start = b.intervalStart

		default:
			return fmt.Errorf("%v: %w", aggTemp, ErrInvalidTemporality)
		}

		// If the processor does not have Config.Memory and it was not updated
		// in the prior round, do not visit this value.
		if !b.config.Memory && value.updated != (b.finishedCollection-1) {
			continue
		}

		if err := f(export.NewRecord(
			key.descriptor,
			value.attrs,
			agg,
			start,
			b.intervalEnd,
		)); err != nil && !errors.Is(err, aggregation.ErrNoData) {
			return err
		}
	}
	return nil
}

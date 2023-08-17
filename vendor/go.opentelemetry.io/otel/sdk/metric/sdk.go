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

package metric // import "go.opentelemetry.io/otel/sdk/metric"

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/sdk/metric/aggregator"
	"go.opentelemetry.io/otel/sdk/metric/export"
	"go.opentelemetry.io/otel/sdk/metric/number"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
)

type (
	// Accumulator implements the OpenTelemetry Meter API.  The
	// Accumulator is bound to a single export.Processor in
	// `NewAccumulator()`.
	//
	// The Accumulator supports a Collect() API to gather and export
	// current data.  Collect() should be arranged according to
	// the processor model.  Push-based processors will setup a
	// timer to call Collect() periodically.  Pull-based processors
	// will call Collect() when a pull request arrives.
	Accumulator struct {
		// current maps `mapkey` to *record.
		current sync.Map

		callbackLock sync.Mutex
		callbacks    map[*callback]struct{}

		// currentEpoch is the current epoch number. It is
		// incremented in `Collect()`.
		currentEpoch int64

		// processor is the configured processor+configuration.
		processor export.Processor

		// collectLock prevents simultaneous calls to Collect().
		collectLock sync.Mutex
	}

	callback struct {
		insts map[*asyncInstrument]struct{}
		f     func(context.Context)
	}

	asyncContextKey struct{}

	asyncInstrument struct {
		baseInstrument
		instrument.Asynchronous
	}

	syncInstrument struct {
		baseInstrument
		instrument.Synchronous
	}

	// mapkey uniquely describes a metric instrument in terms of its
	// InstrumentID and the encoded form of its attributes.
	mapkey struct {
		descriptor *sdkapi.Descriptor
		ordered    attribute.Distinct
	}

	// record maintains the state of one metric instrument.  Due
	// the use of lock-free algorithms, there may be more than one
	// `record` in existence at a time, although at most one can
	// be referenced from the `Accumulator.current` map.
	record struct {
		// refMapped keeps track of refcounts and the mapping state to the
		// Accumulator.current map.
		refMapped refcountMapped

		// updateCount is incremented on every Update.
		updateCount int64

		// collectedCount is set to updateCount on collection,
		// supports checking for no updates during a round.
		collectedCount int64

		// attrs is the stored attribute set for this record, except in cases
		// where a attribute set is shared due to batch recording.
		attrs attribute.Set

		// sortSlice has a single purpose - as a temporary place for sorting
		// during attributes creation to avoid allocation.
		sortSlice attribute.Sortable

		// inst is a pointer to the corresponding instrument.
		inst *baseInstrument

		// current implements the actual RecordOne() API,
		// depending on the type of aggregation.  If nil, the
		// metric was disabled by the exporter.
		current    aggregator.Aggregator
		checkpoint aggregator.Aggregator
	}

	baseInstrument struct {
		meter      *Accumulator
		descriptor sdkapi.Descriptor
	}
)

var (
	_ sdkapi.MeterImpl = &Accumulator{}

	// ErrUninitializedInstrument is returned when an instrument is used when uninitialized.
	ErrUninitializedInstrument = fmt.Errorf("use of an uninitialized instrument")

	// ErrBadInstrument is returned when an instrument from another SDK is
	// attempted to be registered with this SDK.
	ErrBadInstrument = fmt.Errorf("use of a instrument from another SDK")
)

func (b *baseInstrument) Descriptor() sdkapi.Descriptor {
	return b.descriptor
}

func (a *asyncInstrument) Implementation() interface{} {
	return a
}

func (s *syncInstrument) Implementation() interface{} {
	return s
}

// acquireHandle gets or creates a `*record` corresponding to `kvs`,
// the input attributes.
func (b *baseInstrument) acquireHandle(kvs []attribute.KeyValue) *record {
	// This memory allocation may not be used, but it's
	// needed for the `sortSlice` field, to avoid an
	// allocation while sorting.
	rec := &record{}
	rec.attrs = attribute.NewSetWithSortable(kvs, &rec.sortSlice)

	// Create lookup key for sync.Map (one allocation, as this
	// passes through an interface{})
	mk := mapkey{
		descriptor: &b.descriptor,
		ordered:    rec.attrs.Equivalent(),
	}

	if actual, ok := b.meter.current.Load(mk); ok {
		// Existing record case.
		existingRec := actual.(*record)
		if existingRec.refMapped.ref() {
			// At this moment it is guaranteed that the entry is in
			// the map and will not be removed.
			return existingRec
		}
		// This entry is no longer mapped, try to add a new entry.
	}

	rec.refMapped = refcountMapped{value: 2}
	rec.inst = b

	b.meter.processor.AggregatorFor(&b.descriptor, &rec.current, &rec.checkpoint)

	for {
		// Load/Store: there's a memory allocation to place `mk` into
		// an interface here.
		if actual, loaded := b.meter.current.LoadOrStore(mk, rec); loaded {
			// Existing record case. Cannot change rec here because if fail
			// will try to add rec again to avoid new allocations.
			oldRec := actual.(*record)
			if oldRec.refMapped.ref() {
				// At this moment it is guaranteed that the entry is in
				// the map and will not be removed.
				return oldRec
			}
			// This loaded entry is marked as unmapped (so Collect will remove
			// it from the map immediately), try again - this is a busy waiting
			// strategy to wait until Collect() removes this entry from the map.
			//
			// This can be improved by having a list of "Unmapped" entries for
			// one time only usages, OR we can make this a blocking path and use
			// a Mutex that protects the delete operation (delete only if the old
			// record is associated with the key).

			// Let collector get work done to remove the entry from the map.
			runtime.Gosched()
			continue
		}
		// The new entry was added to the map, good to go.
		return rec
	}
}

// RecordOne captures a single synchronous metric event.
//
// The order of the input array `kvs` may be sorted after the function is called.
func (s *syncInstrument) RecordOne(ctx context.Context, num number.Number, kvs []attribute.KeyValue) {
	h := s.acquireHandle(kvs)
	defer h.unbind()
	h.captureOne(ctx, num)
}

// ObserveOne captures a single asynchronous metric event.

// The order of the input array `kvs` may be sorted after the function is called.
func (a *asyncInstrument) ObserveOne(ctx context.Context, num number.Number, attrs []attribute.KeyValue) {
	h := a.acquireHandle(attrs)
	defer h.unbind()
	h.captureOne(ctx, num)
}

// NewAccumulator constructs a new Accumulator for the given
// processor.  This Accumulator supports only a single processor.
//
// The Accumulator does not start any background process to collect itself
// periodically, this responsibility lies with the processor, typically,
// depending on the type of export.  For example, a pull-based
// processor will call Collect() when it receives a request to scrape
// current metric values.  A push-based processor should configure its
// own periodic collection.
func NewAccumulator(processor export.Processor) *Accumulator {
	return &Accumulator{
		processor: processor,
		callbacks: map[*callback]struct{}{},
	}
}

var _ sdkapi.MeterImpl = &Accumulator{}

// NewSyncInstrument implements sdkapi.MetricImpl.
func (m *Accumulator) NewSyncInstrument(descriptor sdkapi.Descriptor) (sdkapi.SyncImpl, error) {
	return &syncInstrument{
		baseInstrument: baseInstrument{
			descriptor: descriptor,
			meter:      m,
		},
	}, nil
}

// NewAsyncInstrument implements sdkapi.MetricImpl.
func (m *Accumulator) NewAsyncInstrument(descriptor sdkapi.Descriptor) (sdkapi.AsyncImpl, error) {
	a := &asyncInstrument{
		baseInstrument: baseInstrument{
			descriptor: descriptor,
			meter:      m,
		},
	}
	return a, nil
}

// RegisterCallback registers f to be called for insts.
func (m *Accumulator) RegisterCallback(insts []instrument.Asynchronous, f func(context.Context)) error {
	cb := &callback{
		insts: map[*asyncInstrument]struct{}{},
		f:     f,
	}
	for _, inst := range insts {
		impl, ok := inst.(sdkapi.AsyncImpl)
		if !ok {
			return ErrBadInstrument
		}

		ai, err := m.fromAsync(impl)
		if err != nil {
			return err
		}
		cb.insts[ai] = struct{}{}
	}

	m.callbackLock.Lock()
	defer m.callbackLock.Unlock()
	m.callbacks[cb] = struct{}{}
	return nil
}

// Collect traverses the list of active records and observers and
// exports data for each active instrument.  Collect() may not be
// called concurrently.
//
// During the collection pass, the export.Processor will receive
// one Export() call per current aggregation.
//
// Returns the number of records that were checkpointed.
func (m *Accumulator) Collect(ctx context.Context) int {
	m.collectLock.Lock()
	defer m.collectLock.Unlock()

	m.runAsyncCallbacks(ctx)
	checkpointed := m.collectInstruments()
	m.currentEpoch++

	return checkpointed
}

func (m *Accumulator) collectInstruments() int {
	checkpointed := 0

	m.current.Range(func(key interface{}, value interface{}) bool {
		// Note: always continue to iterate over the entire
		// map by returning `true` in this function.
		inuse := value.(*record)

		mods := atomic.LoadInt64(&inuse.updateCount)
		coll := inuse.collectedCount

		if mods != coll {
			// Updates happened in this interval,
			// checkpoint and continue.
			checkpointed += m.checkpointRecord(inuse)
			inuse.collectedCount = mods
			return true
		}

		// Having no updates since last collection, try to unmap:
		if unmapped := inuse.refMapped.tryUnmap(); !unmapped {
			// The record is referenced by a binding, continue.
			return true
		}

		// If any other goroutines are now trying to re-insert this
		// entry in the map, they are busy calling Gosched() awaiting
		// this deletion:
		m.current.Delete(inuse.mapkey())

		// There's a potential race between `LoadInt64` and
		// `tryUnmap` in this function.  Since this is the
		// last we'll see of this record, checkpoint
		mods = atomic.LoadInt64(&inuse.updateCount)
		if mods != coll {
			checkpointed += m.checkpointRecord(inuse)
		}
		return true
	})

	return checkpointed
}

func (m *Accumulator) runAsyncCallbacks(ctx context.Context) {
	m.callbackLock.Lock()
	defer m.callbackLock.Unlock()

	ctx = context.WithValue(ctx, asyncContextKey{}, m)

	for cb := range m.callbacks {
		cb.f(ctx)
	}
}

func (m *Accumulator) checkpointRecord(r *record) int {
	if r.current == nil {
		return 0
	}
	err := r.current.SynchronizedMove(r.checkpoint, &r.inst.descriptor)
	if err != nil {
		otel.Handle(err)
		return 0
	}

	a := export.NewAccumulation(&r.inst.descriptor, &r.attrs, r.checkpoint)
	err = m.processor.Process(a)
	if err != nil {
		otel.Handle(err)
	}
	return 1
}

func (r *record) captureOne(ctx context.Context, num number.Number) {
	if r.current == nil {
		// The instrument is disabled according to the AggregatorSelector.
		return
	}
	if err := aggregator.RangeTest(num, &r.inst.descriptor); err != nil {
		otel.Handle(err)
		return
	}
	if err := r.current.Update(ctx, num, &r.inst.descriptor); err != nil {
		otel.Handle(err)
		return
	}
	// Record was modified, inform the Collect() that things need
	// to be collected while the record is still mapped.
	atomic.AddInt64(&r.updateCount, 1)
}

func (r *record) unbind() {
	r.refMapped.unref()
}

func (r *record) mapkey() mapkey {
	return mapkey{
		descriptor: &r.inst.descriptor,
		ordered:    r.attrs.Equivalent(),
	}
}

// fromSync gets an async implementation object, checking for
// uninitialized instruments and instruments created by another SDK.
func (m *Accumulator) fromAsync(async sdkapi.AsyncImpl) (*asyncInstrument, error) {
	if async == nil {
		return nil, ErrUninitializedInstrument
	}
	inst, ok := async.Implementation().(*asyncInstrument)
	if !ok {
		return nil, ErrBadInstrument
	}
	return inst, nil
}

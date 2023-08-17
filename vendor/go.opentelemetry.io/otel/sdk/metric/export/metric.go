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

package export // import "go.opentelemetry.io/otel/sdk/metric/export"

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric/aggregator"
	"go.opentelemetry.io/otel/sdk/metric/export/aggregation"
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
	"go.opentelemetry.io/otel/sdk/resource"
)

// Processor is responsible for deciding which kind of aggregation to
// use (via AggregatorSelector), gathering exported results from the
// SDK during collection, and deciding over which dimensions to group
// the exported data.
//
// The SDK supports binding only one of these interfaces, as it has
// the sole responsibility of determining which Aggregator to use for
// each record.
//
// The embedded AggregatorSelector interface is called (concurrently)
// in instrumentation context to select the appropriate Aggregator for
// an instrument.
//
// The `Process` method is called during collection in a
// single-threaded context from the SDK, after the aggregator is
// checkpointed, allowing the processor to build the set of metrics
// currently being exported.
type Processor interface {
	// AggregatorSelector is responsible for selecting the
	// concrete type of Aggregator used for a metric in the SDK.
	//
	// This may be a static decision based on fields of the
	// Descriptor, or it could use an external configuration
	// source to customize the treatment of each metric
	// instrument.
	//
	// The result from AggregatorSelector.AggregatorFor should be
	// the same type for a given Descriptor or else nil.  The same
	// type should be returned for a given descriptor, because
	// Aggregators only know how to Merge with their own type.  If
	// the result is nil, the metric instrument will be disabled.
	//
	// Note that the SDK only calls AggregatorFor when new records
	// require an Aggregator. This does not provide a way to
	// disable metrics with active records.
	AggregatorSelector

	// Process is called by the SDK once per internal record, passing the
	// export Accumulation (a Descriptor, the corresponding attributes, and
	// the checkpointed Aggregator). This call has no Context argument because
	// it is expected to perform only computation. An SDK is not expected to
	// call exporters from with Process, use a controller for that (see
	// ./controllers/{pull,push}.
	Process(accum Accumulation) error
}

// AggregatorSelector supports selecting the kind of Aggregator to
// use at runtime for a specific metric instrument.
type AggregatorSelector interface {
	// AggregatorFor allocates a variable number of aggregators of
	// a kind suitable for the requested export.  This method
	// initializes a `...*Aggregator`, to support making a single
	// allocation.
	//
	// When the call returns without initializing the *Aggregator
	// to a non-nil value, the metric instrument is explicitly
	// disabled.
	//
	// This must return a consistent type to avoid confusion in
	// later stages of the metrics export process, i.e., when
	// Merging or Checkpointing aggregators for a specific
	// instrument.
	//
	// Note: This is context-free because the aggregator should
	// not relate to the incoming context.  This call should not
	// block.
	AggregatorFor(descriptor *sdkapi.Descriptor, agg ...*aggregator.Aggregator)
}

// Checkpointer is the interface used by a Controller to coordinate
// the Processor with Accumulator(s) and Exporter(s).  The
// StartCollection() and FinishCollection() methods start and finish a
// collection interval.  Controllers call the Accumulator(s) during
// collection to process Accumulations.
type Checkpointer interface {
	// Processor processes metric data for export.  The Process
	// method is bracketed by StartCollection and FinishCollection
	// calls.  The embedded AggregatorSelector can be called at
	// any time.
	Processor

	// Reader returns the current data set.  This may be
	// called before and after collection.  The
	// implementation is required to return the same value
	// throughout its lifetime, since Reader exposes a
	// sync.Locker interface.  The caller is responsible for
	// locking the Reader before initiating collection.
	Reader() Reader

	// StartCollection begins a collection interval.
	StartCollection()

	// FinishCollection ends a collection interval.
	FinishCollection() error
}

// CheckpointerFactory is an interface for producing configured
// Checkpointer instances.
type CheckpointerFactory interface {
	NewCheckpointer() Checkpointer
}

// Exporter handles presentation of the checkpoint of aggregate
// metrics.  This is the final stage of a metrics export pipeline,
// where metric data are formatted for a specific system.
type Exporter interface {
	// Export is called immediately after completing a collection
	// pass in the SDK.
	//
	// The Context comes from the controller that initiated
	// collection.
	//
	// The InstrumentationLibraryReader interface refers to the
	// Processor that just completed collection.
	Export(ctx context.Context, res *resource.Resource, reader InstrumentationLibraryReader) error

	// TemporalitySelector is an interface used by the Processor
	// in deciding whether to compute Delta or Cumulative
	// Aggregations when passing Records to this Exporter.
	aggregation.TemporalitySelector
}

// InstrumentationLibraryReader is an interface for exporters to iterate
// over one instrumentation library of metric data at a time.
type InstrumentationLibraryReader interface {
	// ForEach calls the passed function once per instrumentation library,
	// allowing the caller to emit metrics grouped by the library that
	// produced them.
	ForEach(readerFunc func(instrumentation.Library, Reader) error) error
}

// Reader allows a controller to access a complete checkpoint of
// aggregated metrics from the Processor for a single library of
// metric data.  This is passed to the Exporter which may then use
// ForEach to iterate over the collection of aggregated metrics.
type Reader interface {
	// ForEach iterates over aggregated checkpoints for all
	// metrics that were updated during the last collection
	// period. Each aggregated checkpoint returned by the
	// function parameter may return an error.
	//
	// The TemporalitySelector argument is used to determine
	// whether the Record is computed using Delta or Cumulative
	// aggregation.
	//
	// ForEach tolerates ErrNoData silently, as this is
	// expected from the Meter implementation. Any other kind
	// of error will immediately halt ForEach and return
	// the error to the caller.
	ForEach(tempSelector aggregation.TemporalitySelector, recordFunc func(Record) error) error

	// Locker supports locking the checkpoint set.  Collection
	// into the checkpoint set cannot take place (in case of a
	// stateful processor) while it is locked.
	//
	// The Processor attached to the Accumulator MUST be called
	// with the lock held.
	sync.Locker

	// RLock acquires a read lock corresponding to this Locker.
	RLock()
	// RUnlock releases a read lock corresponding to this Locker.
	RUnlock()
}

// Metadata contains the common elements for exported metric data that
// are shared by the Accumulator->Processor and Processor->Exporter
// steps.
type Metadata struct {
	descriptor *sdkapi.Descriptor
	attrs      *attribute.Set
}

// Accumulation contains the exported data for a single metric instrument
// and attribute set, as prepared by an Accumulator for the Processor.
type Accumulation struct {
	Metadata
	aggregator aggregator.Aggregator
}

// Record contains the exported data for a single metric instrument
// and attribute set, as prepared by the Processor for the Exporter.
// This includes the effective start and end time for the aggregation.
type Record struct {
	Metadata
	aggregation aggregation.Aggregation
	start       time.Time
	end         time.Time
}

// Descriptor describes the metric instrument being exported.
func (m Metadata) Descriptor() *sdkapi.Descriptor {
	return m.descriptor
}

// Attributes returns the attribute set associated with the instrument and the
// aggregated data.
func (m Metadata) Attributes() *attribute.Set {
	return m.attrs
}

// NewAccumulation allows Accumulator implementations to construct new
// Accumulations to send to Processors. The Descriptor, attributes, and
// Aggregator represent aggregate metric events received over a single
// collection period.
func NewAccumulation(descriptor *sdkapi.Descriptor, attrs *attribute.Set, agg aggregator.Aggregator) Accumulation {
	return Accumulation{
		Metadata: Metadata{
			descriptor: descriptor,
			attrs:      attrs,
		},
		aggregator: agg,
	}
}

// Aggregator returns the checkpointed aggregator. It is safe to
// access the checkpointed state without locking.
func (r Accumulation) Aggregator() aggregator.Aggregator {
	return r.aggregator
}

// NewRecord allows Processor implementations to construct export records.
// The Descriptor, attributes, and Aggregator represent aggregate metric
// events received over a single collection period.
func NewRecord(descriptor *sdkapi.Descriptor, attrs *attribute.Set, agg aggregation.Aggregation, start, end time.Time) Record {
	return Record{
		Metadata: Metadata{
			descriptor: descriptor,
			attrs:      attrs,
		},
		aggregation: agg,
		start:       start,
		end:         end,
	}
}

// Aggregation returns the aggregation, an interface to the record and
// its aggregator, dependent on the kind of both the input and exporter.
func (r Record) Aggregation() aggregation.Aggregation {
	return r.aggregation
}

// StartTime is the start time of the interval covered by this aggregation.
func (r Record) StartTime() time.Time {
	return r.start
}

// EndTime is the end time of the interval covered by this aggregation.
func (r Record) EndTime() time.Time {
	return r.end
}

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

//go:generate stringer -type=Temporality

package aggregation // import "go.opentelemetry.io/otel/sdk/metric/export/aggregation"

import (
	"go.opentelemetry.io/otel/sdk/metric/sdkapi"
)

// Temporality indicates the temporal aggregation exported by an exporter.
// These bits may be OR-d together when multiple exporters are in use.
type Temporality uint8

const (
	// CumulativeTemporality indicates that an Exporter expects a
	// Cumulative Aggregation.
	CumulativeTemporality Temporality = 1

	// DeltaTemporality indicates that an Exporter expects a
	// Delta Aggregation.
	DeltaTemporality Temporality = 2
)

// Includes returns if t includes support for other temporality.
func (t Temporality) Includes(other Temporality) bool {
	return t&other != 0
}

// MemoryRequired returns whether an exporter of this temporality requires
// memory to export correctly.
func (t Temporality) MemoryRequired(mkind sdkapi.InstrumentKind) bool {
	switch mkind {
	case sdkapi.HistogramInstrumentKind, sdkapi.GaugeObserverInstrumentKind,
		sdkapi.CounterInstrumentKind, sdkapi.UpDownCounterInstrumentKind:
		// Delta-oriented instruments:
		return t.Includes(CumulativeTemporality)

	case sdkapi.CounterObserverInstrumentKind, sdkapi.UpDownCounterObserverInstrumentKind:
		// Cumulative-oriented instruments:
		return t.Includes(DeltaTemporality)
	}
	// Something unexpected is happening--we could panic.  This
	// will become an error when the exporter tries to access a
	// checkpoint, presumably, so let it be.
	return false
}

type (
	constantTemporalitySelector  Temporality
	statelessTemporalitySelector struct{}
)

var (
	_ TemporalitySelector = constantTemporalitySelector(0)
	_ TemporalitySelector = statelessTemporalitySelector{}
)

// ConstantTemporalitySelector returns an TemporalitySelector that returns
// a constant Temporality.
func ConstantTemporalitySelector(t Temporality) TemporalitySelector {
	return constantTemporalitySelector(t)
}

// CumulativeTemporalitySelector returns an TemporalitySelector that
// always returns CumulativeTemporality.
func CumulativeTemporalitySelector() TemporalitySelector {
	return ConstantTemporalitySelector(CumulativeTemporality)
}

// DeltaTemporalitySelector returns an TemporalitySelector that
// always returns DeltaTemporality.
func DeltaTemporalitySelector() TemporalitySelector {
	return ConstantTemporalitySelector(DeltaTemporality)
}

// StatelessTemporalitySelector returns an TemporalitySelector that
// always returns the Temporality that avoids long-term memory
// requirements.
func StatelessTemporalitySelector() TemporalitySelector {
	return statelessTemporalitySelector{}
}

// TemporalityFor implements TemporalitySelector.
func (c constantTemporalitySelector) TemporalityFor(_ *sdkapi.Descriptor, _ Kind) Temporality {
	return Temporality(c)
}

// TemporalityFor implements TemporalitySelector.
func (s statelessTemporalitySelector) TemporalityFor(desc *sdkapi.Descriptor, kind Kind) Temporality {
	if kind == SumKind && desc.InstrumentKind().PrecomputedSum() {
		return CumulativeTemporality
	}
	return DeltaTemporality
}

// TemporalitySelector is a sub-interface of Exporter used to indicate
// whether the Processor should compute Delta or Cumulative
// Aggregations.
type TemporalitySelector interface {
	// TemporalityFor should return the correct Temporality that
	// should be used when exporting data for the given metric
	// instrument and Aggregator kind.
	TemporalityFor(descriptor *sdkapi.Descriptor, aggregationKind Kind) Temporality
}

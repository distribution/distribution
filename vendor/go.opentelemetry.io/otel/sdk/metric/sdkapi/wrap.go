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

package sdkapi // import "go.opentelemetry.io/otel/sdk/metric/sdkapi"

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/instrument"
	"go.opentelemetry.io/otel/metric/instrument/asyncfloat64"
	"go.opentelemetry.io/otel/metric/instrument/asyncint64"
	"go.opentelemetry.io/otel/metric/instrument/syncfloat64"
	"go.opentelemetry.io/otel/metric/instrument/syncint64"
	"go.opentelemetry.io/otel/sdk/metric/number"
)

type (
	meter   struct{ MeterImpl }
	sfMeter struct{ meter }
	siMeter struct{ meter }
	afMeter struct{ meter }
	aiMeter struct{ meter }

	iAdder    struct{ SyncImpl }
	fAdder    struct{ SyncImpl }
	iRecorder struct{ SyncImpl }
	fRecorder struct{ SyncImpl }
	iObserver struct{ AsyncImpl }
	fObserver struct{ AsyncImpl }
)

// WrapMeterImpl wraps impl to be a full implementation of a Meter.
func WrapMeterImpl(impl MeterImpl) metric.Meter {
	return meter{impl}
}

// UnwrapMeterImpl unwraps the Meter to its bare MeterImpl.
func UnwrapMeterImpl(m metric.Meter) MeterImpl {
	mm, ok := m.(meter)
	if !ok {
		return nil
	}
	return mm.MeterImpl
}

func (m meter) AsyncFloat64() asyncfloat64.InstrumentProvider {
	return afMeter{m}
}

func (m meter) AsyncInt64() asyncint64.InstrumentProvider {
	return aiMeter{m}
}

func (m meter) SyncFloat64() syncfloat64.InstrumentProvider {
	return sfMeter{m}
}

func (m meter) SyncInt64() syncint64.InstrumentProvider {
	return siMeter{m}
}

func (m meter) RegisterCallback(insts []instrument.Asynchronous, cb func(ctx context.Context)) error {
	return m.MeterImpl.RegisterCallback(insts, cb)
}

func (m meter) newSync(name string, ikind InstrumentKind, nkind number.Kind, opts []instrument.Option) (SyncImpl, error) {
	cfg := instrument.NewConfig(opts...)
	return m.NewSyncInstrument(NewDescriptor(name, ikind, nkind, cfg.Description(), cfg.Unit()))
}

func (m meter) newAsync(name string, ikind InstrumentKind, nkind number.Kind, opts []instrument.Option) (AsyncImpl, error) {
	cfg := instrument.NewConfig(opts...)
	return m.NewAsyncInstrument(NewDescriptor(name, ikind, nkind, cfg.Description(), cfg.Unit()))
}

func (m afMeter) Counter(name string, opts ...instrument.Option) (asyncfloat64.Counter, error) {
	inst, err := m.newAsync(name, CounterObserverInstrumentKind, number.Float64Kind, opts)
	return fObserver{inst}, err
}

func (m afMeter) UpDownCounter(name string, opts ...instrument.Option) (asyncfloat64.UpDownCounter, error) {
	inst, err := m.newAsync(name, UpDownCounterObserverInstrumentKind, number.Float64Kind, opts)
	return fObserver{inst}, err
}

func (m afMeter) Gauge(name string, opts ...instrument.Option) (asyncfloat64.Gauge, error) {
	inst, err := m.newAsync(name, GaugeObserverInstrumentKind, number.Float64Kind, opts)
	return fObserver{inst}, err
}

func (m aiMeter) Counter(name string, opts ...instrument.Option) (asyncint64.Counter, error) {
	inst, err := m.newAsync(name, CounterObserverInstrumentKind, number.Int64Kind, opts)
	return iObserver{inst}, err
}

func (m aiMeter) UpDownCounter(name string, opts ...instrument.Option) (asyncint64.UpDownCounter, error) {
	inst, err := m.newAsync(name, UpDownCounterObserverInstrumentKind, number.Int64Kind, opts)
	return iObserver{inst}, err
}

func (m aiMeter) Gauge(name string, opts ...instrument.Option) (asyncint64.Gauge, error) {
	inst, err := m.newAsync(name, GaugeObserverInstrumentKind, number.Int64Kind, opts)
	return iObserver{inst}, err
}

func (m sfMeter) Counter(name string, opts ...instrument.Option) (syncfloat64.Counter, error) {
	inst, err := m.newSync(name, CounterInstrumentKind, number.Float64Kind, opts)
	return fAdder{inst}, err
}

func (m sfMeter) UpDownCounter(name string, opts ...instrument.Option) (syncfloat64.UpDownCounter, error) {
	inst, err := m.newSync(name, UpDownCounterInstrumentKind, number.Float64Kind, opts)
	return fAdder{inst}, err
}

func (m sfMeter) Histogram(name string, opts ...instrument.Option) (syncfloat64.Histogram, error) {
	inst, err := m.newSync(name, HistogramInstrumentKind, number.Float64Kind, opts)
	return fRecorder{inst}, err
}

func (m siMeter) Counter(name string, opts ...instrument.Option) (syncint64.Counter, error) {
	inst, err := m.newSync(name, CounterInstrumentKind, number.Int64Kind, opts)
	return iAdder{inst}, err
}

func (m siMeter) UpDownCounter(name string, opts ...instrument.Option) (syncint64.UpDownCounter, error) {
	inst, err := m.newSync(name, UpDownCounterInstrumentKind, number.Int64Kind, opts)
	return iAdder{inst}, err
}

func (m siMeter) Histogram(name string, opts ...instrument.Option) (syncint64.Histogram, error) {
	inst, err := m.newSync(name, HistogramInstrumentKind, number.Int64Kind, opts)
	return iRecorder{inst}, err
}

func (a fAdder) Add(ctx context.Context, value float64, attrs ...attribute.KeyValue) {
	if a.SyncImpl != nil {
		a.SyncImpl.RecordOne(ctx, number.NewFloat64Number(value), attrs)
	}
}

func (a iAdder) Add(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	if a.SyncImpl != nil {
		a.SyncImpl.RecordOne(ctx, number.NewInt64Number(value), attrs)
	}
}

func (a fRecorder) Record(ctx context.Context, value float64, attrs ...attribute.KeyValue) {
	if a.SyncImpl != nil {
		a.SyncImpl.RecordOne(ctx, number.NewFloat64Number(value), attrs)
	}
}

func (a iRecorder) Record(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	if a.SyncImpl != nil {
		a.SyncImpl.RecordOne(ctx, number.NewInt64Number(value), attrs)
	}
}

func (a fObserver) Observe(ctx context.Context, value float64, attrs ...attribute.KeyValue) {
	if a.AsyncImpl != nil {
		a.AsyncImpl.ObserveOne(ctx, number.NewFloat64Number(value), attrs)
	}
}

func (a iObserver) Observe(ctx context.Context, value int64, attrs ...attribute.KeyValue) {
	if a.AsyncImpl != nil {
		a.AsyncImpl.ObserveOne(ctx, number.NewInt64Number(value), attrs)
	}
}

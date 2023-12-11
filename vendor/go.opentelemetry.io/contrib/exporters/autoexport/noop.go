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

package autoexport // import "go.opentelemetry.io/contrib/exporters/autoexport"

import (
	"context"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace"
)

// noopSpanExporter is an implementation of trace.SpanExporter that performs no operations.
type noopSpanExporter struct{}

var _ trace.SpanExporter = noopSpanExporter{}

// ExportSpans is part of trace.SpanExporter interface.
func (e noopSpanExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	return nil
}

// Shutdown is part of trace.SpanExporter interface.
func (e noopSpanExporter) Shutdown(ctx context.Context) error {
	return nil
}

// IsNoneSpanExporter returns true for the exporter returned by [NewSpanExporter]
// when OTEL_TRACES_EXPORTER environment variable is set to "none".
func IsNoneSpanExporter(e trace.SpanExporter) bool {
	_, ok := e.(noopSpanExporter)
	return ok
}

type noopMetricReader struct {
	*metric.ManualReader
}

func newNoopMetricReader() noopMetricReader {
	return noopMetricReader{metric.NewManualReader()}
}

// IsNoneMetricReader returns true for the exporter returned by [NewMetricReader]
// when OTEL_METRICS_EXPORTER environment variable is set to "none".
func IsNoneMetricReader(e metric.Reader) bool {
	_, ok := e.(noopMetricReader)
	return ok
}

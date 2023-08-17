// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package otlptext // import "go.opentelemetry.io/collector/exporter/loggingexporter/internal/otlptext"

import (
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// NewTextTracesMarshaler returns a ptrace.Marshaler to encode to OTLP text bytes.
func NewTextTracesMarshaler() ptrace.Marshaler {
	return textTracesMarshaler{}
}

type textTracesMarshaler struct{}

// MarshalTraces ptrace.Traces to OTLP text.
func (textTracesMarshaler) MarshalTraces(td ptrace.Traces) ([]byte, error) {
	buf := dataBuffer{}
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		buf.logEntry("ResourceSpans #%d", i)
		rs := rss.At(i)
		buf.logEntry("Resource SchemaURL: %s", rs.SchemaUrl())
		buf.logAttributes("Resource labels", rs.Resource().Attributes())
		ilss := rs.ScopeSpans()
		for j := 0; j < ilss.Len(); j++ {
			buf.logEntry("ScopeSpans #%d", j)
			ils := ilss.At(j)
			buf.logEntry("ScopeSpans SchemaURL: %s", ils.SchemaUrl())
			buf.logInstrumentationScope(ils.Scope())

			spans := ils.Spans()
			for k := 0; k < spans.Len(); k++ {
				buf.logEntry("Span #%d", k)
				span := spans.At(k)
				buf.logAttr("Trace ID", span.TraceID().HexString())
				buf.logAttr("Parent ID", span.ParentSpanID().HexString())
				buf.logAttr("ID", span.SpanID().HexString())
				buf.logAttr("Name", span.Name())
				buf.logAttr("Kind", span.Kind().String())
				buf.logAttr("Start time", span.StartTimestamp().String())
				buf.logAttr("End time", span.EndTimestamp().String())

				buf.logAttr("Status code", span.Status().Code().String())
				buf.logAttr("Status message", span.Status().Message())

				buf.logAttributes("Attributes", span.Attributes())
				buf.logEvents("Events", span.Events())
				buf.logLinks("Links", span.Links())
			}
		}
	}

	return buf.buf.Bytes(), nil
}

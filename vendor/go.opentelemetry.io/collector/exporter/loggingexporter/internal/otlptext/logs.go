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
	"go.opentelemetry.io/collector/pdata/plog"
)

// NewTextLogsMarshaler returns a plog.Marshaler to encode to OTLP text bytes.
func NewTextLogsMarshaler() plog.Marshaler {
	return textLogsMarshaler{}
}

type textLogsMarshaler struct{}

// MarshalLogs plog.Logs to OTLP text.
func (textLogsMarshaler) MarshalLogs(ld plog.Logs) ([]byte, error) {
	buf := dataBuffer{}
	rls := ld.ResourceLogs()
	for i := 0; i < rls.Len(); i++ {
		buf.logEntry("ResourceLog #%d", i)
		rl := rls.At(i)
		buf.logEntry("Resource SchemaURL: %s", rl.SchemaUrl())
		buf.logAttributes("Resource labels", rl.Resource().Attributes())
		ills := rl.ScopeLogs()
		for j := 0; j < ills.Len(); j++ {
			buf.logEntry("ScopeLogs #%d", j)
			ils := ills.At(j)
			buf.logEntry("ScopeLogs SchemaURL: %s", ils.SchemaUrl())
			buf.logInstrumentationScope(ils.Scope())

			logs := ils.LogRecords()
			for k := 0; k < logs.Len(); k++ {
				buf.logEntry("LogRecord #%d", k)
				lr := logs.At(k)
				buf.logEntry("ObservedTimestamp: %s", lr.ObservedTimestamp())
				buf.logEntry("Timestamp: %s", lr.Timestamp())
				buf.logEntry("Severity: %s", lr.SeverityText())
				buf.logEntry("Body: %s", attributeValueToString(lr.Body()))
				buf.logAttributes("Attributes", lr.Attributes())
				buf.logEntry("Trace ID: %s", lr.TraceID().HexString())
				buf.logEntry("Span ID: %s", lr.SpanID().HexString())
				buf.logEntry("Flags: %d", lr.Flags())
			}
		}
	}

	return buf.buf.Bytes(), nil
}

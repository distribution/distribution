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
	"os"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/trace"
)

// SpanOption applies an autoexport configuration option.
type SpanOption = option[trace.SpanExporter]

// Option applies an autoexport configuration option.
//
// Deprecated: Use SpanOption.
type Option = SpanOption

// WithFallbackSpanExporter sets the fallback exporter to use when no exporter
// is configured through the OTEL_TRACES_EXPORTER environment variable.
func WithFallbackSpanExporter(exporter trace.SpanExporter) SpanOption {
	return withFallback[trace.SpanExporter](exporter)
}

// NewSpanExporter returns a configured [go.opentelemetry.io/otel/sdk/trace.SpanExporter]
// defined using the environment variables described below.
//
// OTEL_TRACES_EXPORTER defines the traces exporter; supported values:
//   - "none" - "no operation" exporter
//   - "otlp" (default) - OTLP exporter; see [go.opentelemetry.io/otel/exporters/otlp/otlptrace]
//   - "console" - Standard output exporter; see [go.opentelemetry.io/otel/exporters/stdout/stdouttrace]
//
// OTEL_EXPORTER_OTLP_PROTOCOL defines OTLP exporter's transport protocol;
// supported values:
//   - "grpc" - protobuf-encoded data using gRPC wire format over HTTP/2 connection;
//     see: [go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc]
//   - "http/protobuf" (default) -  protobuf-encoded data over HTTP connection;
//     see: [go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp]
//
// An error is returned if an environment value is set to an unhandled value.
//
// Use [RegisterSpanExporter] to handle more values of OTEL_TRACES_EXPORTER.
//
// Use [WithFallbackSpanExporter] option to change the returned exporter
// when OTEL_TRACES_EXPORTER is unset or empty.
//
// Use [IsNoneSpanExporter] to check if the retured exporter is a "no operation" exporter.
func NewSpanExporter(ctx context.Context, opts ...SpanOption) (trace.SpanExporter, error) {
	return tracesSignal.create(ctx, opts...)
}

// RegisterSpanExporter sets the SpanExporter factory to be used when the
// OTEL_TRACES_EXPORTERS environment variable contains the exporter name. This
// will panic if name has already been registered.
func RegisterSpanExporter(name string, factory func(context.Context) (trace.SpanExporter, error)) {
	must(tracesSignal.registry.store(name, factory))
}

var tracesSignal = newSignal[trace.SpanExporter]("OTEL_TRACES_EXPORTER")

func init() {
	RegisterSpanExporter("otlp", func(ctx context.Context) (trace.SpanExporter, error) {
		proto := os.Getenv(otelExporterOTLPProtoEnvKey)
		if proto == "" {
			proto = "http/protobuf"
		}

		switch proto {
		case "grpc":
			return otlptracegrpc.New(ctx)
		case "http/protobuf":
			return otlptracehttp.New(ctx)
		default:
			return nil, errInvalidOTLPProtocol
		}
	})
	RegisterSpanExporter("console", func(ctx context.Context) (trace.SpanExporter, error) {
		return stdouttrace.New()
	})
	RegisterSpanExporter("none", func(ctx context.Context) (trace.SpanExporter, error) {
		return noopSpanExporter{}, nil
	})
}

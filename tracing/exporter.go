package tracing

import (
	"context"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// compositeExporter is a custom exporter that wraps multiple SpanExporters.
// It allows you to export spans to multiple destinations, e.g., different telemetry backends.
type compositeExporter struct {
	exporters []sdktrace.SpanExporter
}

func newCompositeExporter(exporters ...sdktrace.SpanExporter) *compositeExporter {
	return &compositeExporter{exporters: exporters}
}

// ExportSpans iterates over each SpanExporter in the compositeExporter and
// exports the spans. If any exporter returns an error, the process is stopped
// and the error is returned. This ensures that span exporting behaves correctly
// and reports errors as expected.
func (ce *compositeExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, exporter := range ce.exporters {
		if err := exporter.ExportSpans(ctx, spans); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown iterates over each SpanExporter in the compositeExporter and
// shuts them down. If any exporter returns an error during shutdown, the process
// is stopped and the error is returned. This ensures proper shutdown of all exporters.
func (ce *compositeExporter) Shutdown(ctx context.Context) error {
	for _, exporter := range ce.exporters {
		if err := exporter.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}

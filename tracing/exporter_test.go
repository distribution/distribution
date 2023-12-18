package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/sdk/trace"
)

type mockSpanExporter struct {
	exportSpansCalled bool
	shutdownCalled    bool
	returnError       bool
}

func (m *mockSpanExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	m.exportSpansCalled = true
	if m.returnError {
		return errors.New("export error")
	}
	return nil
}

func (m *mockSpanExporter) Shutdown(ctx context.Context) error {
	m.shutdownCalled = true
	if m.returnError {
		return errors.New("shutdown error")
	}
	return nil
}
func TestCompositeExporterExportSpans(t *testing.T) {
	mockExporter1 := &mockSpanExporter{}
	mockExporter2 := &mockSpanExporter{}
	composite := newCompositeExporter(mockExporter1, mockExporter2)

	err := composite.ExportSpans(context.Background(), nil)
	if err != nil {
		t.Errorf("ExportSpans() error = %v", err)
	}

	if !mockExporter1.exportSpansCalled || !mockExporter2.exportSpansCalled {
		t.Error("ExportSpans was not called on all exporters")
	}
}

func TestCompositeExporterExportSpans_Error(t *testing.T) {
	mockExporter1 := &mockSpanExporter{returnError: true}
	mockExporter2 := &mockSpanExporter{}
	composite := newCompositeExporter(mockExporter1, mockExporter2)

	err := composite.ExportSpans(context.Background(), nil)
	if err == nil {
		t.Error("Expected error from ExportSpans, but got none")
	}
}

func TestCompositeExporterShutdown(t *testing.T) {
	mockExporter1 := &mockSpanExporter{}
	mockExporter2 := &mockSpanExporter{}
	composite := newCompositeExporter(mockExporter1, mockExporter2)

	err := composite.Shutdown(context.Background())
	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	if !mockExporter1.shutdownCalled || !mockExporter2.shutdownCalled {
		t.Error("Shutdown was not called on all exporters")
	}
}

func TestCompositeExporterShutdown_Error(t *testing.T) {
	mockExporter1 := &mockSpanExporter{returnError: true}
	mockExporter2 := &mockSpanExporter{}
	composite := newCompositeExporter(mockExporter1, mockExporter2)

	err := composite.Shutdown(context.Background())
	if err == nil {
		t.Error("Expected error from Shutdown, but got none")
	}
}

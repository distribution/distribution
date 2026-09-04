package tracing

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestInitOpenTelemetryNoneDoesNotLogSpans(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")

	var output bytes.Buffer
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(&output)
	ctx := dcontext.WithLogger(context.Background(), logrus.NewEntry(logger))

	previousProvider := otel.GetTracerProvider()

	if err := InitOpenTelemetry(ctx); err != nil {
		t.Fatal(err)
	}
	provider := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previousProvider)
	})

	_, span := otel.Tracer("test").Start(ctx, "disabled-exporter-span")
	span.End()
	if err := provider.ForceFlush(ctx); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(output.String(), "disabled-exporter-span") {
		t.Fatalf("span was logged with OTEL_TRACES_EXPORTER=none: %s", output.String())
	}
}

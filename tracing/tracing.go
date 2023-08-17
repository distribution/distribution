package tracing

import (
	"context"
	"fmt"
	"time"

	"github.com/distribution/distribution/v3/configuration"

	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// InitOpenTelemetry reads config and initializes otel middleware, sets the exporter
// propagator and global tracer provider
func InitOpenTelemetry(ctx context.Context, config *configuration.OpenTelemetryConfig) (func(), error) {
	// Check if tracing is configured
	if config == nil {
		logrus.Info("OpenTelemetry configuration not found, tracing is disabled")
		return nil, nil
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid open telemetry configuration: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			// Service name used to displace traces in backends
			semconv.ServiceNameKey.String(config.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Configure OTLP trace exporter and set it up to connect to OpenTelemetry collector
	// running on a local host.
	ctrdTraceExporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(config.Exporter.Endpoint),
		otlptracegrpc.WithTimeout(time.Second*10),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Register the trace exporter with a TracerProvider, using a batch span
	// process to aggregate spans before export.
	ctrdBatchSpanProcessor := sdktrace.NewBatchSpanProcessor(ctrdTraceExporter)
	ctrdTracerProvider := sdktrace.NewTracerProvider(
		// We use TraceIDRatioBased sampling. Ratio read from config translated into following
		// if sampling ratio < 0 it is interpreted as 0. If ratio >= 1, it will always sample.
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.TraceSamplingRatio)),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(ctrdBatchSpanProcessor),
	)
	otel.SetTracerProvider(ctrdTracerProvider)

	// set global propagator to tracecontext
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return func() {
		// TraceShutdown will flush any remaining spans and shut down the exporter.
		err := ctrdTracerProvider.Shutdown(ctx)
		if err != nil {
			logrus.WithError(err).Errorf("failed to shutdown TracerProvider")
		}
	}, nil
}

// StartSpan starts child span in a context.
func StartSpan(ctx context.Context, opName string, opts ...trace.SpanStartOption) (trace.Span, context.Context) {
	parentSpan := trace.SpanFromContext(ctx)
	var tracer trace.Tracer
	if parentSpan.SpanContext().IsValid() {
		tracer = parentSpan.TracerProvider().Tracer("")
	} else {
		tracer = otel.Tracer(configuration.ServiceName)
	}
	ctx, span := tracer.Start(ctx, opName, opts...)
	return span, ctx
}

// StopSpan ends the span specified
func StopSpan(span trace.Span) {
	span.End()
}

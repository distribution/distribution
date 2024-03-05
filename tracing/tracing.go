package tracing

import (
	"context"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/version"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

const (
	// ServiceName is trace service name
	serviceName = "distribution"

	// DefaultSamplingRatio default sample ratio
	defaultSamplingRatio = 1

	// AttributePrefix defines a standardized prefix for custom telemetry attributes
	// associated with the CNCF Distribution project.
	AttributePrefix = "io.cncf.distribution."
)

// InitOpenTelemetry initializes OpenTelemetry for the application. This function sets up the
// necessary components for collecting telemetry data, such as traces.
func InitOpenTelemetry(ctx context.Context) error {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(version.Version()),
	)

	autoExp, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return err
	}

	lw := &loggerWriter{
		logger: dcontext.GetLogger(ctx),
	}

	loggerExp, err := stdouttrace.New(stdouttrace.WithWriter(lw))
	if err != nil {
		return err
	}

	compositeExp := newCompositeExporter(autoExp, loggerExp)

	sp := sdktrace.NewBatchSpanProcessor(compositeExp)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(defaultSamplingRatio)),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(sp),
	)
	otel.SetTracerProvider(provider)
	otel.SetErrorHandler(lw)

	pr := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTextMapPropagator(pr)

	return nil
}

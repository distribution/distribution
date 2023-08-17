package tracing

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/exporter/loggingexporter"
	"go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
	"go.opentelemetry.io/collector/extension/ballastextension"
	"go.opentelemetry.io/collector/extension/zpagesextension"
	"go.opentelemetry.io/collector/processor/batchprocessor"
	"go.opentelemetry.io/collector/processor/memorylimiterprocessor"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/service"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type TracingSuite struct {
	suite.Suite
	ctx    context.Context
	host   string
	cancel context.CancelFunc
	wg     *sync.WaitGroup
	col    *service.Collector
}

func (s *TracingSuite) SetupSuite() {
	s.ctx = context.Background()
	s.host = "127.0.0.1:4317"
	components := func() (component.Factories, error) {
		var err error
		factories := component.Factories{}

		factories.Extensions, err = component.MakeExtensionFactoryMap(
			ballastextension.NewFactory(),
			zpagesextension.NewFactory(),
		)
		if err != nil {
			return component.Factories{}, err
		}

		factories.Receivers, err = component.MakeReceiverFactoryMap(
			otlpreceiver.NewFactory(),
		)
		if err != nil {
			return component.Factories{}, err
		}

		factories.Exporters, err = component.MakeExporterFactoryMap(
			loggingexporter.NewFactory(),
			otlpexporter.NewFactory(),
			otlphttpexporter.NewFactory(),
		)
		if err != nil {
			return component.Factories{}, err
		}

		factories.Processors, err = component.MakeProcessorFactoryMap(
			batchprocessor.NewFactory(),
			memorylimiterprocessor.NewFactory(),
		)
		if err != nil {
			return component.Factories{}, err
		}

		return factories, nil
	}
	factories, err := components()
	require.NoError(s.T(), err)
	makeMapProvidersMap := func(providers ...confmap.Provider) map[string]confmap.Provider {
		ret := make(map[string]confmap.Provider, len(providers))
		for _, provider := range providers {
			ret[provider.Scheme()] = provider
		}
		return ret
	}
	cfg := service.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs:       []string{filepath.Join("testdata", "otelcol-address.yaml")},
			Providers:  makeMapProvidersMap(fileprovider.New(), envprovider.New(), yamlprovider.New()),
			Converters: []confmap.Converter{expandconverter.New()},
		},
	}
	cfgProvider, err := service.NewConfigProvider(cfg)
	require.NoError(s.T(), err)

	set := service.CollectorSettings{
		BuildInfo:      component.NewDefaultBuildInfo(),
		Factories:      factories,
		ConfigProvider: cfgProvider,
	}
	col, err := service.New(set)
	require.NoError(s.T(), err)
	s.col = col
	ctx, cancel := context.WithCancel(context.Background())
	startCollector := func(ctx context.Context, t *testing.T, col *service.Collector) *sync.WaitGroup {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, col.Run(ctx))
		}()
		return wg
	}
	s.wg = startCollector(ctx, s.T(), col)
	s.cancel = cancel
	assert.Eventually(s.T(), func() bool {
		return service.Running == col.GetState()
	}, 2*time.Second, 200*time.Millisecond)
}

func (s *TracingSuite) TearDownSuite() {
	s.ctx.Done()
	s.cancel()
	s.wg.Wait()
	s.T().Log("Opentelemetry Collector Stop Success.")
}

func (s *TracingSuite) TestInitOpenTelemetrySuit() {
	config := &configuration.OpenTelemetryConfig{
		Exporter: configuration.ExporterConfig{
			Name:     "otlp",
			Endpoint: s.host,
		},
	}
	op, err := InitOpenTelemetry(s.ctx, config)
	s.Nil(err)
	s.NotNil(op)

	span, spanCtx := StartSpan(s.ctx, fmt.Sprintf("%s:%s", "TestInitOpenTelemetrySuit", "Stat"),
		trace.WithAttributes(attribute.String("id", "1")))
	StopSpan(span)

	s.NotNil(spanCtx)
	s.NotNil(span)
	spanID := span.SpanContext().SpanID()
	traceID := span.SpanContext().TraceID()
	s.NotNil(spanID)
	s.NotNil(traceID)
}

func TestInitOpenTelemetry(t *testing.T) {
	suite.Run(t, new(TracingSuite))
}

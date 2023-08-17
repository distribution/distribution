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

package otlphttpexporter // import "go.opentelemetry.io/collector/exporter/otlphttpexporter"

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/config/configcompression"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	// The value of "type" key in configuration.
	typeStr = "otlphttp"
)

// NewFactory creates a factory for OTLP exporter.
func NewFactory() component.ExporterFactory {
	return component.NewExporterFactory(
		typeStr,
		createDefaultConfig,
		component.WithTracesExporter(createTracesExporter, component.StabilityLevelStable),
		component.WithMetricsExporter(createMetricsExporter, component.StabilityLevelStable),
		component.WithLogsExporter(createLogsExporter, component.StabilityLevelBeta),
	)
}

func createDefaultConfig() config.Exporter {
	return &Config{
		ExporterSettings: config.NewExporterSettings(config.NewComponentID(typeStr)),
		RetrySettings:    exporterhelper.NewDefaultRetrySettings(),
		QueueSettings:    exporterhelper.NewDefaultQueueSettings(),
		HTTPClientSettings: confighttp.HTTPClientSettings{
			Endpoint: "",
			Timeout:  30 * time.Second,
			Headers:  map[string]string{},
			// Default to gzip compression
			Compression: configcompression.Gzip,
			// We almost read 0 bytes, so no need to tune ReadBufferSize.
			WriteBufferSize: 512 * 1024,
		},
	}
}

func composeSignalURL(oCfg *Config, signalOverrideURL string, signalName string) (string, error) {
	switch {
	case signalOverrideURL != "":
		_, err := url.Parse(signalOverrideURL)
		if err != nil {
			return "", fmt.Errorf("%s_endpoint must be a valid URL", signalName)
		}
		return signalOverrideURL, nil
	case oCfg.Endpoint == "":
		return "", fmt.Errorf("either endpoint or %s_endpoint must be specified", signalName)
	default:
		return oCfg.Endpoint + "/v1/" + signalName, nil
	}
}

func createTracesExporter(
	ctx context.Context,
	set component.ExporterCreateSettings,
	cfg config.Exporter,
) (component.TracesExporter, error) {
	oce, err := newExporter(cfg, set)
	if err != nil {
		return nil, err
	}
	oCfg := cfg.(*Config)

	oce.tracesURL, err = composeSignalURL(oCfg, oCfg.TracesEndpoint, "traces")
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewTracesExporterWithContext(ctx, set, cfg,
		oce.pushTraces,
		exporterhelper.WithStart(oce.start),
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// explicitly disable since we rely on http.Client timeout logic.
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(oCfg.RetrySettings),
		exporterhelper.WithQueue(oCfg.QueueSettings))
}

func createMetricsExporter(
	ctx context.Context,
	set component.ExporterCreateSettings,
	cfg config.Exporter,
) (component.MetricsExporter, error) {
	oce, err := newExporter(cfg, set)
	if err != nil {
		return nil, err
	}
	oCfg := cfg.(*Config)

	oce.metricsURL, err = composeSignalURL(oCfg, oCfg.MetricsEndpoint, "metrics")
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewMetricsExporterWithContext(ctx, set, cfg,
		oce.pushMetrics,
		exporterhelper.WithStart(oce.start),
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// explicitly disable since we rely on http.Client timeout logic.
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(oCfg.RetrySettings),
		exporterhelper.WithQueue(oCfg.QueueSettings))
}

func createLogsExporter(
	ctx context.Context,
	set component.ExporterCreateSettings,
	cfg config.Exporter,
) (component.LogsExporter, error) {
	oce, err := newExporter(cfg, set)
	if err != nil {
		return nil, err
	}
	oCfg := cfg.(*Config)

	oce.logsURL, err = composeSignalURL(oCfg, oCfg.LogsEndpoint, "logs")
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewLogsExporterWithContext(ctx, set, cfg,
		oce.pushLogs,
		exporterhelper.WithStart(oce.start),
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// explicitly disable since we rely on http.Client timeout logic.
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(oCfg.RetrySettings),
		exporterhelper.WithQueue(oCfg.QueueSettings))
}

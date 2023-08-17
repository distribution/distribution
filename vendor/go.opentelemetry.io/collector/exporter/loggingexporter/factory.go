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

package loggingexporter // import "go.opentelemetry.io/collector/exporter/loggingexporter"

import (
	"context"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	// The value of "type" key in configuration.
	typeStr                   = "logging"
	defaultSamplingInitial    = 2
	defaultSamplingThereafter = 500
)

// NewFactory creates a factory for Logging exporter
func NewFactory() component.ExporterFactory {
	return component.NewExporterFactory(
		typeStr,
		createDefaultConfig,
		component.WithTracesExporter(createTracesExporter, component.StabilityLevelInDevelopment),
		component.WithMetricsExporter(createMetricsExporter, component.StabilityLevelInDevelopment),
		component.WithLogsExporter(createLogsExporter, component.StabilityLevelInDevelopment),
	)
}

func createDefaultConfig() config.Exporter {
	return &Config{
		ExporterSettings:   config.NewExporterSettings(config.NewComponentID(typeStr)),
		LogLevel:           zapcore.InfoLevel,
		SamplingInitial:    defaultSamplingInitial,
		SamplingThereafter: defaultSamplingThereafter,
	}
}

func createTracesExporter(ctx context.Context, set component.ExporterCreateSettings, config config.Exporter) (component.TracesExporter, error) {
	cfg := config.(*Config)
	exporterLogger := createLogger(cfg, set.TelemetrySettings.Logger)
	s := newLoggingExporter(exporterLogger, cfg.LogLevel)
	return exporterhelper.NewTracesExporterWithContext(ctx, set, cfg,
		s.pushTraces,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout/RetryOnFailure and SendingQueue
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(exporterhelper.RetrySettings{Enabled: false}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithShutdown(loggerSync(exporterLogger)),
	)
}

func createMetricsExporter(ctx context.Context, set component.ExporterCreateSettings, config config.Exporter) (component.MetricsExporter, error) {
	cfg := config.(*Config)
	exporterLogger := createLogger(cfg, set.TelemetrySettings.Logger)
	s := newLoggingExporter(exporterLogger, cfg.LogLevel)
	return exporterhelper.NewMetricsExporterWithContext(ctx, set, cfg,
		s.pushMetrics,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout/RetryOnFailure and SendingQueue
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(exporterhelper.RetrySettings{Enabled: false}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithShutdown(loggerSync(exporterLogger)),
	)
}

func createLogsExporter(ctx context.Context, set component.ExporterCreateSettings, config config.Exporter) (component.LogsExporter, error) {
	cfg := config.(*Config)
	exporterLogger := createLogger(cfg, set.TelemetrySettings.Logger)
	s := newLoggingExporter(exporterLogger, cfg.LogLevel)
	return exporterhelper.NewLogsExporterWithContext(ctx, set, cfg,
		s.pushLogs,
		exporterhelper.WithCapabilities(consumer.Capabilities{MutatesData: false}),
		// Disable Timeout/RetryOnFailure and SendingQueue
		exporterhelper.WithTimeout(exporterhelper.TimeoutSettings{Timeout: 0}),
		exporterhelper.WithRetry(exporterhelper.RetrySettings{Enabled: false}),
		exporterhelper.WithQueue(exporterhelper.QueueSettings{Enabled: false}),
		exporterhelper.WithShutdown(loggerSync(exporterLogger)),
	)
}

func createLogger(cfg *Config, logger *zap.Logger) *zap.Logger {
	core := zapcore.NewSamplerWithOptions(
		logger.Core(),
		1*time.Second,
		cfg.SamplingInitial,
		cfg.SamplingThereafter,
	)

	return zap.New(core)
}

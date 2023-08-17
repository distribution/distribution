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

package service // import "go.opentelemetry.io/collector/service"

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/service/extensions"
	"go.opentelemetry.io/collector/service/internal"
	"go.opentelemetry.io/collector/service/internal/pipelines"
	"go.opentelemetry.io/collector/service/internal/telemetry"
	"go.opentelemetry.io/collector/service/internal/telemetrylogs"
)

// service represents the implementation of a component.Host.
type service struct {
	buildInfo            component.BuildInfo
	config               *Config
	telemetrySettings    component.TelemetrySettings
	host                 *serviceHost
	telemetryInitializer *telemetryInitializer
}

func newService(set *settings) (*service, error) {
	srv := &service{
		buildInfo: set.BuildInfo,
		config:    set.Config,
		telemetrySettings: component.TelemetrySettings{
			Logger: zap.NewNop(),
			TracerProvider: sdktrace.NewTracerProvider(
				// needed for supporting the zpages extension
				sdktrace.WithSampler(internal.AlwaysRecord()),
			),
			MeterProvider: metric.NewNoopMeterProvider(),
			MetricsLevel:  set.Config.Telemetry.Metrics.Level,
		},
		host: &serviceHost{
			factories:         set.Factories,
			buildInfo:         set.BuildInfo,
			asyncErrorChannel: set.AsyncErrorChannel,
		},
		telemetryInitializer: set.telemetry,
	}

	var err error
	if srv.telemetrySettings.Logger, err = telemetrylogs.NewLogger(set.Config.Service.Telemetry.Logs, set.LoggingOptions); err != nil {
		return nil, fmt.Errorf("failed to get logger: %w", err)
	}

	if err = srv.telemetryInitializer.init(set.BuildInfo, srv.telemetrySettings.Logger, set.Config.Service.Telemetry, set.AsyncErrorChannel); err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}
	srv.telemetrySettings.MeterProvider = srv.telemetryInitializer.mp

	extensionsSettings := extensions.Settings{
		Telemetry: srv.telemetrySettings,
		BuildInfo: srv.buildInfo,
		Configs:   srv.config.Extensions,
		Factories: srv.host.factories.Extensions,
	}
	if srv.host.extensions, err = extensions.New(context.Background(), extensionsSettings, srv.config.Service.Extensions); err != nil {
		return nil, fmt.Errorf("failed build extensions: %w", err)
	}

	pipelinesSettings := pipelines.Settings{
		Telemetry:          srv.telemetrySettings,
		BuildInfo:          srv.buildInfo,
		ReceiverFactories:  srv.host.factories.Receivers,
		ReceiverConfigs:    srv.config.Receivers,
		ProcessorFactories: srv.host.factories.Processors,
		ProcessorConfigs:   srv.config.Processors,
		ExporterFactories:  srv.host.factories.Exporters,
		ExporterConfigs:    srv.config.Exporters,
		PipelineConfigs:    srv.config.Service.Pipelines,
	}
	if srv.host.pipelines, err = pipelines.Build(context.Background(), pipelinesSettings); err != nil {
		return nil, fmt.Errorf("cannot build pipelines: %w", err)
	}

	if set.Config.Telemetry.Metrics.Level != configtelemetry.LevelNone && set.Config.Telemetry.Metrics.Address != "" {
		// The process telemetry initialization requires the ballast size, which is available after the extensions are initialized.
		if err = telemetry.RegisterProcessMetrics(srv.telemetryInitializer.ocRegistry, getBallastSize(srv.host)); err != nil {
			return nil, fmt.Errorf("failed to register process metrics: %w", err)
		}
	}

	return srv, nil
}

func (srv *service) Start(ctx context.Context) error {
	if err := srv.host.extensions.Start(ctx, srv.host); err != nil {
		return fmt.Errorf("failed to start extensions: %w", err)
	}

	if err := srv.host.pipelines.StartAll(ctx, srv.host); err != nil {
		return fmt.Errorf("cannot start pipelines: %w", err)
	}

	return srv.host.extensions.NotifyPipelineReady()
}

func (srv *service) Shutdown(ctx context.Context) error {
	// Accumulate errors and proceed with shutting down remaining components.
	var errs error

	if err := srv.host.extensions.NotifyPipelineNotReady(); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to notify that pipeline is not ready: %w", err))
	}

	if err := srv.host.pipelines.ShutdownAll(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown pipelines: %w", err))
	}

	if err := srv.host.extensions.Shutdown(ctx); err != nil {
		errs = multierr.Append(errs, fmt.Errorf("failed to shutdown extensions: %w", err))
	}

	// TODO: Shutdown TracerProvider, MeterProvider, and Sync Logger.
	return errs
}

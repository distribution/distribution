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
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
)

// settings holds configuration for building a new service.
type settings struct {
	// Factories component factories.
	Factories component.Factories

	// BuildInfo provides collector start information.
	BuildInfo component.BuildInfo

	// Config represents the configuration of the service.
	Config *Config

	// AsyncErrorChannel is the channel that is used to report fatal errors.
	AsyncErrorChannel chan error

	// LoggingOptions provides a way to change behavior of zap logging.
	LoggingOptions []zap.Option

	// For testing purpose only.
	telemetry *telemetryInitializer
}

// CollectorSettings holds configuration for creating a new Collector.
type CollectorSettings struct {
	// Factories component factories.
	Factories component.Factories

	// BuildInfo provides collector start information.
	BuildInfo component.BuildInfo

	// DisableGracefulShutdown disables the automatic graceful shutdown
	// of the collector on SIGINT or SIGTERM.
	// Users who want to handle signals themselves can disable this behavior
	// and manually handle the signals to shutdown the collector.
	DisableGracefulShutdown bool

	// ConfigProvider provides the service configuration.
	// If the provider watches for configuration change, collector may reload the new configuration upon changes.
	ConfigProvider ConfigProvider

	// LoggingOptions provides a way to change behavior of zap logging.
	LoggingOptions []zap.Option

	// SkipSettingGRPCLogger avoids setting the grpc logger
	SkipSettingGRPCLogger bool

	// For testing purpose only.
	telemetry *telemetryInitializer
}

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

package telemetrylogs // import "go.opentelemetry.io/collector/service/internal/telemetrylogs"

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
	"google.golang.org/grpc/grpclog"

	"go.opentelemetry.io/collector/service/telemetry"
)

func NewLogger(cfg telemetry.LogsConfig, options []zap.Option) (*zap.Logger, error) {
	// Copied from NewProductionConfig.
	zapCfg := &zap.Config{
		Level:       zap.NewAtomicLevelAt(cfg.Level),
		Development: cfg.Development,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:          cfg.Encoding,
		EncoderConfig:     zap.NewProductionEncoderConfig(),
		OutputPaths:       cfg.OutputPaths,
		ErrorOutputPaths:  cfg.ErrorOutputPaths,
		DisableCaller:     cfg.DisableCaller,
		DisableStacktrace: cfg.DisableStacktrace,
		InitialFields:     cfg.InitialFields,
	}

	if zapCfg.Encoding == "console" {
		// Human-readable timestamps for console format of logs.
		zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	logger, err := zapCfg.Build(options...)
	if err != nil {
		return nil, err
	}

	return logger, nil
}

// SetColGRPCLogger constructs a zapgrpc.Logger instance, and installs it as grpc logger, cloned from baseLogger with
// exact configuration. The minimum level of gRPC logs is set to WARN should the loglevel of the collector is set to
// INFO to avoid copious logging from grpc framework.
func SetColGRPCLogger(baseLogger *zap.Logger, loglevel zapcore.Level) *zapgrpc.Logger {
	logger := zapgrpc.NewLogger(baseLogger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		var c zapcore.Core
		var err error
		if loglevel == zapcore.InfoLevel {
			loglevel = zapcore.WarnLevel
		}
		// NewIncreaseLevelCore errors only if the new log level is less than the initial core level.
		c, err = zapcore.NewIncreaseLevelCore(core, loglevel)
		// In case of an error changing the level, move on, this happens when using the NopCore
		if err != nil {
			c = core
		}
		return c.With([]zapcore.Field{zap.Bool("grpc_log", true)})
	})))

	grpclog.SetLoggerV2(logger)
	return logger
}

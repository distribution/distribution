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

package zpagesextension // import "go.opentelemetry.io/collector/extension/zpagesextension"

import (
	"context"
	"errors"
	"net/http"
	"path"

	"go.opentelemetry.io/contrib/zpages"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
)

const (
	tracezPath = "tracez"
)

type zpagesExtension struct {
	config              *Config
	telemetry           component.TelemetrySettings
	zpagesSpanProcessor *zpages.SpanProcessor
	server              http.Server
	stopCh              chan struct{}
}

// registerableTracerProvider is a tracer that supports
// the SDK methods RegisterSpanProcessor and UnregisterSpanProcessor.
//
// We use an interface instead of casting to the SDK tracer type to support tracer providers
// that extend the SDK.
type registerableTracerProvider interface {
	// RegisterSpanProcessor adds the given SpanProcessor to the list of SpanProcessors.
	// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/trace#TracerProvider.RegisterSpanProcessor.
	RegisterSpanProcessor(SpanProcessor trace.SpanProcessor)

	// UnregisterSpanProcessor removes the given SpanProcessor from the list of SpanProcessors.
	// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/trace#TracerProvider.UnregisterSpanProcessor.
	UnregisterSpanProcessor(SpanProcessor trace.SpanProcessor)
}

func (zpe *zpagesExtension) Start(_ context.Context, host component.Host) error {
	zPagesMux := http.NewServeMux()

	sdktracer, ok := zpe.telemetry.TracerProvider.(registerableTracerProvider)
	if ok {
		sdktracer.RegisterSpanProcessor(zpe.zpagesSpanProcessor)
		zPagesMux.Handle(path.Join("/debug", tracezPath), zpages.NewTracezHandler(zpe.zpagesSpanProcessor))
		zpe.telemetry.Logger.Info("Registered zPages span processor on tracer provider")
	} else {
		zpe.telemetry.Logger.Warn("zPages span processor registration is not available")
	}

	hostZPages, ok := host.(interface {
		RegisterZPages(mux *http.ServeMux, pathPrefix string)
	})
	if ok {
		hostZPages.RegisterZPages(zPagesMux, "/debug")
		zpe.telemetry.Logger.Info("Registered Host's zPages")
	} else {
		zpe.telemetry.Logger.Warn("Host's zPages not available")
	}

	// Start the listener here so we can have earlier failure if port is
	// already in use.
	ln, err := zpe.config.TCPAddr.Listen()
	if err != nil {
		return err
	}

	zpe.telemetry.Logger.Info("Starting zPages extension", zap.Any("config", zpe.config))
	zpe.server = http.Server{Handler: zPagesMux}
	zpe.stopCh = make(chan struct{})
	go func() {
		defer close(zpe.stopCh)

		if errHTTP := zpe.server.Serve(ln); errHTTP != nil && !errors.Is(errHTTP, http.ErrServerClosed) {
			host.ReportFatalError(errHTTP)
		}
	}()

	return nil
}

func (zpe *zpagesExtension) Shutdown(context.Context) error {
	err := zpe.server.Close()
	if zpe.stopCh != nil {
		<-zpe.stopCh
	}

	sdktracer, ok := zpe.telemetry.TracerProvider.(registerableTracerProvider)
	if ok {
		sdktracer.UnregisterSpanProcessor(zpe.zpagesSpanProcessor)
		zpe.telemetry.Logger.Info("Unregistered zPages span processor on tracer provider")
	} else {
		zpe.telemetry.Logger.Warn("zPages span processor registration is not available")
	}

	return err
}

func newServer(config *Config, telemetry component.TelemetrySettings) *zpagesExtension {
	return &zpagesExtension{
		config:              config,
		telemetry:           telemetry,
		zpagesSpanProcessor: zpages.NewSpanProcessor(),
	}
}

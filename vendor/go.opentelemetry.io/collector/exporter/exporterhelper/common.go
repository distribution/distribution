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

package exporterhelper // import "go.opentelemetry.io/collector/exporter/exporterhelper"

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/exporter/exporterhelper/internal"
	"go.opentelemetry.io/collector/obsreport"
)

// TimeoutSettings for timeout. The timeout applies to individual attempts to send data to the backend.
type TimeoutSettings struct {
	// Timeout is the timeout for every attempt to send data to the backend.
	Timeout time.Duration `mapstructure:"timeout"`
}

// NewDefaultTimeoutSettings returns the default settings for TimeoutSettings.
func NewDefaultTimeoutSettings() TimeoutSettings {
	return TimeoutSettings{
		Timeout: 5 * time.Second,
	}
}

// requestSender is an abstraction of a sender for a request independent of the type of the data (traces, metrics, logs).
type requestSender interface {
	send(req internal.Request) error
}

// baseRequest is a base implementation for the internal.Request.
type baseRequest struct {
	ctx                        context.Context
	processingFinishedCallback func()
}

func (req *baseRequest) Context() context.Context {
	return req.ctx
}

func (req *baseRequest) SetContext(ctx context.Context) {
	req.ctx = ctx
}

func (req *baseRequest) SetOnProcessingFinished(callback func()) {
	req.processingFinishedCallback = callback
}

func (req *baseRequest) OnProcessingFinished() {
	if req.processingFinishedCallback != nil {
		req.processingFinishedCallback()
	}
}

// baseSettings represents all the options that users can configure.
type baseSettings struct {
	component.StartFunc
	component.ShutdownFunc
	consumerOptions []consumer.Option
	TimeoutSettings
	QueueSettings
	RetrySettings
}

// fromOptions returns the internal options starting from the default and applying all configured options.
func fromOptions(options ...Option) *baseSettings {
	// Start from the default options:
	opts := &baseSettings{
		TimeoutSettings: NewDefaultTimeoutSettings(),
		// TODO: Enable queuing by default (call DefaultQueueSettings)
		QueueSettings: QueueSettings{Enabled: false},
		// TODO: Enable retry by default (call DefaultRetrySettings)
		RetrySettings: RetrySettings{Enabled: false},
	}

	for _, op := range options {
		op(opts)
	}

	return opts
}

// Option apply changes to baseSettings.
type Option func(*baseSettings)

// WithStart overrides the default Start function for an exporter.
// The default start function does nothing and always returns nil.
func WithStart(start component.StartFunc) Option {
	return func(o *baseSettings) {
		o.StartFunc = start
	}
}

// WithShutdown overrides the default Shutdown function for an exporter.
// The default shutdown function does nothing and always returns nil.
func WithShutdown(shutdown component.ShutdownFunc) Option {
	return func(o *baseSettings) {
		o.ShutdownFunc = shutdown
	}
}

// WithTimeout overrides the default TimeoutSettings for an exporter.
// The default TimeoutSettings is 5 seconds.
func WithTimeout(timeoutSettings TimeoutSettings) Option {
	return func(o *baseSettings) {
		o.TimeoutSettings = timeoutSettings
	}
}

// WithRetry overrides the default RetrySettings for an exporter.
// The default RetrySettings is to disable retries.
func WithRetry(retrySettings RetrySettings) Option {
	return func(o *baseSettings) {
		o.RetrySettings = retrySettings
	}
}

// WithQueue overrides the default QueueSettings for an exporter.
// The default QueueSettings is to disable queueing.
func WithQueue(queueSettings QueueSettings) Option {
	return func(o *baseSettings) {
		o.QueueSettings = queueSettings
	}
}

// WithCapabilities overrides the default Capabilities() function for a Consumer.
// The default is non-mutable data.
// TODO: Verify if we can change the default to be mutable as we do for processors.
func WithCapabilities(capabilities consumer.Capabilities) Option {
	return func(o *baseSettings) {
		o.consumerOptions = append(o.consumerOptions, consumer.WithCapabilities(capabilities))
	}
}

// baseExporter contains common fields between different exporter types.
type baseExporter struct {
	component.StartFunc
	component.ShutdownFunc
	obsrep   *obsExporter
	sender   requestSender
	qrSender *queuedRetrySender
}

func newBaseExporter(cfg config.Exporter, set component.ExporterCreateSettings, bs *baseSettings, signal config.DataType, reqUnmarshaler internal.RequestUnmarshaler) *baseExporter {
	be := &baseExporter{}

	be.obsrep = newObsExporter(obsreport.ExporterSettings{ExporterID: cfg.ID(), ExporterCreateSettings: set}, globalInstruments)
	be.qrSender = newQueuedRetrySender(cfg.ID(), signal, bs.QueueSettings, bs.RetrySettings, reqUnmarshaler, &timeoutSender{cfg: bs.TimeoutSettings}, set.Logger)
	be.sender = be.qrSender
	be.StartFunc = func(ctx context.Context, host component.Host) error {
		// First start the wrapped exporter.
		if err := bs.StartFunc.Start(ctx, host); err != nil {
			return err
		}

		// If no error then start the queuedRetrySender.
		return be.qrSender.start(ctx, host)
	}
	be.ShutdownFunc = func(ctx context.Context) error {
		// First shutdown the queued retry sender
		be.qrSender.shutdown()
		// Last shutdown the wrapped exporter itself.
		return bs.ShutdownFunc.Shutdown(ctx)
	}
	return be
}

// wrapConsumerSender wraps the consumer sender (the sender that uses retries and timeout) with the given wrapper.
// This can be used to wrap with observability (create spans, record metrics) the consumer sender.
func (be *baseExporter) wrapConsumerSender(f func(consumer requestSender) requestSender) {
	be.qrSender.consumerSender = f(be.qrSender.consumerSender)
}

// timeoutSender is a requestSender that adds a `timeout` to every request that passes this sender.
type timeoutSender struct {
	cfg TimeoutSettings
}

func (ts *timeoutSender) send(req internal.Request) error {
	// Intentionally don't overwrite the context inside the request, because in case of retries deadline will not be
	// updated because this deadline most likely is before the next one.
	ctx := req.Context()
	if ts.cfg.Timeout > 0 {
		var cancelFunc func()
		ctx, cancelFunc = context.WithTimeout(req.Context(), ts.cfg.Timeout)
		defer cancelFunc()
	}
	return req.Export(ctx)
}

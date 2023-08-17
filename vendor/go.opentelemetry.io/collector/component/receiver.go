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

package component // import "go.opentelemetry.io/collector/component"

import (
	"context"

	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
)

// Receiver allows the collector to receive metrics, traces and logs.
//
// Receiver receives data from a source (either from a remote source via network
// or scrapes from a local host) and pushes the data to the pipelines it is attached
// to by calling the nextConsumer.Consume*() function.
//
// # Error Handling
//
// The nextConsumer.Consume*() function may return an error to indicate that the data
// was not accepted. There are 2 types of possible errors: Permanent and non-Permanent.
// The receiver must check the type of the error using IsPermanent() helper.
//
// If the error is Permanent, then the nextConsumer.Consume*() call should not be
// retried with the same data. This typically happens when the data cannot be
// serialized by the exporter that is attached to the pipeline or when the destination
// refuses the data because it cannot decode it. The receiver must indicate to
// the source from which it received the data that the received data was bad, if the
// receiving protocol allows to do that. In case of OTLP/HTTP for example, this means
// that HTTP 400 response is returned to the sender.
//
// If the error is non-Permanent then the nextConsumer.Consume*() call may be retried
// with the same data. This may be done by the receiver itself, however typically it is
// done by the original sender, after the receiver returns a response to the sender
// indicating that the Collector is currently overloaded and the request must be
// retried. In case of OTLP/HTTP for example, this means that HTTP 429 or 503 response
// is returned.
//
// # Acknowledgment Handling
//
// The receivers that receive data via a network protocol that support acknowledgments
// MUST follow this order of operations:
//   - Receive data from some sender (typically from a network).
//   - Push received data to the pipeline by calling nextConsumer.Consume*() function.
//   - Acknowledge successful data receipt to the sender if Consume*() succeeded or
//     return a failure to the sender if Consume*() returned an error.
//
// This ensures there are strong delivery guarantees once the data is acknowledged
// by the Collector.
type Receiver interface {
	Component
}

// A TracesReceiver receives traces.
// Its purpose is to translate data from any format to the collector's internal trace format.
// TracesReceiver feeds a consumer.Traces with data.
//
// For example it could be Zipkin data source which translates Zipkin spans into ptrace.Traces.
type TracesReceiver interface {
	Receiver
}

// A MetricsReceiver receives metrics.
// Its purpose is to translate data from any format to the collector's internal metrics format.
// MetricsReceiver feeds a consumer.Metrics with data.
//
// For example it could be Prometheus data source which translates Prometheus metrics into pmetric.Metrics.
type MetricsReceiver interface {
	Receiver
}

// A LogsReceiver receives logs.
// Its purpose is to translate data from any format to the collector's internal logs data format.
// LogsReceiver feeds a consumer.Logs with data.
//
// For example a LogsReceiver can read syslogs and convert them into plog.Logs.
type LogsReceiver interface {
	Receiver
}

// ReceiverCreateSettings configures Receiver creators.
type ReceiverCreateSettings struct {
	TelemetrySettings

	// BuildInfo can be used by components for informational purposes.
	BuildInfo BuildInfo
}

// ReceiverFactory is factory interface for receivers.
//
// This interface cannot be directly implemented. Implementations must
// use the NewReceiverFactory to implement it.
type ReceiverFactory interface {
	Factory

	// CreateDefaultConfig creates the default configuration for the Receiver.
	// This method can be called multiple times depending on the pipeline
	// configuration and should not cause side-effects that prevent the creation
	// of multiple instances of the Receiver.
	// The object returned by this method needs to pass the checks implemented by
	// 'configtest.CheckConfigStruct'. It is recommended to have these checks in the
	// tests of any implementation of the Factory interface.
	CreateDefaultConfig() config.Receiver

	// CreateTracesReceiver creates a TracesReceiver based on this config.
	// If the receiver type does not support tracing or if the config is not valid
	// an error will be returned instead.
	CreateTracesReceiver(ctx context.Context, set ReceiverCreateSettings, cfg config.Receiver, nextConsumer consumer.Traces) (TracesReceiver, error)

	// TracesReceiverStability gets the stability level of the TracesReceiver.
	TracesReceiverStability() StabilityLevel

	// CreateMetricsReceiver creates a MetricsReceiver based on this config.
	// If the receiver type does not support metrics or if the config is not valid
	// an error will be returned instead.
	CreateMetricsReceiver(ctx context.Context, set ReceiverCreateSettings, cfg config.Receiver, nextConsumer consumer.Metrics) (MetricsReceiver, error)

	// MetricsReceiverStability gets the stability level of the MetricsReceiver.
	MetricsReceiverStability() StabilityLevel

	// CreateLogsReceiver creates a LogsReceiver based on this config.
	// If the receiver type does not support the data type or if the config is not valid
	// an error will be returned instead.
	CreateLogsReceiver(ctx context.Context, set ReceiverCreateSettings, cfg config.Receiver, nextConsumer consumer.Logs) (LogsReceiver, error)

	// LogsReceiverStability gets the stability level of the LogsReceiver.
	LogsReceiverStability() StabilityLevel
}

// ReceiverFactoryOption apply changes to ReceiverOptions.
type ReceiverFactoryOption interface {
	// applyReceiverFactoryOption applies the option.
	applyReceiverFactoryOption(o *receiverFactory)
}

var _ ReceiverFactoryOption = (*receiverFactoryOptionFunc)(nil)

// receiverFactoryOptionFunc is an ReceiverFactoryOption created through a function.
type receiverFactoryOptionFunc func(*receiverFactory)

func (f receiverFactoryOptionFunc) applyReceiverFactoryOption(o *receiverFactory) {
	f(o)
}

// ReceiverCreateDefaultConfigFunc is the equivalent of ReceiverFactory.CreateDefaultConfig().
type ReceiverCreateDefaultConfigFunc func() config.Receiver

// CreateDefaultConfig implements ReceiverFactory.CreateDefaultConfig().
func (f ReceiverCreateDefaultConfigFunc) CreateDefaultConfig() config.Receiver {
	return f()
}

// CreateTracesReceiverFunc is the equivalent of ReceiverFactory.CreateTracesReceiver().
type CreateTracesReceiverFunc func(context.Context, ReceiverCreateSettings, config.Receiver, consumer.Traces) (TracesReceiver, error)

// CreateTracesReceiver implements ReceiverFactory.CreateTracesReceiver().
func (f CreateTracesReceiverFunc) CreateTracesReceiver(
	ctx context.Context,
	set ReceiverCreateSettings,
	cfg config.Receiver,
	nextConsumer consumer.Traces) (TracesReceiver, error) {
	if f == nil {
		return nil, ErrDataTypeIsNotSupported
	}
	return f(ctx, set, cfg, nextConsumer)
}

// CreateMetricsReceiverFunc is the equivalent of ReceiverFactory.CreateMetricsReceiver().
type CreateMetricsReceiverFunc func(context.Context, ReceiverCreateSettings, config.Receiver, consumer.Metrics) (MetricsReceiver, error)

// CreateMetricsReceiver implements ReceiverFactory.CreateMetricsReceiver().
func (f CreateMetricsReceiverFunc) CreateMetricsReceiver(
	ctx context.Context,
	set ReceiverCreateSettings,
	cfg config.Receiver,
	nextConsumer consumer.Metrics,
) (MetricsReceiver, error) {
	if f == nil {
		return nil, ErrDataTypeIsNotSupported
	}
	return f(ctx, set, cfg, nextConsumer)
}

// CreateLogsReceiverFunc is the equivalent of ReceiverFactory.CreateLogsReceiver().
type CreateLogsReceiverFunc func(context.Context, ReceiverCreateSettings, config.Receiver, consumer.Logs) (LogsReceiver, error)

// CreateLogsReceiver implements ReceiverFactory.CreateLogsReceiver().
func (f CreateLogsReceiverFunc) CreateLogsReceiver(
	ctx context.Context,
	set ReceiverCreateSettings,
	cfg config.Receiver,
	nextConsumer consumer.Logs,
) (LogsReceiver, error) {
	if f == nil {
		return nil, ErrDataTypeIsNotSupported
	}
	return f(ctx, set, cfg, nextConsumer)
}

type receiverFactory struct {
	baseFactory
	ReceiverCreateDefaultConfigFunc
	CreateTracesReceiverFunc
	CreateMetricsReceiverFunc
	CreateLogsReceiverFunc
}

func (r receiverFactory) TracesReceiverStability() StabilityLevel {
	return r.getStabilityLevel(config.TracesDataType)
}

func (r receiverFactory) MetricsReceiverStability() StabilityLevel {
	return r.getStabilityLevel(config.MetricsDataType)
}

func (r receiverFactory) LogsReceiverStability() StabilityLevel {
	return r.getStabilityLevel(config.LogsDataType)
}

// WithTracesReceiver overrides the default "error not supported" implementation for CreateTracesReceiver and the default "undefined" stability level.
func WithTracesReceiver(createTracesReceiver CreateTracesReceiverFunc, sl StabilityLevel) ReceiverFactoryOption {
	return receiverFactoryOptionFunc(func(o *receiverFactory) {
		o.stability[config.TracesDataType] = sl
		o.CreateTracesReceiverFunc = createTracesReceiver
	})
}

// WithMetricsReceiver overrides the default "error not supported" implementation for CreateMetricsReceiver and the default "undefined" stability level.
func WithMetricsReceiver(createMetricsReceiver CreateMetricsReceiverFunc, sl StabilityLevel) ReceiverFactoryOption {
	return receiverFactoryOptionFunc(func(o *receiverFactory) {
		o.stability[config.MetricsDataType] = sl
		o.CreateMetricsReceiverFunc = createMetricsReceiver
	})
}

// WithLogsReceiver overrides the default "error not supported" implementation for CreateLogsReceiver and the default "undefined" stability level.
func WithLogsReceiver(createLogsReceiver CreateLogsReceiverFunc, sl StabilityLevel) ReceiverFactoryOption {
	return receiverFactoryOptionFunc(func(o *receiverFactory) {
		o.stability[config.LogsDataType] = sl
		o.CreateLogsReceiverFunc = createLogsReceiver
	})
}

// NewReceiverFactory returns a ReceiverFactory.
func NewReceiverFactory(cfgType config.Type, createDefaultConfig ReceiverCreateDefaultConfigFunc, options ...ReceiverFactoryOption) ReceiverFactory {
	f := &receiverFactory{
		baseFactory:                     baseFactory{cfgType: cfgType, stability: make(map[config.DataType]StabilityLevel)},
		ReceiverCreateDefaultConfigFunc: createDefaultConfig,
	}
	for _, opt := range options {
		opt.applyReceiverFactoryOption(f)
	}
	return f
}

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

package processorhelper // import "go.opentelemetry.io/collector/processor/processorhelper"

import (
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/internal/obsreportconfig/obsmetrics"
)

// ErrSkipProcessingData is a sentinel value to indicate when traces or metrics should intentionally be dropped
// from further processing in the pipeline because the data is determined to be irrelevant. A processor can return this error
// to stop further processing without propagating an error back up the pipeline to logs.
var ErrSkipProcessingData = errors.New("sentinel error to skip processing data from the remainder of the pipeline")

// Option apply changes to internalOptions.
type Option func(*baseSettings)

// WithStart overrides the default Start function for an processor.
// The default shutdown function does nothing and always returns nil.
func WithStart(start component.StartFunc) Option {
	return func(o *baseSettings) {
		o.StartFunc = start
	}
}

// WithShutdown overrides the default Shutdown function for an processor.
// The default shutdown function does nothing and always returns nil.
func WithShutdown(shutdown component.ShutdownFunc) Option {
	return func(o *baseSettings) {
		o.ShutdownFunc = shutdown
	}
}

// WithCapabilities overrides the default GetCapabilities function for an processor.
// The default GetCapabilities function returns mutable capabilities.
func WithCapabilities(capabilities consumer.Capabilities) Option {
	return func(o *baseSettings) {
		o.consumerOptions = append(o.consumerOptions, consumer.WithCapabilities(capabilities))
	}
}

type baseSettings struct {
	component.StartFunc
	component.ShutdownFunc
	consumerOptions []consumer.Option
}

// fromOptions returns the internal settings starting from the default and applying all options.
func fromOptions(options []Option) *baseSettings {
	// Start from the default options:
	opts := &baseSettings{
		consumerOptions: []consumer.Option{consumer.WithCapabilities(consumer.Capabilities{MutatesData: true})},
	}

	for _, op := range options {
		op(opts)
	}

	return opts
}

func spanAttributes(id config.ComponentID) trace.EventOption {
	return trace.WithAttributes(attribute.String(obsmetrics.ProcessorKey, id.String()))
}

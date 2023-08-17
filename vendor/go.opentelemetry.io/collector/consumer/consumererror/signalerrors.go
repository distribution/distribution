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

package consumererror // import "go.opentelemetry.io/collector/consumer/consumererror"

import (
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Traces is an error that may carry associated Trace data for a subset of received data
// that failed to be processed or sent.
type Traces struct {
	error
	failed ptrace.Traces
}

// NewTraces creates a Traces that can encapsulate received data that failed to be processed or sent.
func NewTraces(err error, failed ptrace.Traces) error {
	return Traces{
		error:  err,
		failed: failed,
	}
}

// GetTraces returns failed traces from the associated error.
func (err Traces) GetTraces() ptrace.Traces {
	return err.failed
}

// Unwrap returns the wrapped error for functions Is and As in standard package errors.
func (err Traces) Unwrap() error {
	return err.error
}

// Logs is an error that may carry associated Log data for a subset of received data
// that failed to be processed or sent.
type Logs struct {
	error
	failed plog.Logs
}

// NewLogs creates a Logs that can encapsulate received data that failed to be processed or sent.
func NewLogs(err error, failed plog.Logs) error {
	return Logs{
		error:  err,
		failed: failed,
	}
}

// GetLogs returns failed logs from the associated error.
func (err Logs) GetLogs() plog.Logs {
	return err.failed
}

// Unwrap returns the wrapped error for functions Is and As in standard package errors.
func (err Logs) Unwrap() error {
	return err.error
}

// Metrics is an error that may carry associated Metrics data for a subset of received data
// that failed to be processed or sent.
type Metrics struct {
	error
	failed pmetric.Metrics
}

// NewMetrics creates a Metrics that can encapsulate received data that failed to be processed or sent.
func NewMetrics(err error, failed pmetric.Metrics) error {
	return Metrics{
		error:  err,
		failed: failed,
	}
}

// GetMetrics returns failed metrics from the associated error.
func (err Metrics) GetMetrics() pmetric.Metrics {
	return err.failed
}

// Unwrap returns the wrapped error for functions Is and As in standard package errors.
func (err Metrics) Unwrap() error {
	return err.error
}

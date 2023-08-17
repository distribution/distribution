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

package fanoutconsumer // import "go.opentelemetry.io/collector/service/internal/fanoutconsumer"

import (
	"context"

	"go.uber.org/multierr"

	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// NewTraces wraps multiple trace consumers in a single one.
// It fanouts the incoming data to all the consumers, and does smart routing:
//   - Clones only to the consumer that needs to mutate the data.
//   - If all consumers needs to mutate the data one will get the original data.
func NewTraces(tcs []consumer.Traces) consumer.Traces {
	if len(tcs) == 1 {
		// Don't wrap if no need to do it.
		return tcs[0]
	}
	var pass []consumer.Traces
	var clone []consumer.Traces
	for i := 0; i < len(tcs)-1; i++ {
		if !tcs[i].Capabilities().MutatesData {
			pass = append(pass, tcs[i])
		} else {
			clone = append(clone, tcs[i])
		}
	}
	// Give the original data to the last consumer if no other read-only consumer,
	// otherwise put it in the right bucket. Never share the same data between
	// a mutating and a non-mutating consumer since the non-mutating consumer may process
	// data async and the mutating consumer may change the data before that.
	if len(pass) == 0 || !tcs[len(tcs)-1].Capabilities().MutatesData {
		pass = append(pass, tcs[len(tcs)-1])
	} else {
		clone = append(clone, tcs[len(tcs)-1])
	}
	return &tracesConsumer{pass: pass, clone: clone}
}

type tracesConsumer struct {
	pass  []consumer.Traces
	clone []consumer.Traces
}

func (tsc *tracesConsumer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// ConsumeTraces exports the ptrace.Traces to all consumers wrapped by the current one.
func (tsc *tracesConsumer) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	var errs error
	// Initially pass to clone exporter to avoid the case where the optimization of sending
	// the incoming data to a mutating consumer is used that may change the incoming data before
	// cloning.
	for _, tc := range tsc.clone {
		errs = multierr.Append(errs, tc.ConsumeTraces(ctx, td.Clone()))
	}
	for _, tc := range tsc.pass {
		errs = multierr.Append(errs, tc.ConsumeTraces(ctx, td))
	}
	return errs
}

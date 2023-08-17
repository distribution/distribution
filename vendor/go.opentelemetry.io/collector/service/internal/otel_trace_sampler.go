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

package internal // import "go.opentelemetry.io/collector/service/internal"

import (
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type recordSampler struct{}

func (r recordSampler) ShouldSample(parameters sdktrace.SamplingParameters) sdktrace.SamplingResult {
	return sdktrace.SamplingResult{Decision: sdktrace.RecordOnly}
}

func (r recordSampler) Description() string {
	return "Always record sampler"
}

func AlwaysRecord() sdktrace.Sampler {
	rs := &recordSampler{}
	return sdktrace.ParentBased(
		rs,
		sdktrace.WithRemoteParentSampled(sdktrace.AlwaysSample()),
		sdktrace.WithRemoteParentNotSampled(rs),
		sdktrace.WithLocalParentSampled(sdktrace.AlwaysSample()),
		sdktrace.WithRemoteParentSampled(rs))
}

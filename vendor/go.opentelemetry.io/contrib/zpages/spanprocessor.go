// Copyright The OpenTelemetry Authors
// Copyright 2017, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package zpages // import "go.opentelemetry.io/contrib/zpages"

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var _ sdktrace.SpanProcessor = (*SpanProcessor)(nil)

// perMethodSummary is a summary of the spans stored for a single span name.
type perMethodSummary struct {
	activeSpans  int
	latencySpans []int
	errorSpans   int
}

// SpanProcessor is an sdktrace.SpanProcessor implementation that exposes zpages functionality for opentelemetry-go.
//
// It tracks all active spans, and stores samples of spans based on latency for non errored spans,
// and samples for errored spans.
type SpanProcessor struct {
	// Cannot keep track of the active Spans per name because the Span interface,
	// allows the name to be changed, and that will leak memory.
	activeSpansStore sync.Map
	spanSampleStores sync.Map
}

// NewSpanProcessor returns a new SpanProcessor.
func NewSpanProcessor() *SpanProcessor {
	return &SpanProcessor{}
}

// OnStart adds span as active and reports it with zpages.
func (ssm *SpanProcessor) OnStart(_ context.Context, span sdktrace.ReadWriteSpan) {
	sc := span.SpanContext()
	if sc.IsValid() {
		ssm.activeSpansStore.Store(spanKey(sc), span)
	}
}

// OnEnd processes all spans and reports them with zpages.
func (ssm *SpanProcessor) OnEnd(span sdktrace.ReadOnlySpan) {
	sc := span.SpanContext()
	if sc.IsValid() {
		ssm.activeSpansStore.Delete(spanKey(sc))
	}

	name := span.Name()
	value, ok := ssm.spanSampleStores.Load(name)
	if !ok {
		value, _ = ssm.spanSampleStores.LoadOrStore(name, newSampleStore(defaultBucketCapacity, defaultBucketCapacity))
	}
	value.(*sampleStore).sampleSpan(span)
}

// Shutdown does nothing.
func (ssm *SpanProcessor) Shutdown(context.Context) error {
	// Do nothing
	return nil
}

// ForceFlush does nothing.
func (ssm *SpanProcessor) ForceFlush(context.Context) error {
	// Do nothing
	return nil
}

// spanStoreForName returns the sampleStore for the given name.
//
// It returns nil if it doesn't exist.
func (ssm *SpanProcessor) spanStoreForName(name string) *sampleStore {
	if value, ok := ssm.spanSampleStores.Load(name); ok {
		return value.(*sampleStore)
	}
	return nil
}

// spansPerMethod returns a summary of what spans are being stored for each span name.
func (ssm *SpanProcessor) spansPerMethod() map[string]*perMethodSummary {
	out := make(map[string]*perMethodSummary)
	ssm.spanSampleStores.Range(func(name, s interface{}) bool {
		out[name.(string)] = s.(*sampleStore).perMethodSummary()
		return true
	})
	ssm.activeSpansStore.Range(func(_, sp interface{}) bool {
		span := sp.(sdktrace.ReadOnlySpan)
		if pms, ok := out[span.Name()]; ok {
			pms.activeSpans++
			return true
		}
		out[span.Name()] = &perMethodSummary{activeSpans: 1}
		return true
	})
	return out
}

// activeSpans returns the active spans for the given name.
func (ssm *SpanProcessor) activeSpans(name string) []sdktrace.ReadOnlySpan {
	var out []sdktrace.ReadOnlySpan
	ssm.activeSpansStore.Range(func(_, sp interface{}) bool {
		span := sp.(sdktrace.ReadOnlySpan)
		if span.Name() == name {
			out = append(out, span)
		}
		return true
	})
	return out
}

// errorSpans returns a sample of error spans.
func (ssm *SpanProcessor) errorSpans(name string) []sdktrace.ReadOnlySpan {
	s := ssm.spanStoreForName(name)
	if s == nil {
		return nil
	}
	return s.errorSpans()
}

// spansByLatency returns a sample of successful spans.
//
// minLatency is the minimum latency of spans to be returned.
// maxDuration, if nonzero, is the maximum latency of spans to be returned.
func (ssm *SpanProcessor) spansByLatency(name string, latencyBucketIndex int) []sdktrace.ReadOnlySpan {
	s := ssm.spanStoreForName(name)
	if s == nil {
		return nil
	}
	return s.spansByLatency(latencyBucketIndex)
}

// sampleStore stores a sampled of spans for a particular span name.
//
// It contains sample of spans for error requests (status code is codes.Error);
// and a sample of spans for successful requests, bucketed by latency.
type sampleStore struct {
	sync.Mutex // protects everything below.
	latency    []*bucket
	errors     *bucket
}

// newSampleStore creates a sampleStore.
func newSampleStore(latencyBucketSize uint, errorBucketSize uint) *sampleStore {
	s := &sampleStore{
		latency: make([]*bucket, defaultBoundaries.numBuckets()),
		errors:  newBucket(errorBucketSize),
	}
	for i := range s.latency {
		s.latency[i] = newBucket(latencyBucketSize)
	}
	return s
}

func (ss *sampleStore) perMethodSummary() *perMethodSummary {
	ss.Lock()
	defer ss.Unlock()
	p := &perMethodSummary{}
	p.errorSpans = ss.errors.len()
	for _, b := range ss.latency {
		p.latencySpans = append(p.latencySpans, b.len())
	}
	return p
}

func (ss *sampleStore) spansByLatency(latencyBucketIndex int) []sdktrace.ReadOnlySpan {
	ss.Lock()
	defer ss.Unlock()
	if latencyBucketIndex < 0 || latencyBucketIndex >= len(ss.latency) {
		return nil
	}
	return ss.latency[latencyBucketIndex].spans()
}

func (ss *sampleStore) errorSpans() []sdktrace.ReadOnlySpan {
	ss.Lock()
	defer ss.Unlock()
	return ss.errors.spans()
}

// sampleSpan removes adds to the corresponding latency or error bucket.
func (ss *sampleStore) sampleSpan(span sdktrace.ReadOnlySpan) {
	code := span.Status().Code

	ss.Lock()
	defer ss.Unlock()
	if code == codes.Error {
		ss.errors.add(span)
		return
	}

	latency := span.EndTime().Sub(span.StartTime())
	// In case of time skew or wrong time, sample as 0 latency.
	if latency < 0 {
		latency = 0
	}
	ss.latency[defaultBoundaries.getBucketIndex(latency)].add(span)
}

func spanKey(sc trace.SpanContext) [24]byte {
	var sk [24]byte
	tid := sc.TraceID()
	copy(sk[0:16], tid[:])
	sid := sc.SpanID()
	copy(sk[16:24], sid[:])
	return sk
}

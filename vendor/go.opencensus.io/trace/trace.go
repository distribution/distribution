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

package trace

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"go.opencensus.io/internal"
	"go.opencensus.io/trace/tracestate"
)

<<<<<<< HEAD
type tracer struct{}

var _ Tracer = &tracer{}

=======
>>>>>>> main
// Span represents a span of a trace.  It has an associated SpanContext, and
// stores data accumulated while the span is active.
//
// Ideally users should interact with Spans by calling the functions in this
// package that take a Context parameter.
<<<<<<< HEAD
type span struct {
=======
type Span struct {
>>>>>>> main
	// data contains information recorded about the span.
	//
	// It will be non-nil if we are exporting the span or recording events for it.
	// Otherwise, data is nil, and the Span is simply a carrier for the
	// SpanContext, so that the trace ID is propagated.
	data        *SpanData
	mu          sync.Mutex // protects the contents of *data (but not the pointer value.)
	spanContext SpanContext

	// lruAttributes are capped at configured limit. When the capacity is reached an oldest entry
	// is removed to create room for a new entry.
	lruAttributes *lruMap

	// annotations are stored in FIFO queue capped by configured limit.
	annotations *evictedQueue

	// messageEvents are stored in FIFO queue capped by configured limit.
	messageEvents *evictedQueue

	// links are stored in FIFO queue capped by configured limit.
	links *evictedQueue

	// spanStore is the spanStore this span belongs to, if any, otherwise it is nil.
	*spanStore
	endOnce sync.Once

	executionTracerTaskEnd func() // ends the execution tracer span
}

// IsRecordingEvents returns true if events are being recorded for this span.
// Use this check to avoid computing expensive annotations when they will never
// be used.
<<<<<<< HEAD
func (s *span) IsRecordingEvents() bool {
=======
func (s *Span) IsRecordingEvents() bool {
>>>>>>> main
	if s == nil {
		return false
	}
	return s.data != nil
}

// TraceOptions contains options associated with a trace span.
type TraceOptions uint32

// IsSampled returns true if the span will be exported.
func (sc SpanContext) IsSampled() bool {
	return sc.TraceOptions.IsSampled()
}

// setIsSampled sets the TraceOptions bit that determines whether the span will be exported.
func (sc *SpanContext) setIsSampled(sampled bool) {
	if sampled {
		sc.TraceOptions |= 1
	} else {
		sc.TraceOptions &= ^TraceOptions(1)
	}
}

// IsSampled returns true if the span will be exported.
func (t TraceOptions) IsSampled() bool {
	return t&1 == 1
}

// SpanContext contains the state that must propagate across process boundaries.
//
// SpanContext is not an implementation of context.Context.
// TODO: add reference to external Census docs for SpanContext.
type SpanContext struct {
	TraceID      TraceID
	SpanID       SpanID
	TraceOptions TraceOptions
	Tracestate   *tracestate.Tracestate
}

type contextKey struct{}

// FromContext returns the Span stored in a context, or nil if there isn't one.
<<<<<<< HEAD
func (t *tracer) FromContext(ctx context.Context) *Span {
=======
func FromContext(ctx context.Context) *Span {
>>>>>>> main
	s, _ := ctx.Value(contextKey{}).(*Span)
	return s
}

// NewContext returns a new context with the given Span attached.
<<<<<<< HEAD
func (t *tracer) NewContext(parent context.Context, s *Span) context.Context {
=======
func NewContext(parent context.Context, s *Span) context.Context {
>>>>>>> main
	return context.WithValue(parent, contextKey{}, s)
}

// All available span kinds. Span kind must be either one of these values.
const (
	SpanKindUnspecified = iota
	SpanKindServer
	SpanKindClient
)

// StartOptions contains options concerning how a span is started.
type StartOptions struct {
	// Sampler to consult for this Span. If provided, it is always consulted.
	//
	// If not provided, then the behavior differs based on whether
	// the parent of this Span is remote, local, or there is no parent.
	// In the case of a remote parent or no parent, the
	// default sampler (see Config) will be consulted. Otherwise,
	// when there is a non-remote parent, no new sampling decision will be made:
	// we will preserve the sampling of the parent.
	Sampler Sampler

	// SpanKind represents the kind of a span. If none is set,
	// SpanKindUnspecified is used.
	SpanKind int
}

// StartOption apply changes to StartOptions.
type StartOption func(*StartOptions)

// WithSpanKind makes new spans to be created with the given kind.
func WithSpanKind(spanKind int) StartOption {
	return func(o *StartOptions) {
		o.SpanKind = spanKind
	}
}

// WithSampler makes new spans to be be created with a custom sampler.
// Otherwise, the global sampler is used.
func WithSampler(sampler Sampler) StartOption {
	return func(o *StartOptions) {
		o.Sampler = sampler
	}
}

// StartSpan starts a new child span of the current span in the context. If
// there is no span in the context, creates a new trace and span.
//
// Returned context contains the newly created span. You can use it to
// propagate the returned span in process.
<<<<<<< HEAD
func (t *tracer) StartSpan(ctx context.Context, name string, o ...StartOption) (context.Context, *Span) {
	var opts StartOptions
	var parent SpanContext
	if p := t.FromContext(ctx); p != nil {
		if ps, ok := p.internal.(*span); ok {
			ps.addChild()
		}
		parent = p.SpanContext()
=======
func StartSpan(ctx context.Context, name string, o ...StartOption) (context.Context, *Span) {
	var opts StartOptions
	var parent SpanContext
	if p := FromContext(ctx); p != nil {
		p.addChild()
		parent = p.spanContext
>>>>>>> main
	}
	for _, op := range o {
		op(&opts)
	}
	span := startSpanInternal(name, parent != SpanContext{}, parent, false, opts)

	ctx, end := startExecutionTracerTask(ctx, name)
	span.executionTracerTaskEnd = end
<<<<<<< HEAD
	extSpan := NewSpan(span)
	return t.NewContext(ctx, extSpan), extSpan
=======
	return NewContext(ctx, span), span
>>>>>>> main
}

// StartSpanWithRemoteParent starts a new child span of the span from the given parent.
//
// If the incoming context contains a parent, it ignores. StartSpanWithRemoteParent is
// preferred for cases where the parent is propagated via an incoming request.
//
// Returned context contains the newly created span. You can use it to
// propagate the returned span in process.
<<<<<<< HEAD
func (t *tracer) StartSpanWithRemoteParent(ctx context.Context, name string, parent SpanContext, o ...StartOption) (context.Context, *Span) {
=======
func StartSpanWithRemoteParent(ctx context.Context, name string, parent SpanContext, o ...StartOption) (context.Context, *Span) {
>>>>>>> main
	var opts StartOptions
	for _, op := range o {
		op(&opts)
	}
	span := startSpanInternal(name, parent != SpanContext{}, parent, true, opts)
	ctx, end := startExecutionTracerTask(ctx, name)
	span.executionTracerTaskEnd = end
<<<<<<< HEAD
	extSpan := NewSpan(span)
	return t.NewContext(ctx, extSpan), extSpan
}

func startSpanInternal(name string, hasParent bool, parent SpanContext, remoteParent bool, o StartOptions) *span {
	s := &span{}
	s.spanContext = parent

	cfg := config.Load().(*Config)
	if gen, ok := cfg.IDGenerator.(*defaultIDGenerator); ok {
		// lazy initialization
		gen.init()
	}

	if !hasParent {
		s.spanContext.TraceID = cfg.IDGenerator.NewTraceID()
	}
	s.spanContext.SpanID = cfg.IDGenerator.NewSpanID()
=======
	return NewContext(ctx, span), span
}

func startSpanInternal(name string, hasParent bool, parent SpanContext, remoteParent bool, o StartOptions) *Span {
	span := &Span{}
	span.spanContext = parent

	cfg := config.Load().(*Config)

	if !hasParent {
		span.spanContext.TraceID = cfg.IDGenerator.NewTraceID()
	}
	span.spanContext.SpanID = cfg.IDGenerator.NewSpanID()
>>>>>>> main
	sampler := cfg.DefaultSampler

	if !hasParent || remoteParent || o.Sampler != nil {
		// If this span is the child of a local span and no Sampler is set in the
		// options, keep the parent's TraceOptions.
		//
		// Otherwise, consult the Sampler in the options if it is non-nil, otherwise
		// the default sampler.
		if o.Sampler != nil {
			sampler = o.Sampler
		}
<<<<<<< HEAD
		s.spanContext.setIsSampled(sampler(SamplingParameters{
			ParentContext:   parent,
			TraceID:         s.spanContext.TraceID,
			SpanID:          s.spanContext.SpanID,
=======
		span.spanContext.setIsSampled(sampler(SamplingParameters{
			ParentContext:   parent,
			TraceID:         span.spanContext.TraceID,
			SpanID:          span.spanContext.SpanID,
>>>>>>> main
			Name:            name,
			HasRemoteParent: remoteParent}).Sample)
	}

<<<<<<< HEAD
	if !internal.LocalSpanStoreEnabled && !s.spanContext.IsSampled() {
		return s
	}

	s.data = &SpanData{
		SpanContext:     s.spanContext,
=======
	if !internal.LocalSpanStoreEnabled && !span.spanContext.IsSampled() {
		return span
	}

	span.data = &SpanData{
		SpanContext:     span.spanContext,
>>>>>>> main
		StartTime:       time.Now(),
		SpanKind:        o.SpanKind,
		Name:            name,
		HasRemoteParent: remoteParent,
	}
<<<<<<< HEAD
	s.lruAttributes = newLruMap(cfg.MaxAttributesPerSpan)
	s.annotations = newEvictedQueue(cfg.MaxAnnotationEventsPerSpan)
	s.messageEvents = newEvictedQueue(cfg.MaxMessageEventsPerSpan)
	s.links = newEvictedQueue(cfg.MaxLinksPerSpan)

	if hasParent {
		s.data.ParentSpanID = parent.SpanID
=======
	span.lruAttributes = newLruMap(cfg.MaxAttributesPerSpan)
	span.annotations = newEvictedQueue(cfg.MaxAnnotationEventsPerSpan)
	span.messageEvents = newEvictedQueue(cfg.MaxMessageEventsPerSpan)
	span.links = newEvictedQueue(cfg.MaxLinksPerSpan)

	if hasParent {
		span.data.ParentSpanID = parent.SpanID
>>>>>>> main
	}
	if internal.LocalSpanStoreEnabled {
		var ss *spanStore
		ss = spanStoreForNameCreateIfNew(name)
		if ss != nil {
<<<<<<< HEAD
			s.spanStore = ss
			ss.add(s)
		}
	}

	return s
}

// End ends the span.
func (s *span) End() {
=======
			span.spanStore = ss
			ss.add(span)
		}
	}

	return span
}

// End ends the span.
func (s *Span) End() {
>>>>>>> main
	if s == nil {
		return
	}
	if s.executionTracerTaskEnd != nil {
		s.executionTracerTaskEnd()
	}
	if !s.IsRecordingEvents() {
		return
	}
	s.endOnce.Do(func() {
		exp, _ := exporters.Load().(exportersMap)
		mustExport := s.spanContext.IsSampled() && len(exp) > 0
		if s.spanStore != nil || mustExport {
			sd := s.makeSpanData()
			sd.EndTime = internal.MonotonicEndTime(sd.StartTime)
			if s.spanStore != nil {
				s.spanStore.finished(s, sd)
			}
			if mustExport {
				for e := range exp {
					e.ExportSpan(sd)
				}
			}
		}
	})
}

// makeSpanData produces a SpanData representing the current state of the Span.
// It requires that s.data is non-nil.
<<<<<<< HEAD
func (s *span) makeSpanData() *SpanData {
=======
func (s *Span) makeSpanData() *SpanData {
>>>>>>> main
	var sd SpanData
	s.mu.Lock()
	sd = *s.data
	if s.lruAttributes.len() > 0 {
		sd.Attributes = s.lruAttributesToAttributeMap()
		sd.DroppedAttributeCount = s.lruAttributes.droppedCount
	}
	if len(s.annotations.queue) > 0 {
		sd.Annotations = s.interfaceArrayToAnnotationArray()
		sd.DroppedAnnotationCount = s.annotations.droppedCount
	}
	if len(s.messageEvents.queue) > 0 {
		sd.MessageEvents = s.interfaceArrayToMessageEventArray()
		sd.DroppedMessageEventCount = s.messageEvents.droppedCount
	}
	if len(s.links.queue) > 0 {
		sd.Links = s.interfaceArrayToLinksArray()
		sd.DroppedLinkCount = s.links.droppedCount
	}
	s.mu.Unlock()
	return &sd
}

// SpanContext returns the SpanContext of the span.
<<<<<<< HEAD
func (s *span) SpanContext() SpanContext {
=======
func (s *Span) SpanContext() SpanContext {
>>>>>>> main
	if s == nil {
		return SpanContext{}
	}
	return s.spanContext
}

// SetName sets the name of the span, if it is recording events.
<<<<<<< HEAD
func (s *span) SetName(name string) {
=======
func (s *Span) SetName(name string) {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	s.mu.Lock()
	s.data.Name = name
	s.mu.Unlock()
}

// SetStatus sets the status of the span, if it is recording events.
<<<<<<< HEAD
func (s *span) SetStatus(status Status) {
=======
func (s *Span) SetStatus(status Status) {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	s.mu.Lock()
	s.data.Status = status
	s.mu.Unlock()
}

<<<<<<< HEAD
func (s *span) interfaceArrayToLinksArray() []Link {
=======
func (s *Span) interfaceArrayToLinksArray() []Link {
>>>>>>> main
	linksArr := make([]Link, 0, len(s.links.queue))
	for _, value := range s.links.queue {
		linksArr = append(linksArr, value.(Link))
	}
	return linksArr
}

<<<<<<< HEAD
func (s *span) interfaceArrayToMessageEventArray() []MessageEvent {
=======
func (s *Span) interfaceArrayToMessageEventArray() []MessageEvent {
>>>>>>> main
	messageEventArr := make([]MessageEvent, 0, len(s.messageEvents.queue))
	for _, value := range s.messageEvents.queue {
		messageEventArr = append(messageEventArr, value.(MessageEvent))
	}
	return messageEventArr
}

<<<<<<< HEAD
func (s *span) interfaceArrayToAnnotationArray() []Annotation {
=======
func (s *Span) interfaceArrayToAnnotationArray() []Annotation {
>>>>>>> main
	annotationArr := make([]Annotation, 0, len(s.annotations.queue))
	for _, value := range s.annotations.queue {
		annotationArr = append(annotationArr, value.(Annotation))
	}
	return annotationArr
}

<<<<<<< HEAD
func (s *span) lruAttributesToAttributeMap() map[string]interface{} {
=======
func (s *Span) lruAttributesToAttributeMap() map[string]interface{} {
>>>>>>> main
	attributes := make(map[string]interface{}, s.lruAttributes.len())
	for _, key := range s.lruAttributes.keys() {
		value, ok := s.lruAttributes.get(key)
		if ok {
			keyStr := key.(string)
			attributes[keyStr] = value
		}
	}
	return attributes
}

<<<<<<< HEAD
func (s *span) copyToCappedAttributes(attributes []Attribute) {
=======
func (s *Span) copyToCappedAttributes(attributes []Attribute) {
>>>>>>> main
	for _, a := range attributes {
		s.lruAttributes.add(a.key, a.value)
	}
}

<<<<<<< HEAD
func (s *span) addChild() {
=======
func (s *Span) addChild() {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	s.mu.Lock()
	s.data.ChildSpanCount++
	s.mu.Unlock()
}

// AddAttributes sets attributes in the span.
//
// Existing attributes whose keys appear in the attributes parameter are overwritten.
<<<<<<< HEAD
func (s *span) AddAttributes(attributes ...Attribute) {
=======
func (s *Span) AddAttributes(attributes ...Attribute) {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	s.mu.Lock()
	s.copyToCappedAttributes(attributes)
	s.mu.Unlock()
}

<<<<<<< HEAD
func (s *span) printStringInternal(attributes []Attribute, str string) {
	now := time.Now()
	var am map[string]interface{}
	if len(attributes) != 0 {
		am = make(map[string]interface{}, len(attributes))
		for _, attr := range attributes {
			am[attr.key] = attr.value
		}
	}
	s.mu.Lock()
	s.annotations.add(Annotation{
		Time:       now,
		Message:    str,
		Attributes: am,
=======
// copyAttributes copies a slice of Attributes into a map.
func copyAttributes(m map[string]interface{}, attributes []Attribute) {
	for _, a := range attributes {
		m[a.key] = a.value
	}
}

func (s *Span) lazyPrintfInternal(attributes []Attribute, format string, a ...interface{}) {
	now := time.Now()
	msg := fmt.Sprintf(format, a...)
	var m map[string]interface{}
	s.mu.Lock()
	if len(attributes) != 0 {
		m = make(map[string]interface{}, len(attributes))
		copyAttributes(m, attributes)
	}
	s.annotations.add(Annotation{
		Time:       now,
		Message:    msg,
		Attributes: m,
	})
	s.mu.Unlock()
}

func (s *Span) printStringInternal(attributes []Attribute, str string) {
	now := time.Now()
	var a map[string]interface{}
	s.mu.Lock()
	if len(attributes) != 0 {
		a = make(map[string]interface{}, len(attributes))
		copyAttributes(a, attributes)
	}
	s.annotations.add(Annotation{
		Time:       now,
		Message:    str,
		Attributes: a,
>>>>>>> main
	})
	s.mu.Unlock()
}

// Annotate adds an annotation with attributes.
// Attributes can be nil.
<<<<<<< HEAD
func (s *span) Annotate(attributes []Attribute, str string) {
=======
func (s *Span) Annotate(attributes []Attribute, str string) {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	s.printStringInternal(attributes, str)
}

// Annotatef adds an annotation with attributes.
<<<<<<< HEAD
func (s *span) Annotatef(attributes []Attribute, format string, a ...interface{}) {
	if !s.IsRecordingEvents() {
		return
	}
	s.printStringInternal(attributes, fmt.Sprintf(format, a...))
=======
func (s *Span) Annotatef(attributes []Attribute, format string, a ...interface{}) {
	if !s.IsRecordingEvents() {
		return
	}
	s.lazyPrintfInternal(attributes, format, a...)
>>>>>>> main
}

// AddMessageSendEvent adds a message send event to the span.
//
// messageID is an identifier for the message, which is recommended to be
// unique in this span and the same between the send event and the receive
// event (this allows to identify a message between the sender and receiver).
// For example, this could be a sequence id.
<<<<<<< HEAD
func (s *span) AddMessageSendEvent(messageID, uncompressedByteSize, compressedByteSize int64) {
=======
func (s *Span) AddMessageSendEvent(messageID, uncompressedByteSize, compressedByteSize int64) {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	now := time.Now()
	s.mu.Lock()
	s.messageEvents.add(MessageEvent{
		Time:                 now,
		EventType:            MessageEventTypeSent,
		MessageID:            messageID,
		UncompressedByteSize: uncompressedByteSize,
		CompressedByteSize:   compressedByteSize,
	})
	s.mu.Unlock()
}

// AddMessageReceiveEvent adds a message receive event to the span.
//
// messageID is an identifier for the message, which is recommended to be
// unique in this span and the same between the send event and the receive
// event (this allows to identify a message between the sender and receiver).
// For example, this could be a sequence id.
<<<<<<< HEAD
func (s *span) AddMessageReceiveEvent(messageID, uncompressedByteSize, compressedByteSize int64) {
=======
func (s *Span) AddMessageReceiveEvent(messageID, uncompressedByteSize, compressedByteSize int64) {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	now := time.Now()
	s.mu.Lock()
	s.messageEvents.add(MessageEvent{
		Time:                 now,
		EventType:            MessageEventTypeRecv,
		MessageID:            messageID,
		UncompressedByteSize: uncompressedByteSize,
		CompressedByteSize:   compressedByteSize,
	})
	s.mu.Unlock()
}

// AddLink adds a link to the span.
<<<<<<< HEAD
func (s *span) AddLink(l Link) {
=======
func (s *Span) AddLink(l Link) {
>>>>>>> main
	if !s.IsRecordingEvents() {
		return
	}
	s.mu.Lock()
	s.links.add(l)
	s.mu.Unlock()
}

<<<<<<< HEAD
func (s *span) String() string {
=======
func (s *Span) String() string {
>>>>>>> main
	if s == nil {
		return "<nil>"
	}
	if s.data == nil {
		return fmt.Sprintf("span %s", s.spanContext.SpanID)
	}
	s.mu.Lock()
	str := fmt.Sprintf("span %s %q", s.spanContext.SpanID, s.data.Name)
	s.mu.Unlock()
	return str
}

var config atomic.Value // access atomically

func init() {
<<<<<<< HEAD
	config.Store(&Config{
		DefaultSampler:             ProbabilitySampler(defaultSamplingProbability),
		IDGenerator:                &defaultIDGenerator{},
=======
	gen := &defaultIDGenerator{}
	// initialize traceID and spanID generators.
	var rngSeed int64
	for _, p := range []interface{}{
		&rngSeed, &gen.traceIDAdd, &gen.nextSpanID, &gen.spanIDInc,
	} {
		binary.Read(crand.Reader, binary.LittleEndian, p)
	}
	gen.traceIDRand = rand.New(rand.NewSource(rngSeed))
	gen.spanIDInc |= 1

	config.Store(&Config{
		DefaultSampler:             ProbabilitySampler(defaultSamplingProbability),
		IDGenerator:                gen,
>>>>>>> main
		MaxAttributesPerSpan:       DefaultMaxAttributesPerSpan,
		MaxAnnotationEventsPerSpan: DefaultMaxAnnotationEventsPerSpan,
		MaxMessageEventsPerSpan:    DefaultMaxMessageEventsPerSpan,
		MaxLinksPerSpan:            DefaultMaxLinksPerSpan,
	})
}

type defaultIDGenerator struct {
	sync.Mutex

	// Please keep these as the first fields
	// so that these 8 byte fields will be aligned on addresses
	// divisible by 8, on both 32-bit and 64-bit machines when
	// performing atomic increments and accesses.
	// See:
	// * https://github.com/census-instrumentation/opencensus-go/issues/587
	// * https://github.com/census-instrumentation/opencensus-go/issues/865
	// * https://golang.org/pkg/sync/atomic/#pkg-note-BUG
	nextSpanID uint64
	spanIDInc  uint64

	traceIDAdd  [2]uint64
	traceIDRand *rand.Rand
<<<<<<< HEAD

	initOnce sync.Once
}

// init initializes the generator on the first call to avoid consuming entropy
// unnecessarily.
func (gen *defaultIDGenerator) init() {
	gen.initOnce.Do(func() {
		// initialize traceID and spanID generators.
		var rngSeed int64
		for _, p := range []interface{}{
			&rngSeed, &gen.traceIDAdd, &gen.nextSpanID, &gen.spanIDInc,
		} {
			binary.Read(crand.Reader, binary.LittleEndian, p)
		}
		gen.traceIDRand = rand.New(rand.NewSource(rngSeed))
		gen.spanIDInc |= 1
	})
=======
>>>>>>> main
}

// NewSpanID returns a non-zero span ID from a randomly-chosen sequence.
func (gen *defaultIDGenerator) NewSpanID() [8]byte {
	var id uint64
	for id == 0 {
		id = atomic.AddUint64(&gen.nextSpanID, gen.spanIDInc)
	}
	var sid [8]byte
	binary.LittleEndian.PutUint64(sid[:], id)
	return sid
}

// NewTraceID returns a non-zero trace ID from a randomly-chosen sequence.
// mu should be held while this function is called.
func (gen *defaultIDGenerator) NewTraceID() [16]byte {
	var tid [16]byte
	// Construct the trace ID from two outputs of traceIDRand, with a constant
	// added to each half for additional entropy.
	gen.Lock()
	binary.LittleEndian.PutUint64(tid[0:8], gen.traceIDRand.Uint64()+gen.traceIDAdd[0])
	binary.LittleEndian.PutUint64(tid[8:16], gen.traceIDRand.Uint64()+gen.traceIDAdd[1])
	gen.Unlock()
	return tid
}

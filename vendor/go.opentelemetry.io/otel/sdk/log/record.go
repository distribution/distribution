// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package log // import "go.opentelemetry.io/otel/sdk/log"

import (
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/internal/global"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
)

// attributesInlineCount is the number of attributes that are efficiently
// stored in an array within a Record. This value is borrowed from slog which
// performed a quantitative survey of log library use and found this value to
// cover 95% of all use-cases (https://go.dev/blog/slog#performance).
const attributesInlineCount = 5

var logAttrDropped = sync.OnceFunc(func() {
	global.Warn("limit reached: dropping log Record attributes")
})

// indexPool is a pool of index maps used for de-duplication.
var indexPool = sync.Pool{
	New: func() any { return make(map[string]int) },
}

func getIndex() map[string]int {
	return indexPool.Get().(map[string]int)
}

func putIndex(index map[string]int) {
	clear(index)
	indexPool.Put(index)
}

// Record is a log record emitted by the Logger.
//
// Do not create instances of Record on your own in production code.
// You can use [go.opentelemetry.io/otel/sdk/log/logtest.RecordFactory]
// for testing purposes.
type Record struct {
	// Do not embed the log.Record. Attributes need to be overwrite-able and
	// deep-copying needs to be possible.

	timestamp         time.Time
	observedTimestamp time.Time
	severity          log.Severity
	severityText      string
	body              log.Value

	// The fields below are for optimizing the implementation of Attributes and
	// AddAttributes. This design is borrowed from the slog Record type:
	// https://cs.opensource.google/go/go/+/refs/tags/go1.22.0:src/log/slog/record.go;l=20

	// Allocation optimization: an inline array sized to hold
	// the majority of log calls (based on examination of open-source
	// code). It holds the start of the list of attributes.
	front [attributesInlineCount]log.KeyValue

	// The number of attributes in front.
	nFront int

	// The list of attributes except for those in front.
	// Invariants:
	//   - len(back) > 0 if nFront == len(front)
	//   - Unused array elements are zero-ed. Used to detect mistakes.
	back []log.KeyValue

	// dropped is the count of attributes that have been dropped when limits
	// were reached.
	dropped int

	traceID    trace.TraceID
	spanID     trace.SpanID
	traceFlags trace.TraceFlags

	// resource represents the entity that collected the log.
	resource *resource.Resource

	// scope is the Scope that the Logger was created with.
	scope *instrumentation.Scope

	attributeValueLengthLimit int
	attributeCountLimit       int

	noCmp [0]func() //nolint: unused  // This is indeed used.
}

func (r *Record) addDropped(n int) {
	logAttrDropped()
	r.dropped += n
}

func (r *Record) setDropped(n int) {
	logAttrDropped()
	r.dropped = n
}

// Timestamp returns the time when the log record occurred.
func (r *Record) Timestamp() time.Time {
	return r.timestamp
}

// SetTimestamp sets the time when the log record occurred.
func (r *Record) SetTimestamp(t time.Time) {
	r.timestamp = t
}

// ObservedTimestamp returns the time when the log record was observed.
func (r *Record) ObservedTimestamp() time.Time {
	return r.observedTimestamp
}

// SetObservedTimestamp sets the time when the log record was observed.
func (r *Record) SetObservedTimestamp(t time.Time) {
	r.observedTimestamp = t
}

// Severity returns the severity of the log record.
func (r *Record) Severity() log.Severity {
	return r.severity
}

// SetSeverity sets the severity level of the log record.
func (r *Record) SetSeverity(level log.Severity) {
	r.severity = level
}

// SeverityText returns severity (also known as log level) text. This is the
// original string representation of the severity as it is known at the source.
func (r *Record) SeverityText() string {
	return r.severityText
}

// SetSeverityText sets severity (also known as log level) text. This is the
// original string representation of the severity as it is known at the source.
func (r *Record) SetSeverityText(text string) {
	r.severityText = text
}

// Body returns the body of the log record.
func (r *Record) Body() log.Value {
	return r.body
}

// SetBody sets the body of the log record.
func (r *Record) SetBody(v log.Value) {
	r.body = v
}

// WalkAttributes walks all attributes the log record holds by calling f for
// each on each [log.KeyValue] in the [Record]. Iteration stops if f returns false.
func (r *Record) WalkAttributes(f func(log.KeyValue) bool) {
	for i := 0; i < r.nFront; i++ {
		if !f(r.front[i]) {
			return
		}
	}
	for _, a := range r.back {
		if !f(a) {
			return
		}
	}
}

// AddAttributes adds attributes to the log record.
// Attributes in attrs will overwrite any attribute already added to r with the same key.
func (r *Record) AddAttributes(attrs ...log.KeyValue) {
	n := r.AttributesLen()
	if n == 0 {
		// Avoid the more complex duplicate map lookups below.
		var drop int
		attrs, drop = dedup(attrs)
		r.setDropped(drop)

		attrs, drop = head(attrs, r.attributeCountLimit)
		r.addDropped(drop)

		r.addAttrs(attrs)
		return
	}

	// Used to find duplicates between attrs and existing attributes in r.
	rIndex := r.attrIndex()
	defer putIndex(rIndex)

	// Unique attrs that need to be added to r. This uses the same underlying
	// array as attrs.
	//
	// Note, do not iterate attrs twice by just calling dedup(attrs) here.
	unique := attrs[:0]
	// Used to find duplicates within attrs itself. The index value is the
	// index of the element in unique.
	uIndex := getIndex()
	defer putIndex(uIndex)

	// Deduplicate attrs within the scope of all existing attributes.
	for _, a := range attrs {
		// Last-value-wins for any duplicates in attrs.
		idx, found := uIndex[a.Key]
		if found {
			r.addDropped(1)
			unique[idx] = a
			continue
		}

		idx, found = rIndex[a.Key]
		if found {
			// New attrs overwrite any existing with the same key.
			r.addDropped(1)
			if idx < 0 {
				r.front[-(idx + 1)] = a
			} else {
				r.back[idx] = a
			}
		} else {
			// Unique attribute.
			unique = append(unique, a)
			uIndex[a.Key] = len(unique) - 1
		}
	}
	attrs = unique

	if r.attributeCountLimit > 0 && n+len(attrs) > r.attributeCountLimit {
		// Truncate the now unique attributes to comply with limit.
		//
		// Do not use head(attrs, r.attributeCountLimit - n) here. If
		// (r.attributeCountLimit - n) <= 0 attrs needs to be emptied.
		last := max(0, r.attributeCountLimit-n)
		r.addDropped(len(attrs) - last)
		attrs = attrs[:last]
	}

	r.addAttrs(attrs)
}

// attrIndex returns an index map for all attributes in the Record r. The index
// maps the attribute key to location the attribute is stored. If the value is
// < 0 then -(value + 1) (e.g. -1 -> 0, -2 -> 1, -3 -> 2) represents the index
// in r.nFront. Otherwise, the index is the exact index of r.back.
//
// The returned index is taken from the indexPool. It is the callers
// responsibility to return the index to that pool (putIndex) when done.
func (r *Record) attrIndex() map[string]int {
	index := getIndex()
	for i := 0; i < r.nFront; i++ {
		key := r.front[i].Key
		index[key] = -i - 1 // stored in front: negative index.
	}
	for i := 0; i < len(r.back); i++ {
		key := r.back[i].Key
		index[key] = i // stored in back: positive index.
	}
	return index
}

// addAttrs adds attrs to the Record r. This does not validate any limits or
// duplication of attributes, these tasks are left to the caller to handle
// prior to calling.
func (r *Record) addAttrs(attrs []log.KeyValue) {
	var i int
	for i = 0; i < len(attrs) && r.nFront < len(r.front); i++ {
		a := attrs[i]
		r.front[r.nFront] = r.applyAttrLimits(a)
		r.nFront++
	}

	for j, a := range attrs[i:] {
		attrs[i+j] = r.applyAttrLimits(a)
	}
	r.back = slices.Grow(r.back, len(attrs[i:]))
	r.back = append(r.back, attrs[i:]...)
}

// SetAttributes sets (and overrides) attributes to the log record.
func (r *Record) SetAttributes(attrs ...log.KeyValue) {
	var drop int
	attrs, drop = dedup(attrs)
	r.setDropped(drop)

	attrs, drop = head(attrs, r.attributeCountLimit)
	r.addDropped(drop)

	r.nFront = 0
	var i int
	for i = 0; i < len(attrs) && r.nFront < len(r.front); i++ {
		a := attrs[i]
		r.front[r.nFront] = r.applyAttrLimits(a)
		r.nFront++
	}

	r.back = slices.Clone(attrs[i:])
	for i, a := range r.back {
		r.back[i] = r.applyAttrLimits(a)
	}
}

// head returns the first n values of kvs along with the number of elements
// dropped. If n is less than or equal to zero, kvs is returned with 0.
func head(kvs []log.KeyValue, n int) (out []log.KeyValue, dropped int) {
	if n > 0 && len(kvs) > n {
		return kvs[:n], len(kvs) - n
	}
	return kvs, 0
}

// dedup deduplicates kvs front-to-back with the last value saved.
func dedup(kvs []log.KeyValue) (unique []log.KeyValue, dropped int) {
	index := getIndex()
	defer putIndex(index)

	unique = kvs[:0] // Use the same underlying array as kvs.
	for _, a := range kvs {
		idx, found := index[a.Key]
		if found {
			dropped++
			unique[idx] = a
		} else {
			unique = append(unique, a)
			index[a.Key] = len(unique) - 1
		}
	}
	return unique, dropped
}

// AttributesLen returns the number of attributes in the log record.
func (r *Record) AttributesLen() int {
	return r.nFront + len(r.back)
}

// DroppedAttributes returns the number of attributes dropped due to limits
// being reached.
func (r *Record) DroppedAttributes() int {
	return r.dropped
}

// TraceID returns the trace ID or empty array.
func (r *Record) TraceID() trace.TraceID {
	return r.traceID
}

// SetTraceID sets the trace ID.
func (r *Record) SetTraceID(id trace.TraceID) {
	r.traceID = id
}

// SpanID returns the span ID or empty array.
func (r *Record) SpanID() trace.SpanID {
	return r.spanID
}

// SetSpanID sets the span ID.
func (r *Record) SetSpanID(id trace.SpanID) {
	r.spanID = id
}

// TraceFlags returns the trace flags.
func (r *Record) TraceFlags() trace.TraceFlags {
	return r.traceFlags
}

// SetTraceFlags sets the trace flags.
func (r *Record) SetTraceFlags(flags trace.TraceFlags) {
	r.traceFlags = flags
}

// Resource returns the entity that collected the log.
func (r *Record) Resource() resource.Resource {
	if r.resource == nil {
		return *resource.Empty()
	}
	return *r.resource
}

// InstrumentationScope returns the scope that the Logger was created with.
func (r *Record) InstrumentationScope() instrumentation.Scope {
	if r.scope == nil {
		return instrumentation.Scope{}
	}
	return *r.scope
}

// Clone returns a copy of the record with no shared state. The original record
// and the clone can both be modified without interfering with each other.
func (r *Record) Clone() Record {
	res := *r
	res.back = slices.Clone(r.back)
	return res
}

func (r *Record) applyAttrLimits(attr log.KeyValue) log.KeyValue {
	attr.Value = r.applyValueLimits(attr.Value)
	return attr
}

func (r *Record) applyValueLimits(val log.Value) log.Value {
	switch val.Kind() {
	case log.KindString:
		s := val.AsString()
		if len(s) > r.attributeValueLengthLimit {
			val = log.StringValue(truncate(s, r.attributeValueLengthLimit))
		}
	case log.KindSlice:
		sl := val.AsSlice()
		for i := range sl {
			sl[i] = r.applyValueLimits(sl[i])
		}
		val = log.SliceValue(sl...)
	case log.KindMap:
		// Deduplicate then truncate. Do not do at the same time to avoid
		// wasted truncation operations.
		kvs, dropped := dedup(val.AsMap())
		r.addDropped(dropped)
		for i := range kvs {
			kvs[i] = r.applyAttrLimits(kvs[i])
		}
		val = log.MapValue(kvs...)
	}
	return val
}

// truncate returns a copy of str truncated to have a length of at most n
// characters. If the length of str is less than n, str itself is returned.
//
// The truncate of str ensures that no valid UTF-8 code point is split. The
// copy returned will be less than n if a characters straddles the length
// limit.
//
// No truncation is performed if n is less than zero.
func truncate(str string, n int) string {
	if n < 0 {
		return str
	}

	// cut returns a copy of the s truncated to not exceed a length of n. If
	// invalid UTF-8 is encountered, s is returned with false. Otherwise, the
	// truncated copy will be returned with true.
	cut := func(s string) (string, bool) {
		var i int
		for i = 0; i < n; {
			r, size := utf8.DecodeRuneInString(s[i:])
			if r == utf8.RuneError {
				return s, false
			}
			if i+size > n {
				break
			}
			i += size
		}
		return s[:i], true
	}

	cp, ok := cut(str)
	if !ok {
		cp, _ = cut(strings.ToValidUTF8(str, ""))
	}
	return cp
}

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

package ptrace // import "go.opentelemetry.io/collector/pdata/ptrace"

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/gogo/protobuf/jsonpb"
	jsoniter "github.com/json-iterator/go"

	"go.opentelemetry.io/collector/pdata/internal"
	otlpcommon "go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1"
	otlptrace "go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1"
)

// NewJSONMarshaler returns a model.Marshaler. Marshals to OTLP json bytes.
func NewJSONMarshaler() Marshaler {
	return newJSONMarshaler()
}

type jsonMarshaler struct {
	delegate jsonpb.Marshaler
}

func newJSONMarshaler() *jsonMarshaler {
	return &jsonMarshaler{delegate: jsonpb.Marshaler{}}
}

func (e *jsonMarshaler) MarshalTraces(td Traces) ([]byte, error) {
	buf := bytes.Buffer{}
	pb := internal.TracesToProto(td)
	err := e.delegate.Marshal(&buf, &pb)
	return buf.Bytes(), err
}

// NewJSONUnmarshaler returns a model.Unmarshaler. Unmarshalls from OTLP json bytes.
func NewJSONUnmarshaler() Unmarshaler {
	return &jsonUnmarshaler{}
}

type jsonUnmarshaler struct {
}

func (d *jsonUnmarshaler) UnmarshalTraces(buf []byte) (Traces, error) {
	iter := jsoniter.ConfigFastest.BorrowIterator(buf)
	defer jsoniter.ConfigFastest.ReturnIterator(iter)
	td := readTraceData(iter)
	err := iter.Error
	return internal.TracesFromProto(td), err
}

func readTraceData(iter *jsoniter.Iterator) otlptrace.TracesData {
	td := otlptrace.TracesData{}
	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "resourceSpans", "resource_spans":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				td.ResourceSpans = append(td.ResourceSpans, readResourceSpans(iter))
				return true
			})
		default:
			iter.ReportError("root", fmt.Sprintf("unknown field:%v", f))
		}
		return true
	})
	return td
}

func readResourceSpans(iter *jsoniter.Iterator) *otlptrace.ResourceSpans {
	rs := &otlptrace.ResourceSpans{}

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "resource":
			iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
				switch f {
				case "attributes":
					iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
						rs.Resource.Attributes = append(rs.Resource.Attributes, readAttribute(iter))
						return true
					})
				case "droppedAttributesCount", "dropped_attributes_count":
					rs.Resource.DroppedAttributesCount = iter.ReadUint32()
				default:
					iter.ReportError("readResourceSpans.resource", fmt.Sprintf("unknown field:%v", f))
				}
				return true
			})
		case "instrumentationLibrarySpans", "instrumentation_library_spans", "scopeSpans", "scope_spans":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				rs.ScopeSpans = append(rs.ScopeSpans,
					readInstrumentationLibrarySpans(iter))
				return true
			})
		case "schemaUrl", "schema_url":
			rs.SchemaUrl = iter.ReadString()
		default:
			iter.ReportError("readResourceSpans", fmt.Sprintf("unknown field:%v", f))
		}
		return true
	})
	return rs
}

func readInstrumentationLibrarySpans(iter *jsoniter.Iterator) *otlptrace.ScopeSpans {
	ils := &otlptrace.ScopeSpans{}

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "instrumentationLibrary", "instrumentation_library", "scope":
			iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
				switch f {
				case "name":
					ils.Scope.Name = iter.ReadString()
				case "version":
					ils.Scope.Version = iter.ReadString()
				default:
					iter.ReportError("readInstrumentationLibrarySpans.instrumentationLibrary", fmt.Sprintf("unknown field:%v", f))
				}
				return true
			})
		case "spans":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				ils.Spans = append(ils.Spans, readSpan(iter))
				return true
			})
		case "schemaUrl", "schema_url":
			ils.SchemaUrl = iter.ReadString()
		default:
			iter.ReportError("readInstrumentationLibrarySpans", fmt.Sprintf("unknown field:%v", f))
		}
		return true
	})
	return ils
}

func readSpan(iter *jsoniter.Iterator) *otlptrace.Span {
	sp := &otlptrace.Span{}

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "traceId", "trace_id":
			if err := sp.TraceId.UnmarshalJSON([]byte(iter.ReadString())); err != nil {
				iter.ReportError("readSpan.traceId", fmt.Sprintf("parse trace_id:%v", err))
			}
		case "spanId", "span_id":
			if err := sp.SpanId.UnmarshalJSON([]byte(iter.ReadString())); err != nil {
				iter.ReportError("readSpan.spanId", fmt.Sprintf("parse span_id:%v", err))
			}
		case "traceState", "trace_state":
			sp.TraceState = iter.ReadString()
		case "parentSpanId", "parent_span_id":
			if err := sp.ParentSpanId.UnmarshalJSON([]byte(iter.ReadString())); err != nil {
				iter.ReportError("readSpan.parentSpanId", fmt.Sprintf("parse parent_span_id:%v", err))
			}
		case "name":
			sp.Name = iter.ReadString()
		case "kind":
			sp.Kind = readSpanKind(iter)
		case "startTimeUnixNano", "start_time_unix_nano":
			sp.StartTimeUnixNano = uint64(readInt64(iter))
		case "endTimeUnixNano", "end_time_unix_nano":
			sp.EndTimeUnixNano = uint64(readInt64(iter))
		case "attributes":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				sp.Attributes = append(sp.Attributes, readAttribute(iter))
				return true
			})
		case "droppedAttributesCount", "dropped_attributes_count":
			sp.DroppedAttributesCount = iter.ReadUint32()
		case "events":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				sp.Events = append(sp.Events, readSpanEvent(iter))
				return true
			})
		case "droppedEventsCount", "dropped_events_count":
			sp.DroppedEventsCount = iter.ReadUint32()
		case "links":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				sp.Links = append(sp.Links, readSpanLink(iter))
				return true
			})
		case "droppedLinksCount", "dropped_links_count":
			sp.DroppedLinksCount = iter.ReadUint32()
		case "status":
			iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
				switch f {
				case "message":
					sp.Status.Message = iter.ReadString()
				case "code":
					sp.Status.Code = readStatusCode(iter)
				default:
					iter.ReportError("readSpan.status", fmt.Sprintf("unknown field:%v", f))
				}
				return true
			})
		default:
			iter.ReportError("readSpan", fmt.Sprintf("unknown field:%v", f))
		}
		return true
	})
	return sp
}

func readSpanLink(iter *jsoniter.Iterator) *otlptrace.Span_Link {
	link := &otlptrace.Span_Link{}

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "traceId", "trace_id":
			if err := link.TraceId.UnmarshalJSON([]byte(iter.ReadString())); err != nil {
				iter.ReportError("readSpanLink", fmt.Sprintf("parse trace_id:%v", err))
			}
		case "spanId", "span_id":
			if err := link.SpanId.UnmarshalJSON([]byte(iter.ReadString())); err != nil {
				iter.ReportError("readSpanLink", fmt.Sprintf("parse span_id:%v", err))
			}
		case "traceState", "trace_state":
			link.TraceState = iter.ReadString()
		case "attributes":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				link.Attributes = append(link.Attributes, readAttribute(iter))
				return true
			})
		case "droppedAttributesCount", "dropped_attributes_count":
			link.DroppedAttributesCount = iter.ReadUint32()
		default:
			iter.ReportError("readSpanLink", fmt.Sprintf("unknown field:%v", f))
		}
		return true
	})
	return link
}

func readSpanEvent(iter *jsoniter.Iterator) *otlptrace.Span_Event {
	event := &otlptrace.Span_Event{}

	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "timeUnixNano", "time_unix_nano":
			event.TimeUnixNano = uint64(readInt64(iter))
		case "name":
			event.Name = iter.ReadString()
		case "attributes":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				event.Attributes = append(event.Attributes, readAttribute(iter))
				return true
			})
		case "droppedAttributesCount", "dropped_attributes_count":
			event.DroppedAttributesCount = iter.ReadUint32()
		default:
			iter.ReportError("readSpanEvent", fmt.Sprintf("unknown field:%v", f))
		}
		return true
	})
	return event
}

func readAttribute(iter *jsoniter.Iterator) otlpcommon.KeyValue {
	kv := otlpcommon.KeyValue{}
	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "key":
			kv.Key = iter.ReadString()
		case "value":
			iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
				kv.Value = readAnyValue(iter, f)
				return true
			})
		default:
			iter.ReportError("readAttribute", fmt.Sprintf("unknown field:%v", f))
		}
		return true
	})
	return kv
}

func readAnyValue(iter *jsoniter.Iterator, f string) otlpcommon.AnyValue {
	switch f {
	case "stringValue", "string_value":
		return otlpcommon.AnyValue{
			Value: &otlpcommon.AnyValue_StringValue{
				StringValue: iter.ReadString(),
			},
		}
	case "boolValue", "bool_value":
		return otlpcommon.AnyValue{
			Value: &otlpcommon.AnyValue_BoolValue{
				BoolValue: iter.ReadBool(),
			},
		}
	case "intValue", "int_value":
		return otlpcommon.AnyValue{
			Value: &otlpcommon.AnyValue_IntValue{
				IntValue: readInt64(iter),
			},
		}
	case "doubleValue", "double_value":
		return otlpcommon.AnyValue{
			Value: &otlpcommon.AnyValue_DoubleValue{
				DoubleValue: iter.ReadFloat64(),
			},
		}
	case "bytesValue", "bytes_value":
		v, err := base64.StdEncoding.DecodeString(iter.ReadString())
		if err != nil {
			iter.ReportError("bytesValue", fmt.Sprintf("base64 decode:%v", err))
			return otlpcommon.AnyValue{}
		}
		return otlpcommon.AnyValue{
			Value: &otlpcommon.AnyValue_BytesValue{
				BytesValue: v,
			},
		}
	case "arrayValue", "array_value":
		return otlpcommon.AnyValue{
			Value: &otlpcommon.AnyValue_ArrayValue{
				ArrayValue: readArray(iter),
			},
		}
	case "kvlistValue", "kvlist_value":
		return otlpcommon.AnyValue{
			Value: &otlpcommon.AnyValue_KvlistValue{
				KvlistValue: readKvlistValue(iter),
			},
		}
	default:
		iter.ReportError("readAnyValue", fmt.Sprintf("unknown field:%v", f))
		return otlpcommon.AnyValue{}
	}
}

func readArray(iter *jsoniter.Iterator) *otlpcommon.ArrayValue {
	v := &otlpcommon.ArrayValue{}
	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "values":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
					v.Values = append(v.Values, readAnyValue(iter, f))
					return true
				})
				return true
			})
		default:
			iter.ReportError("readArray", fmt.Sprintf("unknown field:%s", f))
		}
		return true
	})
	return v
}

func readKvlistValue(iter *jsoniter.Iterator) *otlpcommon.KeyValueList {
	v := &otlpcommon.KeyValueList{}
	iter.ReadObjectCB(func(iter *jsoniter.Iterator, f string) bool {
		switch f {
		case "values":
			iter.ReadArrayCB(func(iter *jsoniter.Iterator) bool {
				v.Values = append(v.Values, readAttribute(iter))
				return true
			})
		default:
			iter.ReportError("readKvlistValue", fmt.Sprintf("unknown field:%s", f))
		}
		return true
	})
	return v
}

func readInt64(iter *jsoniter.Iterator) int64 {
	return iter.ReadAny().ToInt64()
}

func readSpanKind(iter *jsoniter.Iterator) otlptrace.Span_SpanKind {
	any := iter.ReadAny()
	if v := any.ToInt(); v > 0 {
		return otlptrace.Span_SpanKind(v)
	}
	v := any.ToString()
	return otlptrace.Span_SpanKind(otlptrace.Span_SpanKind_value[v])
}

func readStatusCode(iter *jsoniter.Iterator) otlptrace.Status_StatusCode {
	any := iter.ReadAny()
	if v := any.ToInt(); v > 0 {
		return otlptrace.Status_StatusCode(v)
	}
	v := any.ToString()
	return otlptrace.Status_StatusCode(otlptrace.Status_StatusCode_value[v])
}

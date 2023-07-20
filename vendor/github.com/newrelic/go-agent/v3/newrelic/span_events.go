// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"time"
)

// https://source.datanerd.us/agents/agent-specs/blob/master/Span-Events.md

type spanCategory string

const (
	spanCategoryHTTP      spanCategory = "http"
	spanCategoryDatastore              = "datastore"
	// spanCategoryGeneric is a generic span category.
	spanCategoryGeneric = "generic"
)

// spanEvent represents a span event, necessary to support Distributed Tracing.
type spanEvent struct {
	TraceID         string
	GUID            string
	ParentID        string
	TransactionID   string
	Sampled         bool
	Priority        priority
	Timestamp       time.Time
	Duration        time.Duration
	Name            string
	TxnName         string
	Category        spanCategory
	Component       string
	Kind            string
	IsEntrypoint    bool
	TrustedParentID string
	TracingVendors  string
	AgentAttributes spanAttributeMap
	UserAttributes  spanAttributeMap
}

// WriteJSON prepares JSON in the format expected by the collector.
func (e *spanEvent) WriteJSON(buf *bytes.Buffer) {
	w := jsonFieldsWriter{buf: buf}
	buf.WriteByte('[')
	buf.WriteByte('{')
	w.stringField("type", "Span")
	w.stringField("traceId", e.TraceID)
	w.stringField("guid", e.GUID)
	if "" != e.ParentID {
		w.stringField("parentId", e.ParentID)
	}
	w.stringField("transactionId", e.TransactionID)
	w.boolField("sampled", e.Sampled)
	w.writerField("priority", e.Priority)
	w.intField("timestamp", timeToIntMillis(e.Timestamp))
	w.floatField("duration", e.Duration.Seconds())
	w.stringField("name", e.Name)
	w.stringField("category", string(e.Category))
	if e.IsEntrypoint {
		w.boolField("nr.entryPoint", true)
	}
	if e.Component != "" {
		w.stringField("component", e.Component)
	}
	if e.Kind != "" {
		w.stringField("span.kind", e.Kind)
	}
	if "" != e.TrustedParentID {
		w.stringField("trustedParentId", e.TrustedParentID)
	}
	if "" != e.TracingVendors {
		w.stringField("tracingVendors", e.TracingVendors)
	}
	if "" != e.TxnName {
		w.stringField("transaction.name", e.TxnName)
	}
	buf.WriteByte('}')
	buf.WriteByte(',')
	buf.WriteByte('{')

	writeAttrs(buf, e.UserAttributes)

	buf.WriteByte('}')
	buf.WriteByte(',')
	buf.WriteByte('{')

	writeAttrs(buf, e.AgentAttributes)

	buf.WriteByte('}')
	buf.WriteByte(']')
}

func writeAttrs(buf *bytes.Buffer, attrs spanAttributeMap) {
	w := jsonFieldsWriter{buf: buf}
	for key, val := range attrs {
		w.writerField(key, val)
	}
}

// MarshalJSON is used for testing.
func (e *spanEvent) MarshalJSON() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 256))

	e.WriteJSON(buf)

	return buf.Bytes(), nil
}

type spanEvents struct {
	*analyticsEvents
}

func newSpanEvents(max int) *spanEvents {
	return &spanEvents{
		analyticsEvents: newAnalyticsEvents(max),
	}
}

func (events *spanEvents) addEventPopulated(e *spanEvent) {
	events.analyticsEvents.addEvent(analyticsEvent{priority: e.Priority, jsonWriter: e})
}

// MergeSpanEvents merges the span events from a transaction into the
// harvest's span events.  This should only be called if the transaction was
// sampled and span events are enabled.
func (events *spanEvents) MergeSpanEvents(evts []*spanEvent) {
	for _, evt := range evts {
		events.addEventPopulated(evt)
	}
}

func (events *spanEvents) MergeIntoHarvest(h *harvest) {
	h.SpanEvents.mergeFailed(events.analyticsEvents)
}

func (events *spanEvents) Data(agentRunID string, harvestStart time.Time) ([]byte, error) {
	return events.CollectorJSON(agentRunID)
}

func (events *spanEvents) EndpointMethod() string {
	return cmdSpanEvents
}

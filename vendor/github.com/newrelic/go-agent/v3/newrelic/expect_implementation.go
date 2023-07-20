// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/newrelic/go-agent/v3/internal"
)

func validateStringField(v internal.Validator, fieldName, expect, actual string) {
	// If an expected value is not set, we assume the user does not want to validate it
	if expect == "" {
		return
	}
	if expect != actual {
		v.Error(fieldName, "incorrect: Expected:", expect, " Got:", actual)
	}
}

type addValidatorField struct {
	field    interface{}
	original internal.Validator
}

func (a addValidatorField) Error(fields ...interface{}) {
	fields = append([]interface{}{a.field}, fields...)
	a.original.Error(fields...)
}

// extendValidator is used to add more context to a validator.
func extendValidator(v internal.Validator, field interface{}) internal.Validator {
	return addValidatorField{
		field:    field,
		original: v,
	}
}

// expectTxnMetrics tests that the app contains metrics for a transaction.
func expectTxnMetrics(t internal.Validator, mt *metricTable, want internal.WantTxn) {
	var metrics []internal.WantMetric
	var scope string
	var allWebOther string
	if want.IsWeb {
		scope = "WebTransaction/Go/" + want.Name
		allWebOther = "allWeb"
		metrics = []internal.WantMetric{
			{Name: "WebTransaction/Go/" + want.Name, Scope: "", Forced: true, Data: nil},
			{Name: "WebTransaction", Scope: "", Forced: true, Data: nil},
			{Name: "WebTransactionTotalTime/Go/" + want.Name, Scope: "", Forced: false, Data: nil},
			{Name: "WebTransactionTotalTime", Scope: "", Forced: true, Data: nil},
			{Name: "HttpDispatcher", Scope: "", Forced: true, Data: nil},
			{Name: "Apdex", Scope: "", Forced: true, Data: nil},
			{Name: "Apdex/Go/" + want.Name, Scope: "", Forced: false, Data: nil},
		}
		if want.UnknownCaller {
			metrics = append(metrics,
				internal.WantMetric{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", Scope: "", Forced: false, Data: nil},
			)
			metrics = append(metrics,
				internal.WantMetric{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/allWeb", Scope: "", Forced: false, Data: nil},
			)
		}
		if want.ErrorByCaller {
			metrics = append(metrics,
				internal.WantMetric{Name: "ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/allWeb", Scope: "", Forced: false, Data: nil},
			)
			metrics = append(metrics,
				internal.WantMetric{Name: "ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/all", Scope: "", Forced: false, Data: nil},
			)
		}
	} else {
		scope = "OtherTransaction/Go/" + want.Name
		allWebOther = "allOther"
		metrics = []internal.WantMetric{
			{Name: "OtherTransaction/Go/" + want.Name, Scope: "", Forced: true, Data: nil},
			{Name: "OtherTransaction/all", Scope: "", Forced: true, Data: nil},
			{Name: "OtherTransactionTotalTime/Go/" + want.Name, Scope: "", Forced: false, Data: nil},
			{Name: "OtherTransactionTotalTime", Scope: "", Forced: true, Data: nil},
		}
		if want.UnknownCaller {
			metrics = append(metrics,
				internal.WantMetric{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/all", Scope: "", Forced: false, Data: nil},
			)
			metrics = append(metrics,
				internal.WantMetric{Name: "DurationByCaller/Unknown/Unknown/Unknown/Unknown/allOther", Scope: "", Forced: false, Data: nil},
			)
		}
		if want.ErrorByCaller {
			metrics = append(metrics,
				internal.WantMetric{Name: "ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/allOther", Scope: "", Forced: false, Data: nil},
			)
			metrics = append(metrics,
				internal.WantMetric{Name: "ErrorsByCaller/Unknown/Unknown/Unknown/Unknown/all", Scope: "", Forced: false, Data: nil},
			)
		}
	}
	if want.NumErrors > 0 {
		data := []float64{float64(want.NumErrors), 0, 0, 0, 0, 0}
		metrics = append(metrics, []internal.WantMetric{
			{Name: "Errors/all", Scope: "", Forced: true, Data: data},
			{Name: "Errors/" + allWebOther, Scope: "", Forced: true, Data: data},
			{Name: "Errors/" + scope, Scope: "", Forced: true, Data: data},
		}...)
	}
	expectMetrics(t, mt, metrics)
}

func expectMetricField(t internal.Validator, id metricID, expect, want float64, fieldName string) {
	if expect != want {
		t.Error("incorrect value for metric", fieldName, id, "expect:", expect, "want: ", want)
	}
}

// expectMetricsPresent allows testing of metrics without requiring an exact match
func expectMetricsPresent(t internal.Validator, mt *metricTable, expect []internal.WantMetric) {
	expectMetricsInternal(t, mt, expect, false)
}

// expectMetrics allows testing of metrics.  It passes if mt exactly matches expect.
func expectMetrics(t internal.Validator, mt *metricTable, expect []internal.WantMetric) {
	expectMetricsInternal(t, mt, expect, true)
}

func expectMetricsInternal(t internal.Validator, mt *metricTable, expect []internal.WantMetric, exactMatch bool) {
	if exactMatch {
		if len(mt.metrics) != len(expect) {
			t.Error("incorrect number of metrics stored, expected:", len(expect), "got:", len(mt.metrics))
		}
	}
	expectedIds := make(map[metricID]struct{})
	for _, e := range expect {
		id := metricID{Name: e.Name, Scope: e.Scope}
		expectedIds[id] = struct{}{}
		m := mt.metrics[id]
		if nil == m {
			t.Error("expected metric not found", id)
			continue
		}

		if b, ok := e.Forced.(bool); ok {
			if b != (forced == m.forced) {
				t.Error("metric forced incorrect", b, m.forced, id)
			}
		}

		if nil != e.Data {
			expectMetricField(t, id, e.Data[0], m.data.countSatisfied, "countSatisfied")

			if len(e.Data) > 1 {
				expectMetricField(t, id, e.Data[1], m.data.totalTolerated, "totalTolerated")
				expectMetricField(t, id, e.Data[2], m.data.exclusiveFailed, "exclusiveFailed")
				expectMetricField(t, id, e.Data[3], m.data.min, "min")
				expectMetricField(t, id, e.Data[4], m.data.max, "max")
				expectMetricField(t, id, e.Data[5], m.data.sumSquares, "sumSquares")
			}
		}
	}
	if exactMatch {
		for id := range mt.metrics {
			if _, ok := expectedIds[id]; !ok {
				t.Error("expected metrics does not contain", id.Name, id.Scope)
			}
		}
	}
}

func expectAttributes(v internal.Validator, exists map[string]interface{}, expect map[string]interface{}) {
	// TODO: This params comparison can be made smarter: Alert differences
	// based on sub/super set behavior.
	if len(exists) != len(expect) {
		v.Error("attributes length difference", len(exists), len(expect))
	}
	for key, expectVal := range expect {
		actualVal, ok := exists[key]
		if !ok {
			v.Error("expected attribute not found: ", key)
			continue
		}
		if expectVal == internal.MatchAnything || expectVal == "*" {
			continue
		}

		actualString := fmt.Sprint(actualVal)
		expectString := fmt.Sprint(expectVal)
		switch expectVal.(type) {
		case float64:
			// json.Number type objects need to be converted into float64 strings
			// when compared against a float64 or the comparison will fail due to
			// the number formatting being different
			if number, ok := actualVal.(json.Number); ok {
				numString, _ := number.Float64()
				actualString = fmt.Sprint(numString)
			}
		}

		if expectString != actualString {
			v.Error(fmt.Sprintf("Values of key \"%s\" do not match; Expect: %s Actual: %s", key, expectString, actualString))
		}
	}
	for key, val := range exists {
		_, ok := expect[key]
		if !ok {
			v.Error("unexpected attribute present: ", key, val)
			continue
		}
	}
}

// expectCustomEvents allows testing of custom events.  It passes if cs exactly matches expect.
func expectCustomEvents(v internal.Validator, cs *customEvents, expect []internal.WantEvent) {
	expectEvents(v, cs.analyticsEvents, expect, nil)
}

func expectLogEvents(v internal.Validator, events *logEvents, expect []internal.WantLog) {
	if len(events.logs) != len(expect) {
		v.Error("actual number of events does not match what is expected", len(events.logs), len(expect))
		return
	}

	for i, e := range expect {
		event := events.logs[i]
		expectLogEvent(v, event, e)
	}
}

func expectLogEvent(v internal.Validator, actual logEvent, want internal.WantLog) {
	if actual.message != want.Message && want.Message != internal.MatchAnyString {
		v.Error(fmt.Sprintf("unexpected log message: got %s, want %s", actual.message, want.Message))
		return
	}
	if actual.severity != want.Severity && want.Severity != internal.MatchAnyString {
		v.Error(fmt.Sprintf("unexpected log severity: got %s, want %s", actual.severity, want.Severity))
		return
	}
	if actual.traceID != want.TraceID && want.TraceID != internal.MatchAnyString {
		v.Error(fmt.Sprintf("unexpected log trace id: got %s, want %s", actual.traceID, want.TraceID))
		return
	}
	if actual.spanID != want.SpanID && want.SpanID != internal.MatchAnyString {
		v.Error(fmt.Sprintf("unexpected log span id: got %s, want %s", actual.spanID, want.SpanID))
		return
	}
	if actual.timestamp != want.Timestamp && want.Timestamp != internal.MatchAnyUnixMilli {
		v.Error(fmt.Sprintf("unexpected log timestamp: got %d, want %d", actual.timestamp, want.Timestamp))
		return
	}
}

func expectEvent(v internal.Validator, e json.Marshaler, expect internal.WantEvent) {
	js, err := e.MarshalJSON()
	if nil != err {
		v.Error("unable to marshal event", err)
		return
	}

	// Because we are unmarshaling into a generic struct without types
	// JSON numbers will be set to the float64 type by default, causing
	// errors when comparing to the expected integer timestamp value.
	decoder := json.NewDecoder(bytes.NewReader(js))
	decoder.UseNumber()
	var event []map[string]interface{}
	err = decoder.Decode(&event)
	if nil != err {
		v.Error("unable to parse event json", err)
		return
	}

	// avoid nil pointer errors or index out of bounds errors
	if event == nil || len(event) == 0 {
		v.Error("Event can not be nil or empty")
		return
	}

	intrinsics := event[0]
	userAttributes := event[1]
	agentAttributes := event[2]

	if nil != expect.Intrinsics {
		expectAttributes(v, intrinsics, expect.Intrinsics)
	}
	if nil != expect.UserAttributes {
		expectAttributes(v, userAttributes, expect.UserAttributes)
	}
	if nil != expect.AgentAttributes {
		expectAttributes(v, agentAttributes, expect.AgentAttributes)
	}
}

func expectEvents(v internal.Validator, events *analyticsEvents, expect []internal.WantEvent, extraAttributes map[string]interface{}) {
	if len(events.events) != len(expect) {
		v.Error("number of events does not match", len(events.events), len(expect))
		return
	}
	for i, e := range expect {
		event, ok := events.events[i].jsonWriter.(json.Marshaler)
		if !ok {
			v.Error("event does not implement json.Marshaler")
			continue
		}
		if nil != e.Intrinsics {
			e.Intrinsics = mergeAttributes(extraAttributes, e.Intrinsics)
		}
		expectEvent(v, event, e)
	}
}

// Second attributes have priority.
func mergeAttributes(a1, a2 map[string]interface{}) map[string]interface{} {
	a := make(map[string]interface{})
	for k, v := range a1 {
		a[k] = v
	}
	for k, v := range a2 {
		a[k] = v
	}
	return a
}

// expectErrorEvents allows testing of error events.  It passes if events exactly matches expect.
func expectErrorEvents(v internal.Validator, events *errorEvents, expect []internal.WantEvent) {
	expectEvents(v, events.analyticsEvents, expect, map[string]interface{}{
		// The following intrinsics should always be present in
		// error events:
		"type":      "TransactionError",
		"timestamp": internal.MatchAnything,
		"duration":  internal.MatchAnything,
	})
}

// expectSpanEvents allows testing of span events.  It passes if events exactly matches expect.
func expectSpanEvents(v internal.Validator, events *spanEvents, expect []internal.WantEvent) {
	extraAttrs := map[string]interface{}{
		// The following intrinsics should always be present in
		// span events:
		"type":          "Span",
		"timestamp":     internal.MatchAnything,
		"duration":      internal.MatchAnything,
		"traceId":       internal.MatchAnything,
		"guid":          internal.MatchAnything,
		"transactionId": internal.MatchAnything,
		// All span events are currently sampled.
		"sampled":  true,
		"priority": internal.MatchAnything,
	}
	expectEvents(v, events.analyticsEvents, expect, extraAttrs)
	expectObserverEvents(v, events.analyticsEvents, expect, extraAttrs)
}

// expectTxnEvents allows testing of txn events.
func expectTxnEvents(v internal.Validator, events *txnEvents, expect []internal.WantEvent) {
	expectEvents(v, events.analyticsEvents, expect, map[string]interface{}{
		// The following intrinsics should always be present in
		// txn events:
		"type":      "Transaction",
		"timestamp": internal.MatchAnything,
		"duration":  internal.MatchAnything,
		"totalTime": internal.MatchAnything,
		"error":     internal.MatchAnything,
	})
}

func expectError(v internal.Validator, err *tracedError, expect internal.WantError) {
	validateStringField(v, "txnName", expect.TxnName, err.FinalName)
	validateStringField(v, "klass", expect.Klass, err.Klass)
	validateStringField(v, "msg", expect.Msg, err.Msg)
	js, errr := err.MarshalJSON()
	if nil != errr {
		v.Error("unable to marshal error json", errr)
		return
	}
	var unmarshalled []interface{}
	errr = json.Unmarshal(js, &unmarshalled)
	if nil != errr {
		v.Error("unable to unmarshal error json", errr)
		return
	}
	attributes := unmarshalled[4].(map[string]interface{})
	agentAttributes := attributes["agentAttributes"].(map[string]interface{})
	userAttributes := attributes["userAttributes"].(map[string]interface{})

	if nil != expect.UserAttributes {
		expectAttributes(v, userAttributes, expect.UserAttributes)
	}
	if nil != expect.AgentAttributes {
		expectAttributes(v, agentAttributes, expect.AgentAttributes)
	}
	if stack := attributes["stack_trace"]; nil == stack {
		v.Error("missing error stack trace")
	}
}

// expectErrors allows testing of errors.
func expectErrors(v internal.Validator, errors harvestErrors, expect []internal.WantError) {
	if len(errors) != len(expect) {
		v.Error("number of errors mismatch", len(errors), len(expect))
		return
	}
	for i, e := range expect {
		expectError(v, errors[i], e)
	}
}

func countSegments(node []interface{}) int {
	count := 1
	children := node[4].([]interface{})
	for _, c := range children {
		node := c.([]interface{})
		count += countSegments(node)
	}
	return count
}

func expectTraceSegment(v internal.Validator, nodeObj interface{}, expect internal.WantTraceSegment) {
	node := nodeObj.([]interface{})
	start := int(node[0].(float64))
	stop := int(node[1].(float64))
	name := node[2].(string)
	attributes := node[3].(map[string]interface{})
	children := node[4].([]interface{})

	validateStringField(v, "segmentName", expect.SegmentName, name)
	if nil != expect.RelativeStartMillis {
		expectStart, ok := expect.RelativeStartMillis.(int)
		if !ok {
			v.Error("invalid expect.RelativeStartMillis", expect.RelativeStartMillis)
		} else if expectStart != start {
			v.Error("segmentStartTime", expect.SegmentName, start, expectStart)
		}
	}
	if nil != expect.RelativeStopMillis {
		expectStop, ok := expect.RelativeStopMillis.(int)
		if !ok {
			v.Error("invalid expect.RelativeStopMillis", expect.RelativeStopMillis)
		} else if expectStop != stop {
			v.Error("segmentStopTime", expect.SegmentName, stop, expectStop)
		}
	}
	if nil != expect.Attributes {
		expectAttributes(v, attributes, expect.Attributes)
	}
	if len(children) != len(expect.Children) {
		v.Error("segmentChildrenCount", expect.SegmentName, len(children), len(expect.Children))
	} else {
		for idx, child := range children {
			expectTraceSegment(v, child, expect.Children[idx])
		}
	}
}

func expectTxnTrace(v internal.Validator, got interface{}, expect internal.WantTxnTrace) {
	unmarshalled := got.([]interface{})
	duration := unmarshalled[1].(float64)
	name := unmarshalled[2].(string)
	var arrayURL string
	if nil != unmarshalled[3] {
		arrayURL = unmarshalled[3].(string)
	}
	traceData := unmarshalled[4].([]interface{})

	rootNode := traceData[3].([]interface{})
	attributes := traceData[4].(map[string]interface{})
	userAttributes := attributes["userAttributes"].(map[string]interface{})
	agentAttributes := attributes["agentAttributes"].(map[string]interface{})
	intrinsics := attributes["intrinsics"].(map[string]interface{})

	validateStringField(v, "metric name", expect.MetricName, name)

	if d := expect.DurationMillis; nil != d && *d != duration {
		v.Error("incorrect trace duration millis", *d, duration)
	}

	if nil != expect.UserAttributes {
		expectAttributes(v, userAttributes, expect.UserAttributes)
	}
	if nil != expect.AgentAttributes {
		expectAttributes(v, agentAttributes, expect.AgentAttributes)
		expectURL, _ := expect.AgentAttributes["request.uri"].(string)
		if "" != expectURL {
			validateStringField(v, "request url in array", expectURL, arrayURL)
		}
	}
	if nil != expect.Intrinsics {
		expectAttributes(v, intrinsics, expect.Intrinsics)
	}
	if expect.Root.SegmentName != "" {
		expectTraceSegment(v, rootNode, expect.Root)
	} else {
		numSegments := countSegments(rootNode)
		// The expectation segment count does not include the two root nodes.
		numSegments -= 2
		if expect.NumSegments != numSegments {
			v.Error("wrong number of segments", expect.NumSegments, numSegments)
		}
	}
}

// expectTxnTraces allows testing of transaction traces.
func expectTxnTraces(v internal.Validator, traces *harvestTraces, want []internal.WantTxnTrace) {
	if len(want) != traces.Len() {
		v.Error("number of traces do not match", len(want), traces.Len())
		return
	}
	if len(want) == 0 {
		return
	}
	js, err := traces.Data("agentRunID", time.Now())
	if nil != err {
		v.Error("error creasing harvest traces data", err)
		return
	}

	var unmarshalled []interface{}
	err = json.Unmarshal(js, &unmarshalled)
	if nil != err {
		v.Error("unable to unmarshal error json", err)
		return
	}
	if "agentRunID" != unmarshalled[0].(string) {
		v.Error("traces agent run id wrong", unmarshalled[0])
		return
	}
	gotTraces := unmarshalled[1].([]interface{})
	if len(gotTraces) != len(want) {
		v.Error("number of traces in json does not match", len(gotTraces), len(want))
		return
	}
	for i, expected := range want {
		expectTxnTrace(v, gotTraces[i], expected)
	}
}

func expectSlowQuery(t internal.Validator, slowQuery *slowQuery, want internal.WantSlowQuery) {
	if slowQuery.Count != want.Count {
		t.Error("wrong Count field", slowQuery.Count, want.Count)
	}
	uri, _ := slowQuery.txnEvent.Attrs.GetAgentValue(AttributeRequestURI, destTxnTrace)
	validateStringField(t, "MetricName", want.MetricName, slowQuery.DatastoreMetric)
	validateStringField(t, "Query", want.Query, slowQuery.ParameterizedQuery)
	validateStringField(t, "TxnEvent.FinalName", want.TxnName, slowQuery.txnEvent.FinalName)
	validateStringField(t, "request.uri", want.TxnURL, uri)
	validateStringField(t, "DatabaseName", want.DatabaseName, slowQuery.DatabaseName)
	validateStringField(t, "Host", want.Host, slowQuery.Host)
	validateStringField(t, "PortPathOrID", want.PortPathOrID, slowQuery.PortPathOrID)
	expectAttributes(t, map[string]interface{}(slowQuery.QueryParameters), want.Params)
}

// expectSlowQueries allows testing of slow queries.
func expectSlowQueries(t internal.Validator, slowQueries *slowQueries, want []internal.WantSlowQuery) {
	if len(want) != len(slowQueries.priorityQueue) {
		t.Error("wrong number of slow queries",
			"expected", len(want), "got", len(slowQueries.priorityQueue))
		return
	}
	for _, s := range want {
		idx, ok := slowQueries.lookup[s.Query]
		if !ok {
			t.Error("unable to find slow query", s.Query)
			continue
		}
		expectSlowQuery(t, slowQueries.priorityQueue[idx], s)
	}
}

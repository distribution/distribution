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
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// spanNameQueryField is the header for span name.
	spanNameQueryField = "zspanname"
	// spanTypeQueryField is the header for type (running = 0, latency = 1, error = 2) to display.
	spanTypeQueryField = "ztype"
	// spanLatencyBucketQueryField is the header for latency based samples.
	// Default is [0, 8] representing the latency buckets, where 0 is the first one.
	spanLatencyBucketQueryField = "zlatencybucket"
	// maxTraceMessageLength is the maximum length of a message in tracez output.
	maxTraceMessageLength = 1024
)

type summaryTableData struct {
	Header             []string
	LatencyBucketNames []string
	Links              bool
	TracesEndpoint     string
	Rows               []summaryTableRowData
}

type summaryTableRowData struct {
	Name    string
	Active  int
	Latency []int
	Errors  int
}

// traceTableData contains data for the trace data template.
type traceTableData struct {
	Name string
	Num  int
	Rows []spanRow
}

var _ http.Handler = (*tracezHandler)(nil)

type tracezHandler struct {
	sp *SpanProcessor
}

// NewTracezHandler returns an http.Handler that can be used to serve HTTP requests for trace zpages.
func NewTracezHandler(sp *SpanProcessor) http.Handler {
	return &tracezHandler{sp: sp}
}

// ServeHTTP implements the http.Handler and is capable of serving "tracez" HTTP requests.
func (th *tracezHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	spanName := r.Form.Get(spanNameQueryField)
	spanType, _ := strconv.Atoi(r.Form.Get(spanTypeQueryField))
	spanSubtype, _ := strconv.Atoi(r.Form.Get(spanLatencyBucketQueryField))

	if err := headerTemplate.Execute(w, headerData{Title: "Trace Spans"}); err != nil {
		log.Printf("zpages: executing template: %v", err)
	}
	if err := summaryTableTemplate.Execute(w, th.getSummaryTableData()); err != nil {
		log.Printf("zpages: executing template: %v", err)
	}
	if spanName != "" {
		if err := tracesTableTemplate.Execute(w, th.getTraceTableData(spanName, spanType, spanSubtype)); err != nil {
			log.Printf("zpages: executing template: %v", err)
		}
	}
	if err := footerTemplate.Execute(w, nil); err != nil {
		log.Printf("zpages: executing template: %v", err)
	}
}

func (th *tracezHandler) getTraceTableData(spanName string, spanType, latencyBucket int) traceTableData {
	var spans []sdktrace.ReadOnlySpan
	switch spanType {
	case 0: // active
		spans = th.sp.activeSpans(spanName)
	case 1: // latency
		spans = th.sp.spansByLatency(spanName, latencyBucket)
	case 2: // error
		spans = th.sp.errorSpans(spanName)
	}

	data := traceTableData{
		Name: spanName,
		Num:  len(spans),
	}
	for _, s := range spans {
		data.Rows = append(data.Rows, spanRows(s)...)
	}
	return data
}

func (th *tracezHandler) getSummaryTableData() summaryTableData {
	data := summaryTableData{
		Links:          true,
		TracesEndpoint: "tracez",
	}
	data.Header = []string{"Name", "active"}
	// An implicit 0 lower bound latency bucket is always present.
	latencyBuckets := append([]time.Duration{0}, defaultBoundaries.durations...)
	for _, l := range latencyBuckets {
		s := fmt.Sprintf(">%v", l)
		data.Header = append(data.Header, s)
		data.LatencyBucketNames = append(data.LatencyBucketNames, s)
	}
	data.Header = append(data.Header, "Errors")
	for name, s := range th.sp.spansPerMethod() {
		row := summaryTableRowData{Name: name, Active: s.activeSpans, Errors: s.errorSpans, Latency: s.latencySpans}
		data.Rows = append(data.Rows, row)
	}
	sort.Slice(data.Rows, func(i, j int) bool {
		return data.Rows[i].Name < data.Rows[j].Name
	})
	return data
}

type spanRow struct {
	Fields [3]string
	trace.SpanContext
	ParentSpanContext trace.SpanContext
}

type events []sdktrace.Event

func (e events) Len() int { return len(e) }
func (e events) Less(i, j int) bool {
	return e[i].Time.Before(e[j].Time)
}
func (e events) Swap(i, j int) { e[i], e[j] = e[j], e[i] }

type attributes []attribute.KeyValue

func (e attributes) Len() int { return len(e) }
func (e attributes) Less(i, j int) bool {
	return string(e[i].Key) < string(e[j].Key)
}
func (e attributes) Swap(i, j int) { e[i], e[j] = e[j], e[i] }

func spanRows(s sdktrace.ReadOnlySpan) []spanRow {
	start := s.StartTime()

	lasty, lastm, lastd := start.Date()
	wholeTime := func(t time.Time) string {
		return t.Format("2006/01/02-15:04:05") + fmt.Sprintf(".%06d", t.Nanosecond()/1000)
	}
	formatTime := func(t time.Time) string {
		y, m, d := t.Date()
		if y == lasty && m == lastm && d == lastd {
			return t.Format("           15:04:05") + fmt.Sprintf(".%06d", t.Nanosecond()/1000)
		}
		lasty, lastm, lastd = y, m, d
		return wholeTime(t)
	}

	lastTime := start
	formatElapsed := func(t time.Time) string {
		d := t.Sub(lastTime)
		lastTime = t
		u := int64(d / 1000)
		// There are five cases for duration printing:
		// -1234567890s
		// -1234.123456
		//      .123456
		// 12345.123456
		// 12345678901s
		switch {
		case u < -9999999999:
			return fmt.Sprintf("%11ds", u/1e6)
		case u < 0:
			sec := u / 1e6
			u -= sec * 1e6
			return fmt.Sprintf("%5d.%06d", sec, -u)
		case u < 1e6:
			return fmt.Sprintf("     .%6d", u)
		case u <= 99999999999:
			sec := u / 1e6
			u -= sec * 1e6
			return fmt.Sprintf("%5d.%06d", sec, u)
		default:
			return fmt.Sprintf("%11ds", u/1e6)
		}
	}

	firstRow := spanRow{Fields: [3]string{wholeTime(start), "", ""}, SpanContext: s.SpanContext(), ParentSpanContext: s.Parent()}
	if s.EndTime().IsZero() {
		firstRow.Fields[1] = "            "
	} else {
		firstRow.Fields[1] = formatElapsed(s.EndTime())
		lastTime = start
	}
	out := []spanRow{firstRow}

	formatAttributes := func(a attributes) string {
		sort.Sort(a)
		var s []string
		for i := range a {
			s = append(s, fmt.Sprintf("%s=%v", a[i].Key, a[i].Value.Emit()))
		}
		return "Attributes:{" + strings.Join(s, ", ") + "}"
	}

	msg := fmt.Sprintf("Status{Code=%s, description=%q}", s.Status().Code.String(), s.Status().Description)
	out = append(out, spanRow{Fields: [3]string{"", "", msg}})

	if len(s.Attributes()) != 0 {
		out = append(out, spanRow{Fields: [3]string{"", "", formatAttributes(s.Attributes())}})
	}

	es := events(s.Events())
	sort.Sort(es)
	for _, e := range es {
		msg := e.Name
		if len(e.Attributes) != 0 {
			msg = msg + "  " + formatAttributes(e.Attributes)
		}
		row := spanRow{Fields: [3]string{
			formatTime(e.Time),
			formatElapsed(e.Time),
			msg,
		}}
		out = append(out, row)
	}
	for i := range out {
		if len(out[i].Fields[2]) > maxTraceMessageLength {
			out[i].Fields[2] = out[i].Fields[2][:maxTraceMessageLength]
		}
	}
	return out
}

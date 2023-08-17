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

package otlptext // import "go.opentelemetry.io/collector/exporter/loggingexporter/internal/otlptext"

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type dataBuffer struct {
	buf bytes.Buffer
}

func (b *dataBuffer) logEntry(format string, a ...interface{}) {
	b.buf.WriteString(fmt.Sprintf(format, a...))
	b.buf.WriteString("\n")
}

func (b *dataBuffer) logAttr(label string, value string) {
	b.logEntry("    %-15s: %s", label, value)
}

func (b *dataBuffer) logAttributes(label string, m pcommon.Map) {
	if m.Len() == 0 {
		return
	}

	b.logEntry("%s:", label)
	m.Range(func(k string, v pcommon.Value) bool {
		b.logEntry("     -> %s: %s(%s)", k, v.Type().String(), attributeValueToString(v))
		return true
	})
}

func (b *dataBuffer) logInstrumentationScope(il pcommon.InstrumentationScope) {
	b.logEntry(
		"InstrumentationScope %s %s",
		il.Name(),
		il.Version())
}

func (b *dataBuffer) logMetricDescriptor(md pmetric.Metric) {
	b.logEntry("Descriptor:")
	b.logEntry("     -> Name: %s", md.Name())
	b.logEntry("     -> Description: %s", md.Description())
	b.logEntry("     -> Unit: %s", md.Unit())
	b.logEntry("     -> DataType: %s", md.DataType().String())
}

func (b *dataBuffer) logMetricDataPoints(m pmetric.Metric) {
	switch m.DataType() {
	case pmetric.MetricDataTypeNone:
		return
	case pmetric.MetricDataTypeGauge:
		b.logNumberDataPoints(m.Gauge().DataPoints())
	case pmetric.MetricDataTypeSum:
		data := m.Sum()
		b.logEntry("     -> IsMonotonic: %t", data.IsMonotonic())
		b.logEntry("     -> AggregationTemporality: %s", data.AggregationTemporality().String())
		b.logNumberDataPoints(data.DataPoints())
	case pmetric.MetricDataTypeHistogram:
		data := m.Histogram()
		b.logEntry("     -> AggregationTemporality: %s", data.AggregationTemporality().String())
		b.logHistogramDataPoints(data.DataPoints())
	case pmetric.MetricDataTypeExponentialHistogram:
		data := m.ExponentialHistogram()
		b.logEntry("     -> AggregationTemporality: %s", data.AggregationTemporality().String())
		b.logExponentialHistogramDataPoints(data.DataPoints())
	case pmetric.MetricDataTypeSummary:
		data := m.Summary()
		b.logDoubleSummaryDataPoints(data.DataPoints())
	}
}

func (b *dataBuffer) logNumberDataPoints(ps pmetric.NumberDataPointSlice) {
	for i := 0; i < ps.Len(); i++ {
		p := ps.At(i)
		b.logEntry("NumberDataPoints #%d", i)
		b.logDataPointAttributes(p.Attributes())

		b.logEntry("StartTimestamp: %s", p.StartTimestamp())
		b.logEntry("Timestamp: %s", p.Timestamp())
		switch p.ValueType() {
		case pmetric.NumberDataPointValueTypeInt:
			b.logEntry("Value: %d", p.IntVal())
		case pmetric.NumberDataPointValueTypeDouble:
			b.logEntry("Value: %f", p.DoubleVal())
		}
	}
}

func (b *dataBuffer) logHistogramDataPoints(ps pmetric.HistogramDataPointSlice) {
	for i := 0; i < ps.Len(); i++ {
		p := ps.At(i)
		b.logEntry("HistogramDataPoints #%d", i)
		b.logDataPointAttributes(p.Attributes())

		b.logEntry("StartTimestamp: %s", p.StartTimestamp())
		b.logEntry("Timestamp: %s", p.Timestamp())
		b.logEntry("Count: %d", p.Count())

		if p.HasSum() {
			b.logEntry("Sum: %f", p.Sum())
		}

		if p.HasMin() {
			b.logEntry("Min: %f", p.Min())
		}

		if p.HasMax() {
			b.logEntry("Max: %f", p.Max())
		}

		for i := 0; i < p.ExplicitBounds().Len(); i++ {
			b.logEntry("ExplicitBounds #%d: %f", i, p.ExplicitBounds().At(i))
		}

		for j := 0; j < p.BucketCounts().Len(); j++ {
			b.logEntry("Buckets #%d, Count: %d", j, p.BucketCounts().At(j))
		}
	}
}

func (b *dataBuffer) logExponentialHistogramDataPoints(ps pmetric.ExponentialHistogramDataPointSlice) {
	for i := 0; i < ps.Len(); i++ {
		p := ps.At(i)
		b.logEntry("ExponentialHistogramDataPoints #%d", i)
		b.logDataPointAttributes(p.Attributes())

		b.logEntry("StartTimestamp: %s", p.StartTimestamp())
		b.logEntry("Timestamp: %s", p.Timestamp())
		b.logEntry("Count: %d", p.Count())

		if p.HasSum() {
			b.logEntry("Sum: %f", p.Sum())
		}

		if p.HasMin() {
			b.logEntry("Min: %f", p.Min())
		}

		if p.HasMax() {
			b.logEntry("Max: %f", p.Max())
		}

		scale := int(p.Scale())
		factor := math.Ldexp(math.Ln2, -scale)
		// Note: the equation used here, which is
		//   math.Exp(index * factor)
		// reports +Inf as the _lower_ boundary of the bucket nearest
		// infinity, which is incorrect and can be addressed in various
		// ways.  The OTel-Go implementation of this histogram pending
		// in https://github.com/open-telemetry/opentelemetry-go/pull/2393
		// uses a lookup table for the last finite boundary, which can be
		// easily computed using `math/big` (for scales up to 20).

		negB := p.Negative().BucketCounts()
		posB := p.Positive().BucketCounts()

		for i := 0; i < negB.Len(); i++ {
			pos := negB.Len() - i - 1
			index := p.Negative().Offset() + int32(pos)
			lower := math.Exp(float64(index) * factor)
			upper := math.Exp(float64(index+1) * factor)
			b.logEntry("Bucket (%f, %f], Count: %d", -upper, -lower, negB.At(pos))
		}

		if p.ZeroCount() != 0 {
			b.logEntry("Bucket [0, 0], Count: %d", p.ZeroCount())
		}

		for pos := 0; pos < posB.Len(); pos++ {
			index := p.Positive().Offset() + int32(pos)
			lower := math.Exp(float64(index) * factor)
			upper := math.Exp(float64(index+1) * factor)
			b.logEntry("Bucket [%f, %f), Count: %d", lower, upper, posB.At(pos))
		}
	}
}

func (b *dataBuffer) logDoubleSummaryDataPoints(ps pmetric.SummaryDataPointSlice) {
	for i := 0; i < ps.Len(); i++ {
		p := ps.At(i)
		b.logEntry("SummaryDataPoints #%d", i)
		b.logDataPointAttributes(p.Attributes())

		b.logEntry("StartTimestamp: %s", p.StartTimestamp())
		b.logEntry("Timestamp: %s", p.Timestamp())
		b.logEntry("Count: %d", p.Count())
		b.logEntry("Sum: %f", p.Sum())

		quantiles := p.QuantileValues()
		for i := 0; i < quantiles.Len(); i++ {
			quantile := quantiles.At(i)
			b.logEntry("QuantileValue #%d: Quantile %f, Value %f", i, quantile.Quantile(), quantile.Value())
		}
	}
}

func (b *dataBuffer) logDataPointAttributes(labels pcommon.Map) {
	b.logAttributes("Data point attributes", labels)
}

func (b *dataBuffer) logEvents(description string, se ptrace.SpanEventSlice) {
	if se.Len() == 0 {
		return
	}

	b.logEntry("%s:", description)
	for i := 0; i < se.Len(); i++ {
		e := se.At(i)
		b.logEntry("SpanEvent #%d", i)
		b.logEntry("     -> Name: %s", e.Name())
		b.logEntry("     -> Timestamp: %s", e.Timestamp())
		b.logEntry("     -> DroppedAttributesCount: %d", e.DroppedAttributesCount())

		if e.Attributes().Len() == 0 {
			continue
		}
		b.logEntry("     -> Attributes:")
		e.Attributes().Range(func(k string, v pcommon.Value) bool {
			b.logEntry("         -> %s: %s(%s)", k, v.Type().String(), attributeValueToString(v))
			return true
		})
	}
}

func (b *dataBuffer) logLinks(description string, sl ptrace.SpanLinkSlice) {
	if sl.Len() == 0 {
		return
	}

	b.logEntry("%s:", description)

	for i := 0; i < sl.Len(); i++ {
		l := sl.At(i)
		b.logEntry("SpanLink #%d", i)
		b.logEntry("     -> Trace ID: %s", l.TraceID().HexString())
		b.logEntry("     -> ID: %s", l.SpanID().HexString())
		b.logEntry("     -> TraceState: %s", l.TraceState())
		b.logEntry("     -> DroppedAttributesCount: %d", l.DroppedAttributesCount())
		if l.Attributes().Len() == 0 {
			continue
		}
		b.logEntry("     -> Attributes:")
		l.Attributes().Range(func(k string, v pcommon.Value) bool {
			b.logEntry("         -> %s: %s(%s)", k, v.Type().String(), attributeValueToString(v))
			return true
		})
	}
}

func attributeValueToString(v pcommon.Value) string {
	switch v.Type() {
	case pcommon.ValueTypeString:
		return v.StringVal()
	case pcommon.ValueTypeBool:
		return strconv.FormatBool(v.BoolVal())
	case pcommon.ValueTypeDouble:
		return strconv.FormatFloat(v.DoubleVal(), 'f', -1, 64)
	case pcommon.ValueTypeInt:
		return strconv.FormatInt(v.IntVal(), 10)
	case pcommon.ValueTypeSlice:
		return sliceToString(v.SliceVal())
	case pcommon.ValueTypeMap:
		return mapToString(v.MapVal())
	default:
		return fmt.Sprintf("<Unknown OpenTelemetry attribute value type %q>", v.Type())
	}
}

func sliceToString(s pcommon.Slice) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 0; i < s.Len(); i++ {
		if i < s.Len()-1 {
			fmt.Fprintf(&b, "%s, ", attributeValueToString(s.At(i)))
		} else {
			b.WriteString(attributeValueToString(s.At(i)))
		}
	}

	b.WriteByte(']')
	return b.String()
}

func mapToString(m pcommon.Map) string {
	var b strings.Builder
	b.WriteString("{\n")

	m.Sort().Range(func(k string, v pcommon.Value) bool {
		fmt.Fprintf(&b, "     -> %s: %s(%s)\n", k, v.Type(), v.AsString())
		return true
	})
	b.WriteByte('}')
	return b.String()
}

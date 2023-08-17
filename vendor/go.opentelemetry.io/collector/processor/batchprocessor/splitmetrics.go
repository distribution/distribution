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

package batchprocessor // import "go.opentelemetry.io/collector/processor/batchprocessor"

import (
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// splitMetrics removes metrics from the input data and returns a new data of the specified size.
func splitMetrics(size int, src pmetric.Metrics) pmetric.Metrics {
	dataPoints := src.DataPointCount()
	if dataPoints <= size {
		return src
	}
	totalCopiedDataPoints := 0
	dest := pmetric.NewMetrics()

	src.ResourceMetrics().RemoveIf(func(srcRs pmetric.ResourceMetrics) bool {
		// If we are done skip everything else.
		if totalCopiedDataPoints == size {
			return false
		}

		// If it fully fits
		srcRsDataPointCount := resourceMetricsDPC(srcRs)
		if (totalCopiedDataPoints + srcRsDataPointCount) <= size {
			totalCopiedDataPoints += srcRsDataPointCount
			srcRs.MoveTo(dest.ResourceMetrics().AppendEmpty())
			return true
		}

		destRs := dest.ResourceMetrics().AppendEmpty()
		srcRs.Resource().CopyTo(destRs.Resource())
		srcRs.ScopeMetrics().RemoveIf(func(srcIlm pmetric.ScopeMetrics) bool {
			// If we are done skip everything else.
			if totalCopiedDataPoints == size {
				return false
			}

			// If possible to move all metrics do that.
			srcIlmDataPointCount := scopeMetricsDPC(srcIlm)
			if srcIlmDataPointCount+totalCopiedDataPoints <= size {
				totalCopiedDataPoints += srcIlmDataPointCount
				srcIlm.MoveTo(destRs.ScopeMetrics().AppendEmpty())
				return true
			}

			destIlm := destRs.ScopeMetrics().AppendEmpty()
			srcIlm.Scope().CopyTo(destIlm.Scope())
			srcIlm.Metrics().RemoveIf(func(srcMetric pmetric.Metric) bool {
				// If we are done skip everything else.
				if totalCopiedDataPoints == size {
					return false
				}

				// If possible to move all points do that.
				srcMetricDataPointCount := metricDPC(srcMetric)
				if srcMetricDataPointCount+totalCopiedDataPoints <= size {
					totalCopiedDataPoints += srcMetricDataPointCount
					srcMetric.MoveTo(destIlm.Metrics().AppendEmpty())
					return true
				}

				// If the metric has more data points than free slots we should split it.
				copiedDataPoints, remove := splitMetric(srcMetric, destIlm.Metrics().AppendEmpty(), size-totalCopiedDataPoints)
				totalCopiedDataPoints += copiedDataPoints
				return remove
			})
			return false
		})
		return srcRs.ScopeMetrics().Len() == 0
	})

	return dest
}

// resourceMetricsDPC calculates the total number of data points in the pmetric.ResourceMetrics.
func resourceMetricsDPC(rs pmetric.ResourceMetrics) int {
	dataPointCount := 0
	ilms := rs.ScopeMetrics()
	for k := 0; k < ilms.Len(); k++ {
		dataPointCount += scopeMetricsDPC(ilms.At(k))
	}
	return dataPointCount
}

// scopeMetricsDPC calculates the total number of data points in the pmetric.ScopeMetrics.
func scopeMetricsDPC(ilm pmetric.ScopeMetrics) int {
	dataPointCount := 0
	ms := ilm.Metrics()
	for k := 0; k < ms.Len(); k++ {
		dataPointCount += metricDPC(ms.At(k))
	}
	return dataPointCount
}

// metricDPC calculates the total number of data points in the pmetric.Metric.
func metricDPC(ms pmetric.Metric) int {
	switch ms.DataType() {
	case pmetric.MetricDataTypeGauge:
		return ms.Gauge().DataPoints().Len()
	case pmetric.MetricDataTypeSum:
		return ms.Sum().DataPoints().Len()
	case pmetric.MetricDataTypeHistogram:
		return ms.Histogram().DataPoints().Len()
	case pmetric.MetricDataTypeExponentialHistogram:
		return ms.ExponentialHistogram().DataPoints().Len()
	case pmetric.MetricDataTypeSummary:
		return ms.Summary().DataPoints().Len()
	}
	return 0
}

// splitMetric removes metric points from the input data and moves data of the specified size to destination.
// Returns size of moved data and boolean describing, whether the metric should be removed from original slice.
func splitMetric(ms, dest pmetric.Metric, size int) (int, bool) {
	dest.SetDataType(ms.DataType())
	dest.SetName(ms.Name())
	dest.SetDescription(ms.Description())
	dest.SetUnit(ms.Unit())

	switch ms.DataType() {
	case pmetric.MetricDataTypeGauge:
		return splitNumberDataPoints(ms.Gauge().DataPoints(), dest.Gauge().DataPoints(), size)
	case pmetric.MetricDataTypeSum:
		dest.Sum().SetAggregationTemporality(ms.Sum().AggregationTemporality())
		dest.Sum().SetIsMonotonic(ms.Sum().IsMonotonic())
		return splitNumberDataPoints(ms.Sum().DataPoints(), dest.Sum().DataPoints(), size)
	case pmetric.MetricDataTypeHistogram:
		dest.Histogram().SetAggregationTemporality(ms.Histogram().AggregationTemporality())
		return splitHistogramDataPoints(ms.Histogram().DataPoints(), dest.Histogram().DataPoints(), size)
	case pmetric.MetricDataTypeExponentialHistogram:
		dest.ExponentialHistogram().SetAggregationTemporality(ms.ExponentialHistogram().AggregationTemporality())
		return splitExponentialHistogramDataPoints(ms.ExponentialHistogram().DataPoints(), dest.ExponentialHistogram().DataPoints(), size)
	case pmetric.MetricDataTypeSummary:
		return splitSummaryDataPoints(ms.Summary().DataPoints(), dest.Summary().DataPoints(), size)
	}
	return size, false
}

func splitNumberDataPoints(src, dst pmetric.NumberDataPointSlice, size int) (int, bool) {
	dst.EnsureCapacity(size)
	i := 0
	src.RemoveIf(func(dp pmetric.NumberDataPoint) bool {
		if i < size {
			dp.MoveTo(dst.AppendEmpty())
			i++
			return true
		}
		return false
	})
	return size, false
}

func splitHistogramDataPoints(src, dst pmetric.HistogramDataPointSlice, size int) (int, bool) {
	dst.EnsureCapacity(size)
	i := 0
	src.RemoveIf(func(dp pmetric.HistogramDataPoint) bool {
		if i < size {
			dp.MoveTo(dst.AppendEmpty())
			i++
			return true
		}
		return false
	})
	return size, false
}

func splitExponentialHistogramDataPoints(src, dst pmetric.ExponentialHistogramDataPointSlice, size int) (int, bool) {
	dst.EnsureCapacity(size)
	i := 0
	src.RemoveIf(func(dp pmetric.ExponentialHistogramDataPoint) bool {
		if i < size {
			dp.MoveTo(dst.AppendEmpty())
			i++
			return true
		}
		return false
	})
	return size, false
}

func splitSummaryDataPoints(src, dst pmetric.SummaryDataPointSlice, size int) (int, bool) {
	dst.EnsureCapacity(size)
	i := 0
	src.RemoveIf(func(dp pmetric.SummaryDataPoint) bool {
		if i < size {
			dp.MoveTo(dst.AppendEmpty())
			i++
			return true
		}
		return false
	})
	return size, false
}

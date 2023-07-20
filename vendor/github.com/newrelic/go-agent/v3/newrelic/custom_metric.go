// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

// customMetric is a custom metric.
type customMetric struct {
	RawInputName string
	Value        float64
}

// MergeIntoHarvest implements Harvestable.
func (m customMetric) MergeIntoHarvest(h *harvest) {
	h.Metrics.addValue(customMetricName(m.RawInputName), "", m.Value, unforced)
}

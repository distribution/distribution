// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"github.com/prometheus/client_golang/prometheus"
)

type CacheMetrics struct {
	CacheLength    prometheus.Gauge
	CacheGetsTotal prometheus.Counter
	CacheHitsTotal prometheus.Counter
}

func NewCacheMetrics(reg prometheus.Registerer) *CacheMetrics {
	var m CacheMetrics

	m.CacheLength = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "statsd_metric_mapper_cache_length",
			Help: "The count of unique metrics currently cached.",
		},
	)
	m.CacheGetsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_metric_mapper_cache_gets_total",
			Help: "The count of total metric cache gets.",
		},
	)
	m.CacheHitsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "statsd_metric_mapper_cache_hits_total",
			Help: "The count of total metric cache hits.",
		},
	)

	if reg != nil {
		reg.MustRegister(m.CacheLength)
		reg.MustRegister(m.CacheGetsTotal)
		reg.MustRegister(m.CacheHitsTotal)
	}
	return &m
}

type MetricMapperCacheResult struct {
	Mapping *MetricMapping
	Matched bool
	Labels  prometheus.Labels
}

// MetricMapperCache MUST be thread-safe and should be instrumented with CacheMetrics
type MetricMapperCache interface {
	// Get a cached result
	Get(metricKey string) (interface{}, bool)
	// Add a statsd MetricMapperResult to the cache
	Add(metricKey string, result interface{}) // Add an item to the cache
	// Reset clears the cache for config reloads
	Reset()
}

func formatKey(metricString string, metricType MetricType) string {
	return string(metricType) + "." + metricString
}

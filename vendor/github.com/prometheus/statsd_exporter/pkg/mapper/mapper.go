// Copyright 2013 The Prometheus Authors
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
	"fmt"
	"io/ioutil"
	"regexp"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	yaml "gopkg.in/yaml.v2"

	"github.com/prometheus/statsd_exporter/pkg/mapper/fsm"
)

var (
	// The first segment of a match cannot start with a number
	statsdMetricRE = `[a-zA-Z_](-?[a-zA-Z0-9_])*`
	// The subsequent segments of a match can start with a number
	// See https://github.com/prometheus/statsd_exporter/issues/328
	statsdMetricSubsequentRE = `[a-zA-Z0-9_](-?[a-zA-Z0-9_])*`
	templateReplaceRE        = `(\$\{?\d+\}?)`

	metricLineRE = regexp.MustCompile(`^(\*|` + statsdMetricRE + `)(\.\*|\.` + statsdMetricSubsequentRE + `)*$`)
	metricNameRE = regexp.MustCompile(`^([a-zA-Z_]|` + templateReplaceRE + `)([a-zA-Z0-9_]|` + templateReplaceRE + `)*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]+$`)
)

type MetricMapper struct {
	Registerer prometheus.Registerer
	Defaults   mapperConfigDefaults `yaml:"defaults"`
	Mappings   []MetricMapping      `yaml:"mappings"`
	FSM        *fsm.FSM
	doFSM      bool
	doRegex    bool
	cache      MetricMapperCache
	mutex      sync.RWMutex

	MappingsCount prometheus.Gauge

	Logger log.Logger
}

type SummaryOptions struct {
	Quantiles  []metricObjective `yaml:"quantiles"`
	MaxAge     time.Duration     `yaml:"max_age"`
	AgeBuckets uint32            `yaml:"age_buckets"`
	BufCap     uint32            `yaml:"buf_cap"`
}

type HistogramOptions struct {
	Buckets []float64 `yaml:"buckets"`
}

type metricObjective struct {
	Quantile float64 `yaml:"quantile"`
	Error    float64 `yaml:"error"`
}

var defaultQuantiles = []metricObjective{
	{Quantile: 0.5, Error: 0.05},
	{Quantile: 0.9, Error: 0.01},
	{Quantile: 0.99, Error: 0.001},
}

func (m *MetricMapper) InitFromYAMLString(fileContents string) error {
	var n MetricMapper

	if err := yaml.Unmarshal([]byte(fileContents), &n); err != nil {
		return err
	}

	if len(n.Defaults.HistogramOptions.Buckets) == 0 {
		n.Defaults.HistogramOptions.Buckets = prometheus.DefBuckets
	}

	if len(n.Defaults.SummaryOptions.Quantiles) == 0 {
		n.Defaults.SummaryOptions.Quantiles = defaultQuantiles
	}

	if n.Defaults.MatchType == MatchTypeDefault {
		n.Defaults.MatchType = MatchTypeGlob
	}

	remainingMappingsCount := len(n.Mappings)

	n.FSM = fsm.NewFSM([]string{string(MetricTypeCounter), string(MetricTypeGauge), string(MetricTypeObserver)},
		remainingMappingsCount, n.Defaults.GlobDisableOrdering)

	for i := range n.Mappings {
		remainingMappingsCount--

		currentMapping := &n.Mappings[i]

		// check that label is correct
		for k := range currentMapping.Labels {
			if !labelNameRE.MatchString(k) {
				return fmt.Errorf("invalid label key: %s", k)
			}
		}

		if currentMapping.Name == "" {
			return fmt.Errorf("line %d: metric mapping didn't set a metric name", i)
		}

		if !metricNameRE.MatchString(currentMapping.Name) {
			return fmt.Errorf("metric name '%s' doesn't match regex '%s'", currentMapping.Name, metricNameRE)
		}

		if currentMapping.MatchType == "" {
			currentMapping.MatchType = n.Defaults.MatchType
		}

		if currentMapping.Action == "" {
			currentMapping.Action = ActionTypeMap
		}

		if currentMapping.MatchType == MatchTypeGlob {
			n.doFSM = true
			if !metricLineRE.MatchString(currentMapping.Match) {
				return fmt.Errorf("invalid match: %s", currentMapping.Match)
			}

			captureCount := n.FSM.AddState(currentMapping.Match, string(currentMapping.MatchMetricType),
				remainingMappingsCount, currentMapping)

			currentMapping.nameFormatter = fsm.NewTemplateFormatter(currentMapping.Name, captureCount)

			labelKeys := make([]string, len(currentMapping.Labels))
			labelFormatters := make([]*fsm.TemplateFormatter, len(currentMapping.Labels))
			labelIndex := 0
			for label, valueExpr := range currentMapping.Labels {
				labelKeys[labelIndex] = label
				labelFormatters[labelIndex] = fsm.NewTemplateFormatter(valueExpr, captureCount)
				labelIndex++
			}
			currentMapping.labelFormatters = labelFormatters
			currentMapping.labelKeys = labelKeys
		} else {
			if regex, err := regexp.Compile(currentMapping.Match); err != nil {
				return fmt.Errorf("invalid regex %s in mapping: %v", currentMapping.Match, err)
			} else {
				currentMapping.regex = regex
			}
			n.doRegex = true
		}

		if currentMapping.ObserverType == "" {
			currentMapping.ObserverType = n.Defaults.ObserverType
		}

		if currentMapping.LegacyQuantiles != nil &&
			(currentMapping.SummaryOptions == nil || currentMapping.SummaryOptions.Quantiles != nil) {
			level.Warn(m.Logger).Log("msg", "using the top level quantiles is deprecated.  Please use quantiles in the summary_options hierarchy")
		}

		if currentMapping.LegacyBuckets != nil &&
			(currentMapping.HistogramOptions == nil || currentMapping.HistogramOptions.Buckets != nil) {
			level.Warn(m.Logger).Log("msg", "using the top level buckets is deprecated.  Please use buckets in the histogram_options hierarchy")
		}

		if currentMapping.SummaryOptions != nil &&
			currentMapping.LegacyQuantiles != nil &&
			currentMapping.SummaryOptions.Quantiles != nil {
			return fmt.Errorf("cannot use quantiles in both the top level and summary options at the same time in %s", currentMapping.Match)
		}

		if currentMapping.HistogramOptions != nil &&
			currentMapping.LegacyBuckets != nil &&
			currentMapping.HistogramOptions.Buckets != nil {
			return fmt.Errorf("cannot use buckets in both the top level and histogram options at the same time in %s", currentMapping.Match)
		}

		if currentMapping.ObserverType == ObserverTypeHistogram {
			if currentMapping.SummaryOptions != nil {
				return fmt.Errorf("cannot use histogram observer and summary options at the same time")
			}
			if currentMapping.HistogramOptions == nil {
				currentMapping.HistogramOptions = &HistogramOptions{}
			}
			if currentMapping.LegacyBuckets != nil && len(currentMapping.LegacyBuckets) != 0 {
				currentMapping.HistogramOptions.Buckets = currentMapping.LegacyBuckets
			}
			if currentMapping.HistogramOptions.Buckets == nil || len(currentMapping.HistogramOptions.Buckets) == 0 {
				currentMapping.HistogramOptions.Buckets = n.Defaults.HistogramOptions.Buckets
			}
		}

		if currentMapping.ObserverType == ObserverTypeSummary {
			if currentMapping.HistogramOptions != nil {
				return fmt.Errorf("cannot use summary observer and histogram options at the same time")
			}
			if currentMapping.SummaryOptions == nil {
				currentMapping.SummaryOptions = &SummaryOptions{}
			}
			if currentMapping.LegacyQuantiles != nil && len(currentMapping.LegacyQuantiles) != 0 {
				currentMapping.SummaryOptions.Quantiles = currentMapping.LegacyQuantiles
			}
			if currentMapping.SummaryOptions.Quantiles == nil || len(currentMapping.SummaryOptions.Quantiles) == 0 {
				currentMapping.SummaryOptions.Quantiles = n.Defaults.SummaryOptions.Quantiles
			}
			if currentMapping.SummaryOptions.MaxAge == 0 {
				currentMapping.SummaryOptions.MaxAge = n.Defaults.SummaryOptions.MaxAge
			}
			if currentMapping.SummaryOptions.AgeBuckets == 0 {
				currentMapping.SummaryOptions.AgeBuckets = n.Defaults.SummaryOptions.AgeBuckets
			}
			if currentMapping.SummaryOptions.BufCap == 0 {
				currentMapping.SummaryOptions.BufCap = n.Defaults.SummaryOptions.BufCap
			}
		}

		if currentMapping.Ttl == 0 && n.Defaults.Ttl > 0 {
			currentMapping.Ttl = n.Defaults.Ttl
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Defaults = n.Defaults
	m.Mappings = n.Mappings

	// Reset the cache since this function can be used to reload config
	if m.cache != nil {
		m.cache.Reset()
	}

	if n.doFSM {
		var mappings []string
		for _, mapping := range n.Mappings {
			if mapping.MatchType == MatchTypeGlob {
				mappings = append(mappings, mapping.Match)
			}
		}
		n.FSM.BacktrackingNeeded = fsm.TestIfNeedBacktracking(mappings, n.FSM.OrderingDisabled, m.Logger)

		m.FSM = n.FSM
		m.doRegex = n.doRegex
	}
	m.doFSM = n.doFSM

	if m.MappingsCount != nil {
		m.MappingsCount.Set(float64(len(n.Mappings)))
	}

	if m.Logger == nil {
		m.Logger = log.NewNopLogger()
	}

	return nil
}

func (m *MetricMapper) InitFromFile(fileName string) error {
	mappingStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}

	return m.InitFromYAMLString(string(mappingStr))
}

// UseCache tells the mapper to use a cache that implements the MetricMapperCache interface.
// This cache MUST be thread-safe!
func (m *MetricMapper) UseCache(cache MetricMapperCache) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.cache = cache
}

func (m *MetricMapper) GetMapping(statsdMetric string, statsdMetricType MetricType) (*MetricMapping, prometheus.Labels, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// only use a cache if one is present
	if m.cache != nil {
		result, cached := m.cache.Get(formatKey(statsdMetric, statsdMetricType))
		if cached {
			r := result.(MetricMapperCacheResult)
			return r.Mapping, r.Labels, r.Matched
		}
	}

	// glob matching
	if m.doFSM {
		finalState, captures := m.FSM.GetMapping(statsdMetric, string(statsdMetricType))
		if finalState != nil && finalState.Result != nil {
			v := finalState.Result.(*MetricMapping)
			result := copyMetricMapping(v)
			result.Name = result.nameFormatter.Format(captures)

			labels := prometheus.Labels{}
			for index, formatter := range result.labelFormatters {
				labels[result.labelKeys[index]] = formatter.Format(captures)
			}

			r := MetricMapperCacheResult{
				Mapping: result,
				Matched: true,
				Labels:  labels,
			}
			// add match to cache
			if m.cache != nil {
				m.cache.Add(formatKey(statsdMetric, statsdMetricType), r)
			}

			return result, labels, true
		} else if !m.doRegex {
			// if there's no regex match type, return immediately
			// Add miss to cache
			if m.cache != nil {
				m.cache.Add(formatKey(statsdMetric, statsdMetricType), MetricMapperCacheResult{})
			}
			return nil, nil, false
		}
	}

	// regex matching
	for _, mapping := range m.Mappings {
		// if a rule don't have regex matching type, the regex field is unset
		if mapping.regex == nil {
			continue
		}
		matches := mapping.regex.FindStringSubmatchIndex(statsdMetric)
		if len(matches) == 0 {
			continue
		}

		mapping.Name = string(mapping.regex.ExpandString(
			[]byte{},
			mapping.Name,
			statsdMetric,
			matches,
		))

		if mt := mapping.MatchMetricType; mt != "" && mt != statsdMetricType {
			continue
		}

		labels := prometheus.Labels{}
		for label, valueExpr := range mapping.Labels {
			value := mapping.regex.ExpandString([]byte{}, valueExpr, statsdMetric, matches)
			labels[label] = string(value)
		}

		r := MetricMapperCacheResult{
			Mapping: &mapping,
			Matched: true,
			Labels:  labels,
		}
		// Add Match to cache
		if m.cache != nil {
			m.cache.Add(formatKey(statsdMetric, statsdMetricType), r)
		}

		return &mapping, labels, true
	}

	// Add Miss to cache
	if m.cache != nil {
		m.cache.Add(formatKey(statsdMetric, statsdMetricType), MetricMapperCacheResult{})
	}
	return nil, nil, false
}

// make a shallow copy so that we do not overwrite name
// as multiple names can be matched by same mapping
func copyMetricMapping(in *MetricMapping) *MetricMapping {
	var out MetricMapping
	out = *in
	return &out
}

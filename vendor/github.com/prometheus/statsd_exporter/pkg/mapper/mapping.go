// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either xpress or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/statsd_exporter/pkg/mapper/fsm"
)

type MetricMapping struct {
	Match            string `yaml:"match"`
	Name             string `yaml:"name"`
	nameFormatter    *fsm.TemplateFormatter
	regex            *regexp.Regexp
	Labels           prometheus.Labels `yaml:"labels"`
	labelKeys        []string
	labelFormatters  []*fsm.TemplateFormatter
	ObserverType     ObserverType      `yaml:"observer_type"`
	TimerType        ObserverType      `yaml:"timer_type,omitempty"` // DEPRECATED - field only present to preserve backwards compatibility in configs. Always empty
	LegacyBuckets    []float64         `yaml:"buckets"`
	LegacyQuantiles  []metricObjective `yaml:"quantiles"`
	MatchType        MatchType         `yaml:"match_type"`
	HelpText         string            `yaml:"help"`
	Action           ActionType        `yaml:"action"`
	MatchMetricType  MetricType        `yaml:"match_metric_type"`
	Ttl              time.Duration     `yaml:"ttl"`
	SummaryOptions   *SummaryOptions   `yaml:"summary_options"`
	HistogramOptions *HistogramOptions `yaml:"histogram_options"`
}

// UnmarshalYAML is a custom unmarshal function to allow use of deprecated config keys
// observer_type will override timer_type
func (m *MetricMapping) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type MetricMappingAlias MetricMapping
	var tmp MetricMappingAlias
	if err := unmarshal(&tmp); err != nil {
		return err
	}

	// Copy defaults
	m.Match = tmp.Match
	m.Name = tmp.Name
	m.Labels = tmp.Labels
	m.ObserverType = tmp.ObserverType
	m.LegacyBuckets = tmp.LegacyBuckets
	m.LegacyQuantiles = tmp.LegacyQuantiles
	m.MatchType = tmp.MatchType
	m.HelpText = tmp.HelpText
	m.Action = tmp.Action
	m.MatchMetricType = tmp.MatchMetricType
	m.Ttl = tmp.Ttl
	m.SummaryOptions = tmp.SummaryOptions
	m.HistogramOptions = tmp.HistogramOptions

	// Use deprecated TimerType if necessary
	if tmp.ObserverType == "" {
		m.ObserverType = tmp.TimerType
	}

	return nil
}

// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"time"

	"github.com/newrelic/go-agent/v3/internal"
)

// harvestable is something that can be merged into a harvest.
type harvestable interface {
	MergeIntoHarvest(h *harvest)
}

// harvestTypes is a bit set used to indicate which data types are ready to be
// reported.
type harvestTypes uint

const (
	harvestMetricsTraces harvestTypes = 1 << iota
	harvestSpanEvents
	harvestCustomEvents
	harvestLogEvents
	harvestTxnEvents
	harvestErrorEvents
)

const (
	// harvestTypesEvents includes all Event types
	harvestTypesEvents = harvestSpanEvents | harvestCustomEvents | harvestTxnEvents | harvestErrorEvents | harvestLogEvents
	// harvestTypesAll includes all harvest types
	harvestTypesAll = harvestMetricsTraces | harvestTypesEvents
)

type harvestTimer struct {
	periods     map[harvestTypes]time.Duration
	lastHarvest map[harvestTypes]time.Time
}

func newHarvestTimer(now time.Time, periods map[harvestTypes]time.Duration) *harvestTimer {
	lastHarvest := make(map[harvestTypes]time.Time, len(periods))
	for tp := range periods {
		lastHarvest[tp] = now
	}
	return &harvestTimer{periods: periods, lastHarvest: lastHarvest}
}

func (timer *harvestTimer) ready(now time.Time) (ready harvestTypes) {
	for tp, period := range timer.periods {
		if deadline := timer.lastHarvest[tp].Add(period); now.After(deadline) {
			timer.lastHarvest[tp] = deadline
			ready |= tp
		}
	}
	return
}

// harvest contains collected data.
type harvest struct {
	timer *harvestTimer

	Metrics      *metricTable
	ErrorTraces  harvestErrors
	TxnTraces    *harvestTraces
	SlowSQLs     *slowQueries
	SpanEvents   *spanEvents
	CustomEvents *customEvents
	LogEvents    *logEvents
	TxnEvents    *txnEvents
	ErrorEvents  *errorEvents
}

const (
	// txnEventPayloadlimit is the maximum number of events that should be
	// sent up in one post.
	txnEventPayloadlimit = 5000
)

// Ready returns a new harvest which contains the data types ready for harvest,
// or nil if no data is ready for harvest.
func (h *harvest) Ready(now time.Time) *harvest {
	ready := &harvest{}

	types := h.timer.ready(now)
	if 0 == types {
		return nil
	}

	if 0 != types&harvestCustomEvents {
		h.Metrics.addCount(customEventsSeen, h.CustomEvents.NumSeen(), forced)
		h.Metrics.addCount(customEventsSent, h.CustomEvents.NumSaved(), forced)
		ready.CustomEvents = h.CustomEvents
		h.CustomEvents = newCustomEvents(h.CustomEvents.capacity())
	}
	if 0 != types&harvestLogEvents {
		h.LogEvents.RecordLoggingMetrics(h.Metrics)
		ready.LogEvents = h.LogEvents
		h.LogEvents = newLogEvents(h.LogEvents.commonAttributes, h.LogEvents.config)
	}
	if 0 != types&harvestTxnEvents {
		h.Metrics.addCount(txnEventsSeen, h.TxnEvents.NumSeen(), forced)
		h.Metrics.addCount(txnEventsSent, h.TxnEvents.NumSaved(), forced)
		ready.TxnEvents = h.TxnEvents
		h.TxnEvents = newTxnEvents(h.TxnEvents.capacity())
	}
	if 0 != types&harvestErrorEvents {
		h.Metrics.addCount(errorEventsSeen, h.ErrorEvents.NumSeen(), forced)
		h.Metrics.addCount(errorEventsSent, h.ErrorEvents.NumSaved(), forced)
		ready.ErrorEvents = h.ErrorEvents
		h.ErrorEvents = newErrorEvents(h.ErrorEvents.capacity())
	}
	if 0 != types&harvestSpanEvents {
		h.Metrics.addCount(spanEventsSeen, h.SpanEvents.NumSeen(), forced)
		h.Metrics.addCount(spanEventsSent, h.SpanEvents.NumSaved(), forced)
		ready.SpanEvents = h.SpanEvents
		h.SpanEvents = newSpanEvents(h.SpanEvents.capacity())
	}
	// NOTE! Metrics must happen after the event harvest conditionals to
	// ensure that the metrics contain the event supportability metrics.
	if 0 != types&harvestMetricsTraces {
		ready.Metrics = h.Metrics
		ready.ErrorTraces = h.ErrorTraces
		ready.SlowSQLs = h.SlowSQLs
		ready.TxnTraces = h.TxnTraces
		h.Metrics = newMetricTable(maxMetrics, now)
		h.ErrorTraces = newHarvestErrors(maxHarvestErrors)
		h.SlowSQLs = newSlowQueries(maxHarvestSlowSQLs)
		h.TxnTraces = newHarvestTraces()
	}
	return ready
}

// Payloads returns a slice of payload creators.
func (h *harvest) Payloads(splitLargeTxnEvents bool) (ps []payloadCreator) {
	if nil == h {
		return
	}
	if nil != h.CustomEvents {
		ps = append(ps, h.CustomEvents)
	}
	if nil != h.LogEvents {
		ps = append(ps, h.LogEvents)
	}
	if nil != h.ErrorEvents {
		ps = append(ps, h.ErrorEvents)
	}
	if nil != h.SpanEvents {
		ps = append(ps, h.SpanEvents)
	}
	if nil != h.Metrics {
		ps = append(ps, h.Metrics)
	}
	if nil != h.ErrorTraces {
		ps = append(ps, h.ErrorTraces)
	}
	if nil != h.TxnTraces {
		ps = append(ps, h.TxnTraces)
	}
	if nil != h.SlowSQLs {
		ps = append(ps, h.SlowSQLs)
	}
	if nil != h.TxnEvents {
		if splitLargeTxnEvents {
			ps = append(ps, h.TxnEvents.payloads(txnEventPayloadlimit)...)
		} else {
			ps = append(ps, h.TxnEvents)
		}
	}
	return
}

type harvestConfig struct {
	ReportPeriods    map[harvestTypes]time.Duration
	CommonAttributes commonAttributes
	LoggingConfig    loggingConfig
	MaxSpanEvents    int
	MaxCustomEvents  int
	MaxErrorEvents   int
	MaxTxnEvents     int
}

// newHarvest returns a new Harvest.
func newHarvest(now time.Time, configurer harvestConfig) *harvest {
	return &harvest{
		timer:        newHarvestTimer(now, configurer.ReportPeriods),
		Metrics:      newMetricTable(maxMetrics, now),
		ErrorTraces:  newHarvestErrors(maxHarvestErrors),
		TxnTraces:    newHarvestTraces(),
		SlowSQLs:     newSlowQueries(maxHarvestSlowSQLs),
		SpanEvents:   newSpanEvents(configurer.MaxSpanEvents),
		CustomEvents: newCustomEvents(configurer.MaxCustomEvents),
		LogEvents:    newLogEvents(configurer.CommonAttributes, configurer.LoggingConfig),
		TxnEvents:    newTxnEvents(configurer.MaxTxnEvents),
		ErrorEvents:  newErrorEvents(configurer.MaxErrorEvents),
	}
}

func createTrackUsageMetrics(metrics *metricTable) {
	for _, m := range internal.GetUsageSupportabilityMetrics() {
		metrics.addSingleCount(m, forced)
	}
}

func createTraceObserverMetrics(to traceObserver, metrics *metricTable) {
	if to == nil {
		return
	}
	for name, val := range to.dumpSupportabilityMetrics() {
		metrics.addCount(name, val, forced)
	}
}

func createAppLoggingSupportabilityMetrics(lc *loggingConfig, metrics *metricTable) {
	lc.connectMetrics(metrics)
}

// CreateFinalMetrics creates extra metrics at harvest time.
func (h *harvest) CreateFinalMetrics(run *appRun, to traceObserver) {
	reply := run.Reply
	hc := run.harvestConfig
	if nil == h {
		return
	}
	// Metrics will be non-nil when harvesting metrics (regardless of
	// whether or not there are any metrics to send).
	if nil == h.Metrics {
		return
	}

	h.Metrics.addSingleCount(instanceReporting, forced)

	// Configurable event harvest supportability metrics:
	// https://source.datanerd.us/agents/agent-specs/blob/master/Connect-LEGACY.md#event-harvest-config
	period := reply.ConfigurablePeriod()
	h.Metrics.addDuration(supportReportPeriod, "", period, period, forced)
	h.Metrics.addValue(supportTxnEventLimit, "", float64(hc.MaxTxnEvents), forced)
	h.Metrics.addValue(supportCustomEventLimit, "", float64(hc.MaxCustomEvents), forced)
	h.Metrics.addValue(supportErrorEventLimit, "", float64(hc.MaxErrorEvents), forced)
	h.Metrics.addValue(supportSpanEventLimit, "", float64(hc.MaxSpanEvents), forced)
	h.Metrics.addValue(supportLogEventLimit, "", float64(hc.LoggingConfig.maxLogEvents), forced)

	createTraceObserverMetrics(to, h.Metrics)
	createTrackUsageMetrics(h.Metrics)
	createAppLoggingSupportabilityMetrics(&hc.LoggingConfig, h.Metrics)

	h.Metrics = h.Metrics.ApplyRules(reply.MetricRules)
}

// payloadCreator is a data type in the harvest.
type payloadCreator interface {
	// In the event of a rpm request failure (hopefully simply an
	// intermittent collector issue) the payload may be merged into the next
	// time period's harvest.
	harvestable
	// Data prepares JSON in the format expected by the collector endpoint.
	// This method should return (nil, nil) if the payload is empty and no
	// rpm request is necessary.
	Data(agentRunID string, harvestStart time.Time) ([]byte, error)
	// EndpointMethod is used for the "method" query parameter when posting
	// the data.
	EndpointMethod() string
}

// createTxnMetrics creates metrics for a transaction.
func createTxnMetrics(args *txnData, metrics *metricTable) {
	withoutFirstSegment := removeFirstSegment(args.FinalName)

	// Duration Metrics
	var durationRollup string
	var totalTimeRollup string
	if args.IsWeb {
		durationRollup = webRollup
		totalTimeRollup = totalTimeWeb
		metrics.addDuration(dispatcherMetric, "", args.Duration, 0, forced)
	} else {
		durationRollup = backgroundRollup
		totalTimeRollup = totalTimeBackground
	}

	metrics.addDuration(args.FinalName, "", args.Duration, 0, forced)
	metrics.addDuration(durationRollup, "", args.Duration, 0, forced)

	metrics.addDuration(totalTimeRollup, "", args.TotalTime, args.TotalTime, forced)
	metrics.addDuration(totalTimeRollup+"/"+withoutFirstSegment, "", args.TotalTime, args.TotalTime, unforced)

	// Better CAT Metrics
	if cat := args.BetterCAT; cat.Enabled {
		caller := callerUnknown
		if nil != cat.Inbound && cat.Inbound.HasNewRelicTraceInfo {
			caller.Type = cat.Inbound.Type
			caller.App = cat.Inbound.App
			caller.Account = cat.Inbound.Account
		}
		if cat.TransportType != "" {
			caller.TransportType = cat.TransportType
		}
		m := durationByCallerMetric(caller)
		metrics.addDuration(m.all, "", args.Duration, args.Duration, unforced)
		metrics.addDuration(m.webOrOther(args.IsWeb), "", args.Duration, args.Duration, unforced)

		// Transport Duration Metric
		if nil != cat.Inbound && cat.Inbound.HasNewRelicTraceInfo {
			d := cat.Inbound.TransportDuration
			m = transportDurationMetric(caller)
			metrics.addDuration(m.all, "", d, d, unforced)
			metrics.addDuration(m.webOrOther(args.IsWeb), "", d, d, unforced)
		}

		// CAT Error Metrics
		if args.HasErrors() {
			m = errorsByCallerMetric(caller)
			metrics.addSingleCount(m.all, unforced)
			metrics.addSingleCount(m.webOrOther(args.IsWeb), unforced)
		}

		args.DistributedTracingSupport.createMetrics(metrics)
	}

	// Apdex Metrics
	if args.Zone != apdexNone {
		metrics.addApdex(apdexRollup, "", args.ApdexThreshold, args.Zone, forced)

		mname := apdexPrefix + withoutFirstSegment
		metrics.addApdex(mname, "", args.ApdexThreshold, args.Zone, unforced)
	}

	// Error Metrics
	if args.NoticeErrors() {
		metrics.addSingleCount(errorsRollupMetric.all, forced)
		metrics.addSingleCount(errorsRollupMetric.webOrOther(args.IsWeb), forced)
		metrics.addSingleCount(errorsPrefix+args.FinalName, forced)
	}

	if args.HasExpectedErrors() {
		metrics.addSingleCount(expectedErrorsRollupMetric.all, forced)
	}

	// Queueing Metrics
	if args.Queuing > 0 {
		metrics.addDuration(queueMetric, "", args.Queuing, args.Queuing, forced)
	}
}

var (

	// This should only be used by harvests in cases where a connect response is unavailable
	dfltHarvestCfgr = harvestConfig{
		ReportPeriods:   map[harvestTypes]time.Duration{harvestTypesAll: fixedHarvestPeriod},
		MaxTxnEvents:    internal.MaxTxnEvents,
		MaxSpanEvents:   internal.MaxSpanEvents,
		MaxCustomEvents: internal.MaxCustomEvents,
		MaxErrorEvents:  internal.MaxErrorEvents,
		LoggingConfig: loggingConfig{
			true,
			false,
			true,
			false,
			internal.MaxLogEvents,
		},
	}
)

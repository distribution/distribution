// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/internal"
)

// appRun contains information regarding a single connection session with the
// collector.  It is immutable after creation at application connect.
type appRun struct {
	Reply *internal.ConnectReply

	// AttributeConfig is calculated on every connect since it depends on
	// the security policies.
	AttributeConfig *attributeConfig
	Config          config

	// firstAppName is the value of Config.AppName up to the first semicolon.
	firstAppName string

	adaptiveSampler *adaptiveSampler

	// rulesCache caches the results of creating transaction names.  It
	// exists here since it is specific to a set of rules and is shared
	// between transactions.
	rulesCache *rulesCache

	// harvestConfig contains configuration related to event limits and
	// flexible harvest periods.  This field is created once at appRun
	// creation.
	harvestConfig harvestConfig

	// Error code caches for faster lookups O(1)
	ignoreErrorCodesCache map[int]bool
	expectErrorCodesCache map[int]bool
	mu                    sync.RWMutex
}

const (
	txnNameCacheLimit = 40
)

func newAppRun(config config, reply *internal.ConnectReply) *appRun {
	run := &appRun{
		Reply:                 reply,
		AttributeConfig:       createAttributeConfig(config, reply.SecurityPolicies.AttributesInclude.Enabled()),
		Config:                config,
		rulesCache:            newRulesCache(txnNameCacheLimit),
		ignoreErrorCodesCache: make(map[int]bool),
		expectErrorCodesCache: make(map[int]bool),
	}

	// Overwrite local settings with any server-side-config settings
	// present. NOTE!  This requires that the Config provided to this
	// function is a value and not a pointer: We do not want to change the
	// input Config with values particular to this connection.

	if v := run.Reply.ServerSideConfig.TransactionTracerEnabled; v != nil {
		run.Config.TransactionTracer.Enabled = *v
	}
	if v := run.Reply.ServerSideConfig.ErrorCollectorEnabled; v != nil {
		run.Config.ErrorCollector.Enabled = *v
	}
	if v := run.Reply.ServerSideConfig.CrossApplicationTracerEnabled; v != nil {
		run.Config.CrossApplicationTracer.Enabled = *v
	}
	if v := run.Reply.ServerSideConfig.TransactionTracerThreshold; v != nil {
		switch val := v.(type) {
		case float64:
			run.Config.TransactionTracer.Threshold.IsApdexFailing = false
			run.Config.TransactionTracer.Threshold.Duration = internal.FloatSecondsToDuration(val)
		case string:
			if val == "apdex_f" {
				run.Config.TransactionTracer.Threshold.IsApdexFailing = true
			}
		}
	}
	if v := run.Reply.ServerSideConfig.TransactionTracerStackTraceThreshold; v != nil {
		run.Config.TransactionTracer.Segments.StackTraceThreshold = internal.FloatSecondsToDuration(*v)
	}
	if v := run.Reply.ServerSideConfig.ErrorCollectorIgnoreStatusCodes; v != nil {
		run.Config.ErrorCollector.IgnoreStatusCodes = v
	}
	if run.Config.ErrorCollector.IgnoreStatusCodes != nil {
		run.mu.Lock()
		for _, errorCode := range run.Config.ErrorCollector.IgnoreStatusCodes {
			run.ignoreErrorCodesCache[errorCode] = true
		}
		run.mu.Unlock()
	}

	if v := run.Reply.ServerSideConfig.ErrorCollectorExpectStatusCodes; v != nil {
		run.Config.ErrorCollector.ExpectStatusCodes = v
	}
	if run.Config.ErrorCollector.IgnoreStatusCodes != nil {
		run.mu.Lock()
		for _, errorCode := range run.Config.ErrorCollector.ExpectStatusCodes {
			run.expectErrorCodesCache[errorCode] = true
		}
		run.mu.Unlock()
	}

	if !run.Reply.CollectErrorEvents {
		run.Config.ErrorCollector.CaptureEvents = false
	}
	if !run.Reply.CollectAnalyticsEvents {
		run.Config.TransactionEvents.Enabled = false
	}
	if !run.Reply.CollectTraces {
		run.Config.TransactionTracer.Enabled = false
		run.Config.DatastoreTracer.SlowQuery.Enabled = false
	}
	if !run.Reply.CollectSpanEvents {
		run.Config.SpanEvents.Enabled = false
	}

	// Distributed tracing takes priority over cross-app-tracing per:
	// https://source.datanerd.us/agents/agent-specs/blob/master/Distributed-Tracing.md#distributed-trace-payload
	if run.Config.DistributedTracer.Enabled {
		run.Config.CrossApplicationTracer.Enabled = false
	}

	// Cache the first application name set on the config
	run.firstAppName = strings.SplitN(config.AppName, ";", 2)[0]

	run.adaptiveSampler = newAdaptiveSampler(
		time.Duration(reply.SamplingTargetPeriodInSeconds)*time.Second,
		reply.SamplingTarget,
		time.Now())

	if run.Reply.RunID != "" {
		js, _ := json.Marshal(settings(run.Config.Config))
		run.Config.Logger.Debug("final configuration", map[string]interface{}{
			"config": jsonString(js),
		})
	}

	run.harvestConfig = harvestConfig{
		ReportPeriods:   run.ReportPeriods(),
		MaxTxnEvents:    run.MaxTxnEvents(),
		MaxCustomEvents: run.MaxCustomEvents(),
		MaxErrorEvents:  run.MaxErrorEvents(),
		MaxSpanEvents:   run.MaxSpanEvents(),
		LoggingConfig:   run.LoggingConfig(),
	}

	return run
}

func newPlaceholderAppRun(config config) *appRun {
	reply := internal.ConnectReplyDefaults()
	// Do no sampling if the app isn't connected:
	reply.SamplingTarget = 0
	return newAppRun(config, reply)
}

const (
	// https://source.datanerd.us/agents/agent-specs/blob/master/Lambda.md#distributed-tracing
	serverlessDefaultPrimaryAppID = "Unknown"
)

func newServerlessConnectReply(config config) *internal.ConnectReply {
	reply := internal.ConnectReplyDefaults()

	reply.ApdexThresholdSeconds = config.ServerlessMode.ApdexThreshold.Seconds()

	reply.AccountID = config.ServerlessMode.AccountID
	reply.TrustedAccountKey = config.ServerlessMode.TrustedAccountKey
	reply.PrimaryAppID = config.ServerlessMode.PrimaryAppID

	if reply.TrustedAccountKey == "" {
		// The trust key does not need to be provided by customers whose
		// account ID is the same as the trust key.
		reply.TrustedAccountKey = reply.AccountID
	}

	if reply.PrimaryAppID == "" {
		reply.PrimaryAppID = serverlessDefaultPrimaryAppID
	}

	// https://source.datanerd.us/agents/agent-specs/blob/master/Lambda.md#adaptive-sampling
	reply.SamplingTargetPeriodInSeconds = 60
	reply.SamplingTarget = 10

	return reply
}

func (run *appRun) responseCodeIsError(code int) bool {
	// Response codes below 100 are allowed to be errors to support gRPC.
	if code < 400 && code >= 100 {
		return false
	}
	run.mu.RLock()
	defer run.mu.RUnlock()
	return !run.ignoreErrorCodesCache[code]
}

func (run *appRun) responseCodeIsExpected(code int) bool {
	run.mu.RLock()
	defer run.mu.RUnlock()
	return run.expectErrorCodesCache[code]
}

func (run *appRun) txnTraceThreshold(apdexThreshold time.Duration) time.Duration {
	if run.Config.TransactionTracer.Threshold.IsApdexFailing {
		return apdexFailingThreshold(apdexThreshold)
	}
	return run.Config.TransactionTracer.Threshold.Duration
}

func (run *appRun) ptrTxnEvents() *uint    { return run.Reply.EventData.Limits.TxnEvents }
func (run *appRun) ptrCustomEvents() *uint { return run.Reply.EventData.Limits.CustomEvents }
func (run *appRun) ptrLogEvents() *uint    { return run.Reply.EventData.Limits.LogEvents }
func (run *appRun) ptrErrorEvents() *uint  { return run.Reply.EventData.Limits.ErrorEvents }
func (run *appRun) ptrSpanEvents() *uint   { return run.Reply.SpanEventHarvestConfig.HarvestLimit }

func (run *appRun) MaxTxnEvents() int { return run.limit(run.Config.maxTxnEvents(), run.ptrTxnEvents) }
func (run *appRun) MaxCustomEvents() int {
	return run.limit(internal.MaxCustomEvents, run.ptrCustomEvents)
}
func (run *appRun) MaxLogEvents() int {
	return run.limit(internal.MaxLogEvents, run.ptrLogEvents)
}
func (run *appRun) MaxErrorEvents() int {
	return run.limit(internal.MaxErrorEvents, run.ptrErrorEvents)
}

func (run *appRun) LoggingConfig() (config loggingConfig) {
	logging := run.Config.ApplicationLogging

	config.loggingEnabled = logging.Enabled
	config.collectEvents = logging.Enabled && logging.Forwarding.Enabled && !run.Config.HighSecurity
	config.maxLogEvents = run.MaxLogEvents()
	config.collectMetrics = logging.Enabled && logging.Metrics.Enabled
	config.localEnrichment = logging.Enabled && logging.LocalDecorating.Enabled

	return config
}

// MaxSpanEvents returns the reservoir limit for collected span events,
// which will be the default or the user's configured size (if any), but
// may be capped to the maximum allowed by the collector.
func (run *appRun) MaxSpanEvents() int {
	return run.limit(internal.MaxSpanEvents, run.ptrSpanEvents)
}

func (run *appRun) limit(dflt int, field func() *uint) int {
	if field() != nil {
		return int(*field())
	}
	return dflt
}

func (run *appRun) ReportPeriods() map[harvestTypes]time.Duration {
	fixed := harvestMetricsTraces
	configurable := harvestTypes(0)

	for tp, fn := range map[harvestTypes]func() *uint{
		harvestTxnEvents:    run.ptrTxnEvents,
		harvestCustomEvents: run.ptrCustomEvents,
		harvestLogEvents:    run.ptrLogEvents,
		harvestErrorEvents:  run.ptrErrorEvents,
		harvestSpanEvents:   run.ptrSpanEvents,
	} {
		if run != nil && fn() != nil {
			configurable |= tp
		} else {
			fixed |= tp
		}
	}
	return map[harvestTypes]time.Duration{
		configurable: run.Reply.ConfigurablePeriod(),
		fixed:        fixedHarvestPeriod,
	}
}

func (run *appRun) createTransactionName(input string, isWeb bool) string {
	if name := run.rulesCache.find(input, isWeb); name != "" {
		return name
	}
	name := internal.CreateFullTxnName(input, run.Reply, isWeb)
	if name != "" {
		// Note that we  don't cache situations where the rules say
		// ignore.  It would increase complication (we would need to
		// disambiguate not-found vs ignore).  Also, the ignore code
		// path is probably extremely uncommon.
		run.rulesCache.set(input, isWeb, name)
	}
	return name
}

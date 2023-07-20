// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

// AgentRunID identifies the current connection with the collector.
type AgentRunID string

// DialerFunc is a shorthand that is used in tests for connecting directly
// to a local gRPC server
type DialerFunc func(context.Context, string) (net.Conn, error)

func (id AgentRunID) String() string {
	return string(id)
}

// PreconnectReply contains settings from the preconnect endpoint.
type PreconnectReply struct {
	Collector        string           `json:"redirect_host"`
	SecurityPolicies SecurityPolicies `json:"security_policies"`
}

// ConnectReply contains all of the settings and state send down from the
// collector.  It should not be modified after creation.
type ConnectReply struct {
	RunID                 AgentRunID        `json:"agent_run_id"`
	RequestHeadersMap     map[string]string `json:"request_headers_map"`
	MaxPayloadSizeInBytes int               `json:"max_payload_size_in_bytes"`
	EntityGUID            string            `json:"entity_guid"`

	// Transaction Name Modifiers
	SegmentTerms segmentRules `json:"transaction_segment_terms"`
	TxnNameRules MetricRules  `json:"transaction_name_rules"`
	URLRules     MetricRules  `json:"url_rules"`
	MetricRules  MetricRules  `json:"metric_name_rules"`

	// Cross Process
	EncodingKey     string            `json:"encoding_key"`
	CrossProcessID  string            `json:"cross_process_id"`
	TrustedAccounts TrustedAccountSet `json:"trusted_account_ids"`

	// Settings
	KeyTxnApdex            map[string]float64 `json:"web_transactions_apdex"`
	ApdexThresholdSeconds  float64            `json:"apdex_t"`
	CollectAnalyticsEvents bool               `json:"collect_analytics_events"`
	CollectCustomEvents    bool               `json:"collect_custom_events"`
	CollectTraces          bool               `json:"collect_traces"`
	CollectErrors          bool               `json:"collect_errors"`
	CollectErrorEvents     bool               `json:"collect_error_events"`
	CollectSpanEvents      bool               `json:"collect_span_events"`

	// RUM
	AgentLoader string `json:"js_agent_loader"`
	Beacon      string `json:"beacon"`
	BrowserKey  string `json:"browser_key"`
	AppID       string `json:"application_id"`
	ErrorBeacon string `json:"error_beacon"`
	JSAgentFile string `json:"js_agent_file"`

	// PreconnectReply fields are not in the connect reply, this embedding
	// is done to simplify code.
	PreconnectReply `json:"-"`

	Messages []struct {
		Message string `json:"message"`
		Level   string `json:"level"`
	} `json:"messages"`

	// TraceIDGenerator creates random IDs for distributed tracing.  It
	// exists here in the connect reply so it can be modified to create
	// deterministic identifiers in tests.
	TraceIDGenerator *TraceIDGenerator `json:"-"`
	// DistributedTraceTimestampGenerator allows tests to fix the outbound
	// DT header timestamp.
	DistributedTraceTimestampGenerator func() time.Time `json:"-"`
	// TraceObsDialer allows tests to connect to a local TraceObserver directly
	TraceObsDialer DialerFunc

	// BetterCAT/Distributed Tracing
	AccountID                     string `json:"account_id"`
	TrustedAccountKey             string `json:"trusted_account_key"`
	PrimaryAppID                  string `json:"primary_application_id"`
	SamplingTarget                uint64 `json:"sampling_target"`
	SamplingTargetPeriodInSeconds int    `json:"sampling_target_period_in_seconds"`

	ServerSideConfig struct {
		TransactionTracerEnabled *bool `json:"transaction_tracer.enabled"`
		// TransactionTracerThreshold should contain either a number or
		// "apdex_f" if it is non-nil.
		TransactionTracerThreshold           interface{} `json:"transaction_tracer.transaction_threshold"`
		TransactionTracerStackTraceThreshold *float64    `json:"transaction_tracer.stack_trace_threshold"`
		ErrorCollectorEnabled                *bool       `json:"error_collector.enabled"`
		ErrorCollectorIgnoreStatusCodes      []int       `json:"error_collector.ignore_status_codes"`
		ErrorCollectorExpectStatusCodes      []int       `json:"error_collector.expected_status_codes"`
		CrossApplicationTracerEnabled        *bool       `json:"cross_application_tracer.enabled"`
	} `json:"agent_config"`

	// Faster Event Harvest
	EventData              EventHarvestConfig `json:"event_harvest_config"`
	SpanEventHarvestConfig `json:"span_event_harvest_config"`
}

// EventHarvestConfig contains fields relating to faster event harvest.
// This structure is used in the connect request (to send up defaults)
// and in the connect response (to get the server values).
//
// https://source.datanerd.us/agents/agent-specs/blob/master/Connect-LEGACY.md#event_harvest_config-hash
// https://source.datanerd.us/agents/agent-specs/blob/master/Connect-LEGACY.md#event-harvest-config
type EventHarvestConfig struct {
	ReportPeriodMs int `json:"report_period_ms,omitempty"`
	Limits         struct {
		TxnEvents    *uint `json:"analytic_event_data,omitempty"`
		CustomEvents *uint `json:"custom_event_data,omitempty"`
		LogEvents    *uint `json:"log_event_data,omitempty"`
		ErrorEvents  *uint `json:"error_event_data,omitempty"`
		SpanEvents   *uint `json:"span_event_data,omitempty"`
	} `json:"harvest_limits"`
}

// SpanEventHarvestConfig contains the Reporting period time and the given harvest limit.
type SpanEventHarvestConfig struct {
	ReportPeriod *uint `json:"report_period_ms"`
	HarvestLimit *uint `json:"harvest_limit"`
}

// ConfigurablePeriod returns the Faster Event Harvest configurable reporting period if it is set, or the default
// report period otherwise.
func (r *ConnectReply) ConfigurablePeriod() time.Duration {
	ms := DefaultConfigurableEventHarvestMs
	if nil != r && r.EventData.ReportPeriodMs > 0 {
		ms = r.EventData.ReportPeriodMs
	}
	return time.Duration(ms) * time.Millisecond
}

func uintPtr(x uint) *uint { return &x }

// DefaultEventHarvestConfig provides faster event harvest defaults.
func DefaultEventHarvestConfig(maxTxnEvents, maxLogEvents, maxCustomEvents int) EventHarvestConfig {
	cfg := EventHarvestConfig{}
	cfg.ReportPeriodMs = DefaultConfigurableEventHarvestMs
	cfg.Limits.TxnEvents = uintPtr(uint(maxTxnEvents))
	cfg.Limits.CustomEvents = uintPtr(uint(maxCustomEvents))
	cfg.Limits.LogEvents = uintPtr(uint(maxLogEvents))
	cfg.Limits.ErrorEvents = uintPtr(uint(MaxErrorEvents))
	return cfg
}

// DefaultEventHarvestConfigWithDT is an extended version of DefaultEventHarvestConfig,
// with the addition that it takes into account distributed tracer span event harvest limits.
func DefaultEventHarvestConfigWithDT(maxTxnEvents, maxLogEvents, maxCustomEvents, spanEventLimit int, dtEnabled bool) EventHarvestConfig {
	cfg := DefaultEventHarvestConfig(maxTxnEvents, maxLogEvents, maxCustomEvents)
	if dtEnabled {
		cfg.Limits.SpanEvents = uintPtr(uint(spanEventLimit))
	}
	return cfg
}

// TrustedAccountSet is used for CAT.
type TrustedAccountSet map[int]struct{}

// IsTrusted reveals whether the account can be trusted.
func (t *TrustedAccountSet) IsTrusted(account int) bool {
	_, exists := (*t)[account]
	return exists
}

// UnmarshalJSON unmarshals the trusted set from the connect reply JSON.
func (t *TrustedAccountSet) UnmarshalJSON(data []byte) error {
	accounts := make([]int, 0)
	if err := json.Unmarshal(data, &accounts); err != nil {
		return err
	}

	*t = make(TrustedAccountSet)
	for _, account := range accounts {
		(*t)[account] = struct{}{}
	}

	return nil
}

// ConnectReplyDefaults returns a newly allocated ConnectReply with the proper
// default settings.  A pointer to a global is not used to prevent consumers
// from changing the default settings.
func ConnectReplyDefaults() *ConnectReply {
	return &ConnectReply{
		ApdexThresholdSeconds:  0.5,
		CollectAnalyticsEvents: true,
		CollectCustomEvents:    true,
		CollectTraces:          true,
		CollectErrors:          true,
		CollectErrorEvents:     true,
		CollectSpanEvents:      true,
		MaxPayloadSizeInBytes:  MaxPayloadSizeInBytes,

		SamplingTarget:                10,
		SamplingTargetPeriodInSeconds: 60,

		TraceIDGenerator:                   NewTraceIDGenerator(int64(time.Now().UnixNano())),
		DistributedTraceTimestampGenerator: time.Now,
	}
}

// CalculateApdexThreshold calculates the apdex threshold.
func CalculateApdexThreshold(c *ConnectReply, txnName string) time.Duration {
	if t, ok := c.KeyTxnApdex[txnName]; ok {
		return FloatSecondsToDuration(t)
	}
	return FloatSecondsToDuration(c.ApdexThresholdSeconds)
}

const (
	webMetricPrefix        = "WebTransaction/Go"
	backgroundMetricPrefix = "OtherTransaction/Go"
)

// CreateFullTxnName uses collector rules and the appropriate metric prefix to
// construct the full transaction metric name from the name given by the
// consumer.
func CreateFullTxnName(input string, reply *ConnectReply, isWeb bool) string {
	var afterURLRules string
	if "" != input {
		afterURLRules = reply.URLRules.Apply(input)
		if "" == afterURLRules {
			return ""
		}
	}

	prefix := backgroundMetricPrefix
	if isWeb {
		prefix = webMetricPrefix
	}

	var beforeNameRules string
	if strings.HasPrefix(afterURLRules, "/") {
		beforeNameRules = prefix + afterURLRules
	} else {
		beforeNameRules = prefix + "/" + afterURLRules
	}

	afterNameRules := reply.TxnNameRules.Apply(beforeNameRules)
	if "" == afterNameRules {
		return ""
	}

	return reply.SegmentTerms.apply(afterNameRules)
}

// RequestEventLimits sets limits for reservior testing
type RequestEventLimits struct {
	CustomEvents int
}

const (
	// CustomEventHarvestsPerMinute is the number of times per minute custom events are harvested
	CustomEventHarvestsPerMinute = 5
)

// MockConnectReplyEventLimits sets up a mock connect reply to test event limits
// currently only verifies custom insights events
func (r *ConnectReply) MockConnectReplyEventLimits(limits *RequestEventLimits) {
	r.SetSampleEverything()

	r.EventData.Limits.CustomEvents = uintPtr(uint(limits.CustomEvents) / (60 / CustomEventHarvestsPerMinute))

	// The mock server will be limited to a maximum of 100,000 events per minute
	if limits.CustomEvents > 100000 {
		r.EventData.Limits.CustomEvents = uintPtr(uint(100000) / (60 / CustomEventHarvestsPerMinute))
	}

	if limits.CustomEvents <= 0 {
		r.EventData.Limits.CustomEvents = uintPtr(uint(0) / (60 / CustomEventHarvestsPerMinute))
	}
}

// SetSampleEverything is used for testing to ensure span events get saved.
func (r *ConnectReply) SetSampleEverything() {
	// These constants are not large enough to sample everything forever,
	// but should satisfy our tests!
	r.SamplingTarget = 1000 * 1000 * 1000
	r.SamplingTargetPeriodInSeconds = 1000 * 1000 * 1000
}

// SetSampleNothing is used for testing to ensure no span events get saved.
func (r *ConnectReply) SetSampleNothing() {
	r.SamplingTarget = 0
}

// UnmarshalConnectReply takes the body of a Connect reply, in the form of bytes, and a
// PreconnectReply, and converts it into a *ConnectReply
func UnmarshalConnectReply(body []byte, preconnect PreconnectReply) (*ConnectReply, error) {
	var reply struct {
		Reply *ConnectReply `json:"return_value"`
	}
	reply.Reply = ConnectReplyDefaults()
	err := json.Unmarshal(body, &reply)
	if nil != err {
		return nil, fmt.Errorf("unable to parse connect reply: %v", err)
	}

	reply.Reply.PreconnectReply = preconnect

	return reply.Reply, nil
}

// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

const (
	// app behavior

	// DefaultConfigurableEventHarvestMs is the period for custom, error,
	// and transaction events if the connect response's
	// "event_harvest_config.report_period_ms" is missing or invalid.
	DefaultConfigurableEventHarvestMs = 60 * 1000
	// MaxPayloadSizeInBytes specifies the maximum payload size in bytes that
	// should be sent to any endpoint
	MaxPayloadSizeInBytes = 1000 * 1000
	// MaxCustomEvents is the maximum number of Transaction Events that can be captured
	// per 60-second harvest cycle
	MaxCustomEvents = 30 * 1000
	// MaxLogEvents is the maximum number of Log Events that can be captured per
	// 60-second harvest cycle
	MaxLogEvents = 10 * 1000
	// MaxTxnEvents is the maximum number of Transaction Events that can be captured
	// per 60-second harvest cycle
	MaxTxnEvents = 10 * 1000
	// MaxErrorEvents is the maximum number of Error Events that can be captured
	// per 60-second harvest cycle
	MaxErrorEvents = 100
	// MaxSpanEvents is the maximum number of Spans Events that can be captured
	// per 60-second harvest cycle
	MaxSpanEvents = 1000
)

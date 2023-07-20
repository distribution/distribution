// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// +build !go1.9

package newrelic

import (
	"github.com/newrelic/go-agent/v3/internal"
)

func newTraceObserver(runID internal.AgentRunID, requestHeadersMap map[string]string, cfg observerConfig) (traceObserver, error) {
	return nil, errUnsupportedVersion
}

const (
	// versionSupports8T records whether we are using a supported version of Go for
	// Infinite Tracing
	versionSupports8T = false
	grpcVersion       = "not-installed"
)

func expectObserverEvents(v internal.Validator, events *analyticsEvents, expect []internal.WantEvent, extraAttributes map[string]interface{}) {
}

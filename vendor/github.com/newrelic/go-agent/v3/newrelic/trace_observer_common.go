// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"errors"
	"time"

	"github.com/newrelic/go-agent/v3/internal"
)

type traceObserver interface {
	// restart reconnects to the remote trace observer with the given runID and
	// request headers.
	restart(runID internal.AgentRunID, requestHeadersMap map[string]string)
	// shutdown initiates a shutdown of the trace observer and blocks until either
	// shutdown is complete or the given timeout is hit.
	shutdown(time.Duration) error
	// consumeSpan enqueues the span to be sent to the remote trace observer
	consumeSpan(*spanEvent)
	// dumpSupportabilityMetrics returns a map of string to float to be turned into metrics
	dumpSupportabilityMetrics() map[string]float64
	// initialConnCompleted indicates that the initial connection to the remote trace
	// observer was made, but it does NOT indicate anything about the current state of the
	// connection
	initialConnCompleted() bool
}

type observerConfig struct {
	// endpoint includes data about connecting to the remote trace observer
	endpoint observerURL
	// license is the New Relic License key
	license string
	// log will be used for logging
	log Logger
	// queueSize is the size of the channel used to send span events to
	// the remote trace observer
	queueSize int
	// appShutdown communicates to the trace observer when the application has
	// completed shutting down
	appShutdown chan struct{}

	// dialer is only used for testing - it allows the trace observer to connect directly
	// to an in-memory gRPC server
	dialer internal.DialerFunc
	// removeBackoff sets the recordSpanBackoff to 0 and is useful for testing
	removeBackoff bool
}

type observerURL struct {
	host   string
	secure bool
}

const (
	localTestingHost = "localhost"
)

var (
	errUnsupportedVersion = errors.New("non supported Go version - to use Infinite Tracing, " +
		"you must use at least version 1.9 or higher of Go")

	errSpanOrDTDisabled = errors.New("in order to enable Infinite Tracing, you must have both " +
		"Distributed Tracing and Span Events enabled")
)

// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import "time"

type customEvents struct {
	*analyticsEvents
}

func newCustomEvents(max int) *customEvents {
	return &customEvents{
		analyticsEvents: newAnalyticsEvents(max),
	}
}

func (cs *customEvents) Add(e *customEvent) {
	// For the Go Agent, customEvents are added to the application, not the transaction.
	// As a result, customEvents do not inherit their priority from the transaction, though
	// they are still sampled according to priority sampling.
	priority := newPriority()
	cs.addEvent(analyticsEvent{priority, e})
}

func (cs *customEvents) MergeIntoHarvest(h *harvest) {
	h.CustomEvents.mergeFailed(cs.analyticsEvents)
}

func (cs *customEvents) Data(agentRunID string, harvestStart time.Time) ([]byte, error) {
	return cs.CollectorJSON(agentRunID)
}

func (cs *customEvents) EndpointMethod() string {
	return cmdCustomEvents
}

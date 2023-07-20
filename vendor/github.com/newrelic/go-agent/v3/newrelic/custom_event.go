// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"fmt"
	"regexp"
	"time"
)

// https://newrelic.atlassian.net/wiki/display/eng/Custom+Events+in+New+Relic+Agents

var (
	eventTypeRegexRaw = `^[a-zA-Z0-9:_ ]+$`
	eventTypeRegex    = regexp.MustCompile(eventTypeRegexRaw)

	errEventTypeLength = fmt.Errorf("event type exceeds length limit of %d",
		attributeKeyLengthLimit)
	// errEventTypeRegex will be returned to caller of app.RecordCustomEvent
	// if the event type is not valid.
	errEventTypeRegex = fmt.Errorf("event type must match %s", eventTypeRegexRaw)
	errNumAttributes  = fmt.Errorf("maximum of %d attributes exceeded",
		customEventAttributeLimit)
)

// customEvent is a custom event.
type customEvent struct {
	eventType       string
	timestamp       time.Time
	truncatedParams map[string]interface{}
}

// WriteJSON prepares JSON in the format expected by the collector.
func (e *customEvent) WriteJSON(buf *bytes.Buffer) {
	w := jsonFieldsWriter{buf: buf}
	buf.WriteByte('[')
	buf.WriteByte('{')
	w.stringField("type", e.eventType)
	w.intField("timestamp", timeToIntMillis(e.timestamp))
	buf.WriteByte('}')

	buf.WriteByte(',')
	buf.WriteByte('{')
	w = jsonFieldsWriter{buf: buf}
	for key, val := range e.truncatedParams {
		writeAttributeValueJSON(&w, key, val)
	}
	buf.WriteByte('}')

	buf.WriteByte(',')
	buf.WriteByte('{')
	buf.WriteByte('}')
	buf.WriteByte(']')
}

// MarshalJSON is used for testing.
func (e *customEvent) MarshalJSON() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 256))

	e.WriteJSON(buf)

	return buf.Bytes(), nil
}

func eventTypeValidate(eventType string) error {
	if len(eventType) > attributeKeyLengthLimit {
		return errEventTypeLength
	}
	if !eventTypeRegex.MatchString(eventType) {
		return errEventTypeRegex
	}
	return nil
}

// CreateCustomEvent creates a custom event.
func createCustomEvent(eventType string, params map[string]interface{}, now time.Time) (*customEvent, error) {
	if err := eventTypeValidate(eventType); nil != err {
		return nil, err
	}

	if len(params) > customEventAttributeLimit {
		return nil, errNumAttributes
	}

	truncatedParams := make(map[string]interface{})
	for key, val := range params {
		val, err := validateUserAttribute(key, val)
		if nil != err {
			return nil, err
		}
		truncatedParams[key] = val
	}

	return &customEvent{
		eventType:       eventType,
		timestamp:       now,
		truncatedParams: truncatedParams,
	}, nil
}

// MergeIntoHarvest implements Harvestable.
func (e *customEvent) MergeIntoHarvest(h *harvest) {
	h.CustomEvents.Add(e)
}

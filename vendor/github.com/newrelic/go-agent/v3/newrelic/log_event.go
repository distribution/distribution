// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/internal/logcontext"
)

const (
	// MaxLogLength is the maximum number of bytes the log message is allowed to be
	MaxLogLength = 32768
)

type logEvent struct {
	priority  priority
	timestamp int64
	severity  string
	message   string
	spanID    string
	traceID   string
}

// LogData contains data fields that are needed to generate log events.
type LogData struct {
	Timestamp int64  // Optional: Unix Millisecond Timestamp; A timestamp will be generated if unset
	Severity  string // Optional: Severity of log being consumed
	Message   string // Optional: Message of log being consumed; Maximum size: 32768 Bytes.
}

// writeJSON prepares JSON in the format expected by the collector.
func (e *logEvent) WriteJSON(buf *bytes.Buffer) {
	w := jsonFieldsWriter{buf: buf}
	buf.WriteByte('{')
	w.stringField(logcontext.LogSeverityFieldName, e.severity)
	w.stringField(logcontext.LogMessageFieldName, e.message)

	if len(e.spanID) > 0 {
		w.stringField(logcontext.LogSpanIDFieldName, e.spanID)
	}
	if len(e.traceID) > 0 {
		w.stringField(logcontext.LogTraceIDFieldName, e.traceID)
	}

	w.needsComma = false
	buf.WriteByte(',')
	w.intField(logcontext.LogTimestampFieldName, e.timestamp)
	buf.WriteByte('}')
}

// MarshalJSON is used for testing.
func (e *logEvent) MarshalJSON() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, logcontext.AverageLogSizeEstimate))
	e.WriteJSON(buf)
	return buf.Bytes(), nil
}

var (
	errNilLogData         = errors.New("log data can not be nil")
	errLogMessageTooLarge = fmt.Errorf("log message can not exceed %d bytes", MaxLogLength)
)

func (data *LogData) toLogEvent() (logEvent, error) {
	if data == nil {
		return logEvent{}, errNilLogData
	}
	if data.Severity == "" {
		data.Severity = logcontext.LogSeverityUnknown
	}
	if len(data.Message) > MaxLogLength {
		return logEvent{}, errLogMessageTooLarge
	}
	if data.Timestamp == 0 {
		data.Timestamp = int64(timeToUnixMilliseconds(time.Now()))
	}

	data.Message = strings.TrimSpace(data.Message)
	data.Severity = strings.TrimSpace(data.Severity)

	event := logEvent{
		priority:  newPriority(),
		message:   data.Message,
		severity:  data.Severity,
		timestamp: data.Timestamp,
	}

	return event, nil
}

func (e *logEvent) MergeIntoHarvest(h *harvest) {
	h.LogEvents.Add(e)
}

const (
	logDecorationErrorHeader = "New Relic failed to decorate a log"
)

var (
	// ErrNilLogBuffer is a type of error that occurs when the New Relic log decorator is passed a nil object when it was
	// expecting a valid, non nil pointer to a log bytes.Buffer object.
	ErrNilLogBuffer = fmt.Errorf("%s: the EnrichLog() function must not be passed a nil byte buffer", logDecorationErrorHeader)

	// ErrNoApplication is a type of error that occurs when the New Relic log decorator is passed a nil New Relic Application
	// when it was expecting a valid, non nil pointer to a New Relic application.
	ErrNoApplication = fmt.Errorf("%s: a non nil application or transaction must be provided to enrich a log", logDecorationErrorHeader)
)

type logEnricherConfig struct {
	app *Application
	txn *Transaction
}

// EnricherOption is a function that configures the enricher based on the source of data it receives.
type EnricherOption func(*logEnricherConfig)

// FromApp configures the log enricher to build a linking payload from an application.
func FromApp(app *Application) EnricherOption {
	return func(cfg *logEnricherConfig) { cfg.app = app }
}

// FromTxn configures the log enricher to build a linking payload from a transaction.
func FromTxn(txn *Transaction) EnricherOption {
	return func(cfg *logEnricherConfig) { cfg.txn = txn }
}

type linkingMetadata struct {
	traceID    string
	spanID     string
	entityGUID string
	hostname   string
	entityName string
}

// EnrichLog appends newrelic linking metadata to a log stored in a byte buffer.
// This should only be used by plugins built for frameworks.
func EnrichLog(buf *bytes.Buffer, opts EnricherOption) error {
	config := logEnricherConfig{}
	opts(&config)

	if buf == nil {
		return ErrNilLogBuffer
	}

	md := linkingMetadata{}

	var app *Application
	var txn *Transaction

	if config.app != nil {
		app = config.app
	} else if config.txn != nil {
		app = config.txn.Application()
		txn = config.txn

		txnMD := txn.thread.GetTraceMetadata()
		md.spanID = txnMD.SpanID
		md.traceID = txnMD.TraceID
	} else {
		return ErrNoApplication
	}

	if app.app == nil {
		return ErrNoApplication
	}

	reply, err := app.app.getState()
	if err != nil {
		return err
	}

	md.entityGUID = reply.Reply.EntityGUID
	md.entityName = app.app.config.AppName
	md.hostname = app.app.config.hostname

	if reply.Config.ApplicationLogging.Enabled && reply.Config.ApplicationLogging.LocalDecorating.Enabled {
		md.appendLinkingMetadata(buf)
	}

	return nil
}

func (md *linkingMetadata) appendLinkingMetadata(buf *bytes.Buffer) {
	if md.entityGUID == "" || md.entityName == "" || md.hostname == "" {
		return
	}

	addDynamicSpacing(buf)

	buf.WriteString("NR-LINKING|")
	if md.traceID != "" && md.spanID != "" {
		buf.WriteString(md.entityGUID)
		buf.WriteByte('|')
		buf.WriteString(md.hostname)
		buf.WriteByte('|')
		buf.WriteString(md.traceID)
		buf.WriteByte('|')
		buf.WriteString(md.spanID)
		buf.WriteByte('|')
		buf.WriteString(md.entityName)
		buf.WriteByte('|')
	} else {
		buf.WriteString(md.entityGUID)
		buf.WriteByte('|')
		buf.WriteString(md.hostname)
		buf.WriteByte('|')
		buf.WriteString(md.entityName)
		buf.WriteByte('|')
	}
}

func addDynamicSpacing(buf *bytes.Buffer) {
	if buf.Len() == 0 {
		return
	}

	bytes := buf.Bytes()
	if bytes[len(bytes)-1] != ' ' {
		buf.WriteByte(' ')
	}
}

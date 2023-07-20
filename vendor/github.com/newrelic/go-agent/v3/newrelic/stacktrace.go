// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"path"
	"runtime"
	"strings"
)

// stackTrace is a stack trace.
type stackTrace []uintptr

// getStackTrace returns a new stackTrace.
func getStackTrace() stackTrace {
	skip := 1 // skip runtime.Callers
	callers := make([]uintptr, maxStackTraceFrames)
	written := runtime.Callers(skip, callers)
	return callers[:written]
}

type StacktraceFrame struct {
	Name string
	File string
	Line int64
}

func (f StacktraceFrame) formattedName() string {
	if strings.HasPrefix(f.Name, "go.") {
		// This indicates an anonymous struct. eg.
		// "go.(*struct { github.com/newrelic/go-agent.threadWithExtras }).NoticeError"
		return f.Name
	}
	return path.Base(f.Name)
}

func (f StacktraceFrame) isAgent() bool {
	// Note this is not a contains conditional rather than a prefix
	// conditional to handle anonymous functions like:
	// "go.(*struct { github.com/newrelic/go-agent.threadWithExtras }).NoticeError"
	return strings.Contains(f.Name, "github.com/newrelic/go-agent/v3/internal.") ||
		strings.Contains(f.Name, "github.com/newrelic/go-agent/v3/newrelic.")
}

func (f StacktraceFrame) WriteJSON(buf *bytes.Buffer) {
	buf.WriteByte('{')
	w := jsonFieldsWriter{buf: buf}
	if f.Name != "" {
		w.stringField("name", f.formattedName())
	}
	if f.File != "" {
		w.stringField("filepath", f.File)
	}
	if f.Line != 0 {
		w.intField("line", f.Line)
	}
	buf.WriteByte('}')
}

func writeFrames(buf *bytes.Buffer, frames []StacktraceFrame) {
	// Remove top agent frames.
	for len(frames) > 0 && frames[0].isAgent() {
		frames = frames[1:]
	}
	// Truncate excessively long stack traces (they may be provided by the
	// customer).
	if len(frames) > maxStackTraceFrames {
		frames = frames[0:maxStackTraceFrames]
	}

	buf.WriteByte('[')
	for idx, frame := range frames {
		if idx > 0 {
			buf.WriteByte(',')
		}
		frame.WriteJSON(buf)
	}
	buf.WriteByte(']')
}

func (st stackTrace) frames() []StacktraceFrame {
	if len(st) == 0 {
		return nil
	}
	frames := runtime.CallersFrames(st) // CallersFrames is only available in Go 1.7+
	fs := make([]StacktraceFrame, 0, maxStackTraceFrames)
	var frame runtime.Frame
	more := true
	for more {
		frame, more = frames.Next()
		fs = append(fs, StacktraceFrame{
			Name: frame.Function,
			File: frame.File,
			Line: int64(frame.Line),
		})
	}
	return fs
}

// WriteJSON adds the stack trace to the buffer in the JSON form expected by the
// collector.
func (st stackTrace) WriteJSON(buf *bytes.Buffer) {
	frames := st.frames()
	writeFrames(buf, frames)
}

// MarshalJSON prepares JSON in the format expected by the collector.
func (st stackTrace) MarshalJSON() ([]byte, error) {
	estimate := 256 * len(st)
	buf := bytes.NewBuffer(make([]byte, 0, estimate))

	st.WriteJSON(buf)

	return buf.Bytes(), nil
}

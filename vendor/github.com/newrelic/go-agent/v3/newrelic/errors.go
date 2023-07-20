// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

// stackTracer can be implemented by errors to provide a stack trace when using
// Transaction.NoticeError.
type stackTracer interface {
	StackTrace() []uintptr
}

// errorClasser can be implemented by errors to provide a custom class when
// using Transaction.NoticeError.
type errorClasser interface {
	ErrorClass() string
}

// errorAttributer can be implemented by errors to provide extra context when
// using Transaction.NoticeError.
type errorAttributer interface {
	ErrorAttributes() map[string]interface{}
}

// Error is an error designed for use with Transaction.NoticeError.  It allows
// direct control over the recorded error's message, class, stacktrace, and
// attributes.
type Error struct {
	// Message is the error message which will be returned by the Error()
	// method.
	Message string
	// Class indicates how the error may be aggregated.
	Class string
	// Attributes are attached to traced errors and error events for
	// additional context.  These attributes are validated just like those
	// added to Transaction.AddAttribute.
	Attributes map[string]interface{}
	// Stack is the stack trace.  Assign this field using NewStackTrace,
	// or leave it nil to indicate that Transaction.NoticeError should
	// generate one.
	Stack []uintptr
}

// NewStackTrace generates a stack trace for the newrelic.Error struct's Stack
// field.
func NewStackTrace() []uintptr {
	st := getStackTrace()
	return []uintptr(st)
}

func (e Error) Error() string { return e.Message }

// ErrorClass returns the error's class.
func (e Error) ErrorClass() string { return e.Class }

// ErrorAttributes returns the error's extra attributes.
func (e Error) ErrorAttributes() map[string]interface{} { return e.Attributes }

// StackTrace returns the error's stack.
func (e Error) StackTrace() []uintptr { return e.Stack }

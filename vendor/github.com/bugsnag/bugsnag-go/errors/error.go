// Package errors provides errors that have stack-traces.
package errors

import (
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"runtime"
)

// The maximum number of stackframes on any error.
var MaxStackDepth = 50

// Error is an error with an attached stacktrace. It can be used
// wherever the builtin error interface is expected.
type Error struct {
	Err    error
	Cause  *Error
	stack  []uintptr
	frames []StackFrame
}

// ErrorWithCallers allows passing in error objects that
// also have caller information attached.
type ErrorWithCallers interface {
	Error() string
	Callers() []uintptr
}

// ErrorWithStackFrames allows the stack to be rebuilt from the stack frames, thus
// allowing to use the Error type when the program counter is not available.
type ErrorWithStackFrames interface {
	Error() string
	StackFrames() []StackFrame
}

type errorWithStack interface {
	StackTrace() errors.StackTrace
	Error() string
}

type errorWithCause interface {
	Unwrap() error
}

// New makes an Error from the given value. If that value is already an
// error then it will be used directly, if not, it will be passed to
// fmt.Errorf("%v"). The skip parameter indicates how far up the stack
// to start the stacktrace. 0 is from the current call, 1 from its caller, etc.
func New(e interface{}, skip int) *Error {
	var err error

	switch e := e.(type) {
	case *Error:
		return e
	case ErrorWithCallers:
		return &Error{
			Err:   e,
			stack: e.Callers(),
			Cause: unwrapCause(e),
		}
	case errorWithStack:
		trace := e.StackTrace()
		stack := make([]uintptr, len(trace))
		for i, ptr := range trace {
			stack[i] = uintptr(ptr) - 1
		}
		return &Error{
			Err:   e,
			Cause: unwrapCause(e),
			stack: stack,
		}
	case ErrorWithStackFrames:
		stack := make([]uintptr, len(e.StackFrames()))
		for i, frame := range e.StackFrames() {
			stack[i] = frame.ProgramCounter
		}
		return &Error{
			Err:    e,
			Cause:  unwrapCause(e),
			stack:  stack,
			frames: e.StackFrames(),
		}
	case error:
		err = e
	default:
		err = fmt.Errorf("%v", e)
	}

	stack := make([]uintptr, MaxStackDepth)
	length := runtime.Callers(2+skip, stack[:])
	return &Error{
		Err:   err,
		Cause: unwrapCause(err),
		stack: stack[:length],
	}
}

// Errorf creates a new error with the given message. You can use it
// as a drop-in replacement for fmt.Errorf() to provide descriptive
// errors in return values.
func Errorf(format string, a ...interface{}) *Error {
	return New(fmt.Errorf(format, a...), 1)
}

// Error returns the underlying error's message.
func (err *Error) Error() string {
	return err.Err.Error()
}

// Callers returns the raw stack frames as returned by runtime.Callers()
func (err *Error) Callers() []uintptr {
	return err.stack[:]
}

// Stack returns the callstack formatted the same way that go does
// in runtime/debug.Stack()
func (err *Error) Stack() []byte {
	buf := bytes.Buffer{}

	for _, frame := range err.StackFrames() {
		buf.WriteString(frame.String())
	}

	return buf.Bytes()
}

// StackFrames returns an array of frames containing information about the
// stack.
func (err *Error) StackFrames() []StackFrame {
	if err.frames == nil {
		callers := runtime.CallersFrames(err.stack)
		err.frames = make([]StackFrame, 0, len(err.stack))
		for frame, more := callers.Next(); more; frame, more = callers.Next() {
			if frame.Func == nil {
				// Ignore fully inlined functions
				continue
			}
			pkg, name := packageAndName(frame.Func)
			err.frames = append(err.frames, StackFrame{
				function:       frame.Func,
				File:           frame.File,
				LineNumber:     frame.Line,
				Name:           name,
				Package:        pkg,
				ProgramCounter: frame.PC,
			})
		}
	}

	return err.frames
}

// TypeName returns the type this error. e.g. *errors.stringError.
func (err *Error) TypeName() string {
	if p, ok := err.Err.(uncaughtPanic); ok {
		return p.typeName
	}
	if name := reflect.TypeOf(err.Err).String(); len(name) > 0 {
		return name
	}
	return "error"
}

func unwrapCause(err interface{}) *Error {
	if causer, ok := err.(errorWithCause); ok {
		cause := causer.Unwrap()
		if cause == nil {
			return nil
		} else if hasStack(cause) { // avoid generating a (duplicate) stack from the current frame
			return New(cause, 0)
		} else {
			return &Error{
				Err:   cause,
				Cause: unwrapCause(cause),
				stack: []uintptr{},
			}
		}
	}
	return nil
}

func hasStack(err error) bool {
	if _, ok := err.(errorWithStack); ok {
		return true
	}
	if _, ok := err.(ErrorWithStackFrames); ok {
		return true
	}
	if _, ok := err.(ErrorWithCallers); ok {
		return true
	}
	return false
}

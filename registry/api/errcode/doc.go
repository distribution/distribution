// Package errcode provides a toolkit for defining and assigning error
// codes to HTTP API responses. An ErrorCode is identified globally
// by a string value, typically all uppercase, by convention. When an
// `ErrorCode` is registered, a value unique to the process is assigned,
// which can be used for identity tests.
//
// The package provides central registration and querying. Along with
// `ErrorDescriptors`, such facilities can be used to populate templates
// and generate API error documentation.
//
// Use of this package is defined by the following flow:
// * Each error is registered with the errcode package via the `Register()`
// function. The `Register()` function takes a `group` name and an
// `ErrorDescriptor` structure. The `group` name allows for errors to be
// associated with a particular component, or any other grouping mechanism
// that may have meaning to the code registering the error.  The
// `ErrorDescriptor` describes the error itself. See below for more
// information. The `Register()` function will return an `ErrorCode`
// that uniquely identifies the register error.
//
// * Once an error is registered, the return `ErrorCode` can be used just like
// any other golang `error` type.
//
// * If a particular error needs to have additional information or processing
// performed then the `WithArgs()` and `WithDetails()` functions are available.
// The `WithArgs()` function allows for the code generating the error to
// specify the substitution values of the `%s` variables in the error's
// message. `WithDetails()` allows for the specification of any additional
// information that may need to be provided to the end-user for this particular
// error.  In both cases, the functions will return an `Error` resource
// which extends the `ErrorCode` resource with the additonal information.
//
// The package consists of three main resource types:
//
// * ErrorCode
// ErrorCode is a unique (numerical) identifier for a particular error
// registered with the `errcode` package. This value is returned by the
// `Register` function.
//
// * ErrorDescriptor
//   ErrorDescriptor describes a single error condition. It contains the
//   following bits of information:
//
//   Code  - a unique (numerical) value for this error condition. This
//   value will be assigned by the errcode's Regsitry function. This is of
//   type `ErrorCode`.
//
//   Value - a unique identifier for this particular error condition.
//   It must be unique across all ErrorDescriptors.
//
//   Message - the human readable sentence that will be displayed for this
//   error. It can contain '%s' substitutions that allows for the code
//   generating the error to specify values that will be inserted in the
//   string prior to being displayed to the end-user. The `WithArgs()`
//   function can be used to specify the insertion strings. Note, the
//   evaluation of the strings will be done at the time `WithArgs()`
//   is called.
//
//   Description - additional human readable text to further explain the
//   circumstances of the error situation.
//
//   HTTPStatusCode - when the error is returned back to a CLI, this value
//   will be used to populate the HTTP status code. If not present the
//   default value will be `StatusInternalServerError`, 500.
//
// * Error
// Error extends an ErrorCode resource with additional information
// such as its substitution variables and details.
package errcode

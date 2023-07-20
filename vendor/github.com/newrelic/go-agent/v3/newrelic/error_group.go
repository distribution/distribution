package newrelic

import "time"

const (
	// The error class for panics
	PanicErrorClass = panicErrorKlass
)

// ErrorInfo contains info for user defined callbacks that are relevant to an error.
// All fields are either safe to access copies of internal agent data, or protected from direct
// access with methods and can not manipulate or distort any agent data.
type ErrorInfo struct {
	errAttributes map[string]interface{}
	txnAttributes *attributes
	stackTrace    stackTrace

	// TransactionName is the formatted name of a transaction that is equivilent to how it appears in
	// the New Relic UI. For example, user defined transactions will be named `OtherTransaction/Go/yourTxnName`.
	TransactionName string

	// Error contains the raw error object that the agent collected.
	//
	// Not all errors collected by the system are collected as
	// error objects, like web/http errors and panics.
	// In these cases, Error will be nil, but details will be captured in
	// the Message and Class fields.
	Error error

	// Time Occured is the time.Time when the error was noticed by the go agent
	TimeOccured time.Time

	// Message will always be populated by a string message describing an error
	Message string

	// Class is a string containing the New Relic error class.
	//
	// If an error implements errorClasser, its value will be derived from that.
	// Otherwise, it will be derived from the way the error was
	// collected by the agent. For http errors, this will be the
	// error number. Panics will be the constant value `newrelic.PanicErrorClass`.
	// If no errorClass was defined, this will be reflect.TypeOf() the root
	// error object, which is commonly `*errors.errorString`.
	Class string

	// Expected is true if the error was expected by the go agent
	Expected bool
}

// GetTransactionUserAttribute safely looks up a user attribute by string key from the parent transaction
// of an error. This function will return the attribute vaue as an interface{}, and a bool indicating whether the
// key was found in the attribute map. If the key was not found, then the return will be (nil, false).
func (e *ErrorInfo) GetTransactionUserAttribute(attribute string) (interface{}, bool) {
	a, b := e.txnAttributes.user[attribute]
	if b {
		return a.value, b
	}

	return nil, b
}

// GetErrorAttribute safely looks up an error attribute by string key. The value of the attribute will be returned
// as an interface{}, and a bool indicating whether the key was found in the attribute map. If no matching key was
// found, the return will be (nil, false).
func (e *ErrorInfo) GetErrorAttribute(attribute string) (interface{}, bool) {
	a, b := e.errAttributes[attribute]
	return a, b
}

// GetStackTraceFrames returns a slice of StacktraceFrame objects containing up to 100 lines of stack trace
// data gathered from the Go runtime. Calling this function may be expensive since it allocates and
// populates a new slice with stack trace data, and should be called only when needed.
func (e *ErrorInfo) GetStackTraceFrames() []StacktraceFrame {
	return e.stackTrace.frames()
}

// GetRequestURI returns the URI of the http request made during the parent transaction of this error. If no web request occured,
// this will return an empty string.
func (e *ErrorInfo) GetRequestURI() string {
	val, ok := e.txnAttributes.Agent[AttributeRequestURI]
	if !ok {
		return ""
	}

	return val.stringVal
}

// GetRequestMethod will return the HTTP method used to make a web request if one occured during the parent transaction
// of this error. If no web request occured, then an empty string will be returned.
func (e *ErrorInfo) GetRequestMethod() string {
	val, ok := e.txnAttributes.Agent[AttributeRequestMethod]
	if !ok {
		return ""
	}

	return val.stringVal
}

// GetHttpResponseCode will return the HTTP response code that resulted from the web request made in the parent transaction of
// this error. If no web request occured, then an empty string will be returned.
func (e *ErrorInfo) GetHttpResponseCode() string {
	val, ok := e.txnAttributes.Agent[AttributeResponseCode]
	if !ok {
		return ""
	}

	code := val.stringVal
	if code != "" {
		return code
	}

	val, ok = e.txnAttributes.Agent[AttributeResponseCodeDeprecated]
	if !ok {
		return ""
	}

	return val.stringVal
}

// GetUserID will return the User ID set for the parent transaction of this error. It will return empty string
// if none was set.
func (e *ErrorInfo) GetUserID() string {
	val, ok := e.txnAttributes.Agent[AttributeUserID]
	if !ok {
		return ""
	}

	return val.stringVal
}

// ErrorGroupCallback is a user defined callback function that takes an error as an input
// and returns a string that will be applied to an error to put it in an error group.
//
// If no error group is identified for a given error, this function should return an empty string.
//
// If an ErrorGroupCallbeck is defined, it will be executed against every error the go agent notices that
// is not ignored.
type ErrorGroupCallback func(ErrorInfo) string

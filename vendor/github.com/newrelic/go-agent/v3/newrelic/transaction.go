// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Transaction instruments one logical unit of work: either an inbound web
// request or background task.  Start a new Transaction with the
// Application.StartTransaction method.
//
// All methods on Transaction are nil safe. Therefore, a nil Transaction
// pointer can be safely used as a mock.
type Transaction struct {
	Private any
	thread  *thread
}

// End finishes the Transaction.  After that, subsequent calls to End or
// other Transaction methods have no effect.  All segments and
// instrumentation must be completed before End is called.
func (txn *Transaction) End() {
	if txn == nil || txn.thread == nil {
		return
	}

	var r any
	if txn.thread.Config.ErrorCollector.RecordPanics {
		// recover must be called in the function directly being deferred,
		// not any nested call!
		r = recover()
	}
	if txn.thread.IsWeb {
		secureAgent.SendEvent("INBOUND_END", "")
	}
	txn.thread.logAPIError(txn.thread.End(r), "end transaction", nil)
}

// SetOption allows the setting of some transaction TraceOption parameters
// after the transaction has already been started, such as specifying a new
// source code location for code-level metrics.
//
// The set of options should be the complete set you wish to have in effect,
// just as if you were calling StartTransaction now with the same set of options.
func (txn *Transaction) SetOption(options ...TraceOption) {
	if txn == nil || txn.thread == nil || txn.thread.txn == nil {
		return
	}
	txn.thread.txn.setOption(options...)
}

// Ignore prevents this transaction's data from being recorded.
func (txn *Transaction) Ignore() {
	if txn == nil || txn.thread == nil {
		return
	}
	txn.thread.logAPIError(txn.thread.Ignore(), "ignore transaction", nil)
}

// SetName names the transaction.  Use a limited set of unique names to
// ensure that Transactions are grouped usefully.
func (txn *Transaction) SetName(name string) {
	if txn == nil || txn.thread == nil {
		return
	}
	txn.thread.logAPIError(txn.thread.SetName(name), "set transaction name", nil)
}

// NoticeError records an error.  The Transaction saves the first five
// errors.  For more control over the recorded error fields, see the
// newrelic.Error type.
//
// In certain situations, using this method may result in an error being
// recorded twice.  Errors are automatically recorded when
// Transaction.WriteHeader receives a status code at or above 400 or strictly
// below 100 that is not in the IgnoreStatusCodes configuration list.  This
// method is unaffected by the IgnoreStatusCodes configuration list.
//
// NoticeError examines whether the error implements the following optional
// methods:
//
//	// StackTrace records a stack trace
//	StackTrace() []uintptr
//
//	// ErrorClass sets the error's class
//	ErrorClass() string
//
//	// ErrorAttributes sets the errors attributes
//	ErrorAttributes() map[string]any
//
// The newrelic.Error type, which implements these methods, is the recommended
// way to directly control the recorded error's message, class, stacktrace,
// and attributes.
func (txn *Transaction) NoticeError(err error) {
	if txn == nil || txn.thread == nil {
		return
	}
	txn.thread.logAPIError(txn.thread.NoticeError(err, false), "notice error", nil)
}

// NoticeExpectedError records an error that was expected to occur. Errors recoreded with this
// method will not trigger any error alerts or count towards your error metrics.
// The Transaction saves the first five errors.
// For more control over the recorded error fields, see the
// newrelic.Error type.
//
// In certain situations, using this method may result in an error being
// recorded twice.  Errors are automatically recorded when
// Transaction.WriteHeader receives a status code at or above 400 or strictly
// below 100 that is not in the IgnoreStatusCodes configuration list.  This
// method is unaffected by the IgnoreStatusCodes configuration list.
//
// NoticeExpectedError examines whether the error implements the following optional
// methods:
//
//	// StackTrace records a stack trace
//	StackTrace() []uintptr
//
//	// ErrorClass sets the error's class
//	ErrorClass() string
//
//	// ErrorAttributes sets the errors attributes
//	ErrorAttributes() map[string]any
//
// The newrelic.Error type, which implements these methods, is the recommended
// way to directly control the recorded error's message, class, stacktrace,
// and attributes.
func (txn *Transaction) NoticeExpectedError(err error) {
	if txn == nil || txn.thread == nil {
		return
	}
	txn.thread.logAPIError(txn.thread.NoticeError(err, true), "notice error", nil)
}

// AddAttribute adds a key value pair to the transaction event, errors,
// and traces.
//
// The key must contain fewer than than 255 bytes.  The value must be a
// number, string, or boolean.
//
// For more information, see:
// https://docs.newrelic.com/docs/agents/manage-apm-agents/agent-metrics/collect-custom-attributes
func (txn *Transaction) AddAttribute(key string, value any) {
	if txn == nil || txn.thread == nil {
		return
	}
	txn.thread.logAPIError(txn.thread.AddAttribute(key, value), "add attribute", nil)
}

// SetUserID is used to track the user that a transaction, and all data that is recorded as a subset of that transaction,
// belong to or interact with. This will propogate an attribute containing this information to all events that are
// a child of this transaction, like errors and spans.
func (txn *Transaction) SetUserID(userID string) {
	if txn == nil || txn.thread == nil {
		return
	}

	txn.thread.logAPIError(txn.thread.AddUserID(userID), "set user ID", nil)
}

// RecordLog records the data from a single log line.
// This consumes a LogData object that should be configured
// with data taken from a logging framework.
//
// Certian parts of this feature can be turned off based on your
// config settings. Record log is capable of recording log events,
// as well as log metrics depending on how your application is
// configured.
func (txn *Transaction) RecordLog(log LogData) {
	event, err := log.toLogEvent()
	if err != nil {
		txn.Application().app.Error("unable to record log", map[string]any{
			"reason": err.Error(),
		})
		return
	}

	metadata := txn.GetTraceMetadata()
	event.spanID = metadata.SpanID
	event.traceID = metadata.TraceID
	txn.thread.StoreLog(&event)
}

// SetWebRequestHTTP marks the transaction as a web transaction.  If
// the request is non-nil, SetWebRequestHTTP will additionally collect
// details on request attributes, url, and method.  If headers are
// present, the agent will look for distributed tracing headers using
// Transaction.AcceptDistributedTraceHeaders.
func (txn *Transaction) SetWebRequestHTTP(r *http.Request) {
	if r == nil {
		txn.SetWebRequest(WebRequest{})
		return
	}
	wr := WebRequest{
		Header:        r.Header,
		URL:           r.URL,
		Method:        r.Method,
		Transport:     transport(r),
		Host:          r.Host,
		Body:          reqBody(r),
		ServerName:    serverName(r),
		Type:          "HTTP",
		RemoteAddress: r.RemoteAddr,
	}
	txn.SetWebRequest(wr)
}

func transport(r *http.Request) TransportType {
	if strings.HasPrefix(r.Proto, "HTTP") {
		if r.TLS != nil {
			return TransportHTTPS
		}
		return TransportHTTP
	}
	return TransportUnknown
}

func serverName(r *http.Request) string {
	if strings.HasPrefix(r.Proto, "HTTP") {
		if r.TLS != nil {
			return r.TLS.ServerName
		}
	}
	return ""
}

func reqBody(req *http.Request) []byte {
	var bodyBuffer bytes.Buffer
	requestBuffer := make([]byte, 0)
	bodyReader := io.TeeReader(req.Body, &bodyBuffer)

	if bodyReader != nil && req.Body != nil {
		reqBuffer, err := io.ReadAll(bodyReader)
		if err == nil {
			requestBuffer = reqBuffer
		}
		r := io.NopCloser(bytes.NewBuffer(requestBuffer))
		req.Body = r
	}
	return bytes.TrimRight(requestBuffer, "\x00")
}

// SetWebRequest marks the transaction as a web transaction.  SetWebRequest
// additionally collects details on request attributes, url, and method if
// these fields are set.  If headers are present, the agent will look for
// distributed tracing headers using Transaction.AcceptDistributedTraceHeaders.
// Use Transaction.SetWebRequestHTTP if you have a *http.Request.
func (txn *Transaction) SetWebRequest(r WebRequest) {
	if txn == nil || txn.thread == nil {
		return
	}
	secureAgent.SendEvent("INBOUND", r)
	txn.thread.logAPIError(txn.thread.SetWebRequest(r), "set web request", nil)
}

// SetWebResponse allows the Transaction to instrument response code and
// response headers.  Use the return value of this method in place of the input
// parameter http.ResponseWriter in your instrumentation.
//
// The returned http.ResponseWriter is safe to use even if the Transaction
// receiver is nil or has already been ended.
//
// The returned http.ResponseWriter implements the combination of
// http.CloseNotifier, http.Flusher, http.Hijacker, and io.ReaderFrom
// implemented by the input http.ResponseWriter.
//
// This method is used by WrapHandle, WrapHandleFunc, and most integration
// package middlewares.  Therefore, you probably want to use this only if you
// are writing your own instrumentation middleware.
func (txn *Transaction) SetWebResponse(w http.ResponseWriter) http.ResponseWriter {
	if txn == nil || txn.thread == nil {
		return w
	}
	return txn.thread.SetWebResponse(w)
}

// StartSegmentNow starts timing a segment.  The SegmentStartTime returned can
// be used as the StartTime field in Segment, DatastoreSegment, or
// ExternalSegment.  The returned SegmentStartTime is safe to use even  when the
// Transaction receiver is nil.  In this case, the segment will have no effect.
func (txn *Transaction) StartSegmentNow() SegmentStartTime {
	return txn.startSegmentAt(time.Now())
}

func (txn *Transaction) startSegmentAt(at time.Time) SegmentStartTime {
	if txn == nil || txn.thread == nil {
		return SegmentStartTime{}
	}
	return txn.thread.startSegmentAt(at)
}

// StartSegment makes it easy to instrument segments.  To time a function, do
// the following:
//
//	func timeMe(txn newrelic.Transaction) {
//		defer txn.StartSegment("timeMe").End()
//		// ... function code here ...
//	}
//
// To time a block of code, do the following:
//
//	segment := txn.StartSegment("myBlock")
//	// ... code you want to time here ...
//	segment.End()
func (txn *Transaction) StartSegment(name string) *Segment {
	if txn != nil && txn.thread != nil && txn.thread.thread != nil && txn.thread.thread.threadID > 0 {
		// async segment start
		secureAgent.SendEvent("NEW_GOROUTINE_LINKER", txn.thread.csecData)
	}
	return &Segment{
		StartTime: txn.StartSegmentNow(),
		Name:      name,
	}
}

// InsertDistributedTraceHeaders adds the Distributed Trace headers used to
// link transactions.  InsertDistributedTraceHeaders should be called every
// time an outbound call is made since the payload contains a timestamp.
//
// When the Distributed Tracer is enabled, InsertDistributedTraceHeaders will
// always insert W3C trace context headers.  It also by default inserts the New Relic
// distributed tracing header, but can be configured based on the
// Config.DistributedTracer.ExcludeNewRelicHeader option.
//
// StartExternalSegment calls InsertDistributedTraceHeaders, so you don't need
// to use it for outbound HTTP calls: Just use StartExternalSegment!
func (txn *Transaction) InsertDistributedTraceHeaders(hdrs http.Header) {
	if txn == nil || txn.thread == nil {
		return
	}
	txn.thread.CreateDistributedTracePayload(hdrs)
}

// AcceptDistributedTraceHeaders links transactions by accepting distributed
// trace headers from another transaction.
//
// Transaction.SetWebRequest and Transaction.SetWebRequestHTTP both call this
// method automatically with the request headers.  Therefore, this method does
// not need to be used for typical HTTP transactions.
//
// AcceptDistributedTraceHeaders should be used as early in the transaction as
// possible.  It may not be called after a call to
// Transaction.InsertDistributedTraceHeaders.
//
// AcceptDistributedTraceHeaders first looks for the presence of W3C trace
// context headers.  Only when those are not found will it look for the New
// Relic distributed tracing header.
func (txn *Transaction) AcceptDistributedTraceHeaders(t TransportType, hdrs http.Header) {
	if txn == nil || txn.thread == nil {
		return
	}
	txn.thread.logAPIError(txn.thread.AcceptDistributedTraceHeaders(t, hdrs), "accept trace payload", nil)
}

// AcceptDistributedTraceHeadersFromJSON works just like AcceptDistributedTraceHeaders(), except
// that it takes the header data as a JSON string à la DistributedTraceHeadersFromJSON(). Additionally
// (unlike AcceptDistributedTraceHeaders()) it returns an error if it was unable to successfully
// convert the JSON string to http headers. There is no guarantee that the header data found in JSON
// is correct beyond conforming to the expected types and syntax.
func (txn *Transaction) AcceptDistributedTraceHeadersFromJSON(t TransportType, jsondata string) error {
	hdrs, err := DistributedTraceHeadersFromJSON(jsondata)
	if err != nil {
		return err
	}
	txn.AcceptDistributedTraceHeaders(t, hdrs)
	return nil
}

// DistributedTraceHeadersFromJSON takes a set of distributed trace headers as a JSON-encoded string
// and emits a http.Header value suitable for passing on to the
// txn.AcceptDistributedTraceHeaders() function.
//
// This is a convenience function provided for cases where you receive the trace header data
// already as a JSON string and want to avoid manually converting that to an http.Header.
// It helps facilitate handling of headers passed to your Go application from components written in other
// languages which may natively handle these header values as JSON strings.
//
// For example, given the input string
//
//	`{"traceparent": "frob", "tracestate": "blorfl", "newrelic": "xyzzy"}`
//
// This will emit an http.Header value with headers "traceparent", "tracestate", and "newrelic".
// Specifically:
//
//	http.Header{
//	  "Traceparent": {"frob"},
//	  "Tracestate": {"blorfl"},
//	  "Newrelic": {"xyzzy"},
//	}
//
// The JSON string must be a single object whose values may be strings or arrays of strings.
// These are translated directly to http headers with singleton or multiple values.
// In the case of multiple string values, these are translated to a multi-value HTTP
// header. For example:
//
//	`{"traceparent": "12345", "colors": ["red", "green", "blue"]}`
//
// which produces
//
//	http.Header{
//	  "Traceparent": {"12345"},
//	  "Colors": {"red", "green", "blue"},
//	}
//
// (Note that the HTTP headers are capitalized.)
func DistributedTraceHeadersFromJSON(jsondata string) (hdrs http.Header, err error) {
	var raw any
	hdrs = http.Header{}
	if jsondata == "" {
		return
	}
	err = json.Unmarshal([]byte(jsondata), &raw)
	if err != nil {
		return
	}

	switch d := raw.(type) {
	case map[string]any:
		for k, v := range d {
			switch hval := v.(type) {
			case string:
				hdrs.Set(k, hval)
			case []any:
				for _, subval := range hval {
					switch sval := subval.(type) {
					case string:
						hdrs.Add(k, sval)
					default:
						err = fmt.Errorf("the JSON object must have only strings or arrays of strings")
						return
					}
				}
			default:
				err = fmt.Errorf("the JSON object must have only strings or arrays of strings")
				return
			}
		}
	default:
		err = fmt.Errorf("the JSON string must consist of only a single object")
		return
	}
	return
}

// Application returns the Application which started the transaction.
func (txn *Transaction) Application() *Application {
	if txn == nil || txn.thread == nil {
		return nil
	}
	return txn.thread.Application()
}

// BrowserTimingHeader generates the JavaScript required to enable New
// Relic's Browser product.  This code should be placed into your pages
// as close to the top of the <head> element as possible, but after any
// position-sensitive <meta> tags (for example, X-UA-Compatible or
// charset information).
//
// This function freezes the transaction name: any calls to SetName()
// after BrowserTimingHeader() will be ignored.
//
// The *BrowserTimingHeader return value will be nil if browser
// monitoring is disabled, the application is not connected, or an error
// occurred.  It is safe to call the pointer's methods if it is nil.
func (txn *Transaction) BrowserTimingHeader() *BrowserTimingHeader {
	if txn == nil || txn.thread == nil {
		return nil
	}
	b, err := txn.thread.BrowserTimingHeader()
	txn.thread.logAPIError(err, "create browser timing header", nil)
	return b
}

// NewGoroutine allows you to use the Transaction in multiple
// goroutines.
//
// Each goroutine must have its own Transaction reference returned by
// NewGoroutine.  You must call NewGoroutine to get a new Transaction
// reference every time you wish to pass the Transaction to another
// goroutine. It does not matter if you call this before or after the
// other goroutine has started.
//
// All Transaction methods can be used in any Transaction reference.
// The Transaction will end when End() is called in any goroutine.
// Note that any segments that end after the transaction ends will not
// be reported.
func (txn *Transaction) NewGoroutine() *Transaction {
	if txn == nil || txn.thread == nil {
		return nil
	}
	newTxn := txn.thread.NewGoroutine()
	if newTxn.thread != nil && newTxn.thread.csecData == nil {
		newTxn.thread.csecData = secureAgent.SendEvent("NEW_GOROUTINE", "")
	}
	return newTxn
}

// GetTraceMetadata returns distributed tracing identifiers.  Empty
// string identifiers are returned if the transaction has finished.
func (txn *Transaction) GetTraceMetadata() TraceMetadata {
	if txn == nil || txn.thread == nil {
		return TraceMetadata{}
	}
	return txn.thread.GetTraceMetadata()
}

// GetLinkingMetadata returns the fields needed to link data to a trace or
// entity.
func (txn *Transaction) GetLinkingMetadata() LinkingMetadata {
	if txn == nil || txn.thread == nil {
		return LinkingMetadata{}
	}
	return txn.thread.GetLinkingMetadata()
}

// IsSampled indicates if the Transaction is sampled.  A sampled
// Transaction records a span event for each segment.  Distributed tracing
// must be enabled for transactions to be sampled.  False is returned if
// the Transaction has finished.
func (txn *Transaction) IsSampled() bool {
	if txn == nil || txn.thread == nil {
		return false
	}
	return txn.thread.IsSampled()
}

const (
	// DistributedTraceNewRelicHeader is the header used by New Relic agents
	// for automatic trace payload instrumentation.
	DistributedTraceNewRelicHeader = "Newrelic"
	// DistributedTraceW3CTraceStateHeader is one of two headers used by W3C
	// trace context
	DistributedTraceW3CTraceStateHeader = "Tracestate"
	// DistributedTraceW3CTraceParentHeader is one of two headers used by W3C
	// trace context
	DistributedTraceW3CTraceParentHeader = "Traceparent"
)

// TransportType is used in Transaction.AcceptDistributedTraceHeaders to
// represent the type of connection that the trace payload was transported
// over.
type TransportType string

// TransportType names used across New Relic agents:
const (
	TransportUnknown TransportType = "Unknown"
	TransportHTTP    TransportType = "HTTP"
	TransportHTTPS   TransportType = "HTTPS"
	TransportKafka   TransportType = "Kafka"
	TransportJMS     TransportType = "JMS"
	TransportIronMQ  TransportType = "IronMQ"
	TransportAMQP    TransportType = "AMQP"
	TransportQueue   TransportType = "Queue"
	TransportOther   TransportType = "Other"
)

func (tt TransportType) toString() string {
	switch tt {
	case TransportHTTP, TransportHTTPS, TransportKafka, TransportJMS, TransportIronMQ, TransportAMQP,
		TransportQueue, TransportOther:
		return string(tt)
	default:
		return string(TransportUnknown)
	}
}

// WebRequest is used to provide request information to Transaction.SetWebRequest.
type WebRequest struct {
	// Header may be nil if you don't have any headers or don't want to
	// transform them to http.Header format.
	Header http.Header
	// URL may be nil if you don't have a URL or don't want to transform
	// it to *url.URL.
	URL *url.URL
	// Method is the request's method.
	Method string
	// If a distributed tracing header is found in the WebRequest.Header,
	// this TransportType will be used in the distributed tracing metrics.
	Transport TransportType
	// This is the value of the `Host` header. Go does not add it to the
	// http.Header object and so must be passed separately.
	Host string

	// The following fields are needed for the secure agent's vulnerability
	// detection features.
	Body          []byte
	ServerName    string
	Type          string
	RemoteAddress string
}

func (webrequest WebRequest) GetHeader() http.Header {
	return webrequest.Header
}

func (webrequest WebRequest) GetURL() *url.URL {
	return webrequest.URL
}

func (webrequest WebRequest) GetMethod() string {
	return webrequest.Method
}

func (webrequest WebRequest) GetTransport() string {
	return webrequest.Transport.toString()
}

func (webrequest WebRequest) GetHost() string {
	return webrequest.Host
}

func (webrequest WebRequest) GetBody() []byte {
	return webrequest.Body
}

func (webrequest WebRequest) GetServerName() string {
	return webrequest.ServerName
}

func (webrequest WebRequest) Type1() string {
	return webrequest.Type
}
func (webrequest WebRequest) GetRemoteAddress() string {
	return webrequest.RemoteAddress
}

// LinkingMetadata is returned by Transaction.GetLinkingMetadata.  It contains
// identifiers needed to link data to a trace or entity.
type LinkingMetadata struct {
	// TraceID identifies the entire distributed trace.  This field is empty
	// if distributed tracing is disabled.
	TraceID string
	// SpanID identifies the currently active segment.  This field is empty
	// if distributed tracing is disabled or the transaction is not sampled.
	SpanID string
	// EntityName is the Application name as set on the Config.  If multiple
	// application names are specified in the Config, only the first is
	// returned.
	EntityName string
	// EntityType is the type of this entity and is always the string
	// "SERVICE".
	EntityType string
	// EntityGUID is the unique identifier for this entity.
	EntityGUID string
	// Hostname is the hostname this entity is running on.
	Hostname string
}

// TraceMetadata is returned by Transaction.GetTraceMetadata.  It contains
// distributed tracing identifiers.
type TraceMetadata struct {
	// TraceID identifies the entire distributed trace.  This field is empty
	// if distributed tracing is disabled.
	TraceID string
	// SpanID identifies the currently active segment.  This field is empty
	// if distributed tracing is disabled or the transaction is not sampled.
	SpanID string
}

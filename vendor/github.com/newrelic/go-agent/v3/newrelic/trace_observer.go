// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// +build go1.9
// This build tag is necessary because GRPC/ProtoBuf libraries only support Go version 1.9 and up.

package newrelic

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/newrelic/go-agent/v3/internal"
	v1 "github.com/newrelic/go-agent/v3/internal/com_newrelic_trace_v1"
)

type gRPCtraceObserver struct {
	initialConnSuccess chan struct{}
	// initConnOnce protects initialConnSuccess from being closed multiple times.
	initConnOnce sync.Once

	initiateShutdown chan struct{}
	// initShutdownOnce protects initiateShutdown from being closed multiple times.
	initShutdownOnce sync.Once

	messages         chan *spanEvent
	restartChan      chan struct{}
	shutdownComplete chan struct{}

	metadata     metadata.MD
	metadataLock sync.Mutex

	// dialOptions are the grpc.DialOptions to be used when calling grpc.Dial.
	dialOptions []grpc.DialOption

	supportability *observerSupport

	observerConfig
}

type observerSupport struct {
	increment chan string
	dump      chan map[string]float64
}

const (
	// versionSupports8T records whether we are using a supported version of Go
	// for Infinite Tracing
	versionSupports8T = true
	grpcVersion       = grpc.Version
	// recordSpanBackoff is the time to wait after a failure on the RecordSpan
	// endpoint before retrying
	recordSpanBackoff = 15 * time.Second
	// numCodes is the total number of grpc.Codes
	numCodes = 17

	licenseMetadataKey = "license_key"
	runIDMetadataKey   = "agent_run_token"

	observerSeen        = "Supportability/InfiniteTracing/Span/Seen"
	observerSent        = "Supportability/InfiniteTracing/Span/Sent"
	observerCodeErr     = "Supportability/InfiniteTracing/Span/gRPC/"
	observerResponseErr = "Supportability/InfiniteTracing/Span/Response/Error"
)

var (
	codeStrings = map[codes.Code]string{
		codes.Code(0):  "OK",
		codes.Code(1):  "CANCELLED",
		codes.Code(2):  "UNKNOWN",
		codes.Code(3):  "INVALID_ARGUMENT",
		codes.Code(4):  "DEADLINE_EXCEEDED",
		codes.Code(5):  "NOT_FOUND",
		codes.Code(6):  "ALREADY_EXISTS",
		codes.Code(7):  "PERMISSION_DENIED",
		codes.Code(8):  "RESOURCE_EXHAUSTED",
		codes.Code(9):  "FAILED_PRECONDITION",
		codes.Code(10): "ABORTED",
		codes.Code(11): "OUT_OF_RANGE",
		codes.Code(12): "UNIMPLEMENTED",
		codes.Code(13): "INTERNAL",
		codes.Code(14): "UNAVAILABLE",
		codes.Code(15): "DATA_LOSS",
		codes.Code(16): "UNAUTHENTICATED",
	}
)

type obsResult struct {
	// shutdown is if the trace observer should shutdown and stop sending
	// spans.
	shutdown bool
	// backoff is true if a backoff should be used before reconnecting to
	// RecordSpan.
	backoff bool
}

func newTraceObserver(runID internal.AgentRunID, requestHeadersMap map[string]string, cfg observerConfig) (traceObserver, error) {
	to := &gRPCtraceObserver{
		messages:           make(chan *spanEvent, cfg.queueSize),
		initialConnSuccess: make(chan struct{}),
		restartChan:        make(chan struct{}, 1),
		initiateShutdown:   make(chan struct{}),
		shutdownComplete:   make(chan struct{}),
		metadata:           newMetadata(runID, cfg.license, requestHeadersMap),
		observerConfig:     cfg,
		supportability:     newObserverSupport(),
		dialOptions:        newDialOptions(cfg),
	}
	go to.handleSupportability()
	go func() {
		to.connectToTraceObserver()

		// Closing shutdownComplete must be done before draining the messages.
		// This prevents spans from being put onto the messages channel while
		// we are trying to empty the channel.
		close(to.shutdownComplete)
		for len(to.messages) > 0 {
			// drain the channel
			<-to.messages
		}
	}()
	return to, nil
}

// newMetadata creates a grpc metadata with proper keys and values for use when
// connecting to RecordSpan.
func newMetadata(runID internal.AgentRunID, license string, requestHeadersMap map[string]string) metadata.MD {
	md := metadata.New(requestHeadersMap)
	md.Set(licenseMetadataKey, license)
	md.Set(runIDMetadataKey, string(runID))
	return md
}

// markInitialConnSuccessful closes the gRPCtraceObserver initialConnSuccess channel and
// is safe to call multiple times.
func (to *gRPCtraceObserver) markInitialConnSuccessful() {
	to.initConnOnce.Do(func() {
		close(to.initialConnSuccess)
	})
}

// startShutdown closes the gRPCtraceObserver initiateShutdown channel and
// is safe to call multiple times.
func (to *gRPCtraceObserver) startShutdown() {
	to.initShutdownOnce.Do(func() {
		close(to.initiateShutdown)
	})
}

func newDialOptions(cfg observerConfig) []grpc.DialOption {
	do := []grpc.DialOption{
		grpc.WithConnectParams(grpc.ConnectParams{
			Backoff: backoff.Config{
				BaseDelay:  15 * time.Second,
				Multiplier: 2,
				MaxDelay:   300 * time.Second,
			},
		}),
	}
	if cfg.endpoint.secure {
		do = append(do, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	} else {
		do = append(do, grpc.WithInsecure())
	}
	if nil != cfg.dialer {
		do = append(do, grpc.WithContextDialer(cfg.dialer))
	}
	return do
}

func (to *gRPCtraceObserver) connectToTraceObserver() {
	conn, err := grpc.Dial(to.endpoint.host, to.dialOptions...)
	if nil != err {
		// this error is unrecoverable and will not be retried
		to.log.Error("trace observer unable to dial grpc endpoint", map[string]interface{}{
			"host": to.endpoint.host,
			"err":  err.Error(),
		})
		return
	}
	defer to.closeConn(conn)

	serviceClient := v1.NewIngestServiceClient(conn)

	for {
		result := to.connectToStream(serviceClient)
		if result.shutdown {
			return
		}
		if result.backoff && !to.removeBackoff {
			time.Sleep(recordSpanBackoff)
		}
	}
}

func (to *gRPCtraceObserver) closeConn(conn *grpc.ClientConn) {
	// Related to https://github.com/grpc/grpc-go/issues/2159
	// If we call conn.Close() immediately, some messages may still be
	// buffered and will never be sent. Initial testing suggests this takes
	// around 150-200ms with a full channel.
	time.Sleep(500 * time.Millisecond)
	if err := conn.Close(); nil != err {
		to.log.Info("closing trace observer connection was not successful", map[string]interface{}{
			"err": err.Error(),
		})
	}
}

func (to *gRPCtraceObserver) connectToStream(serviceClient v1.IngestServiceClient) obsResult {
	to.metadataLock.Lock()
	md := to.metadata
	to.metadataLock.Unlock()
	ctx := metadata.NewOutgoingContext(context.Background(), md)
	spanClient, err := serviceClient.RecordSpan(ctx)
	if nil != err {
		to.log.Error("trace observer unable to create span client", map[string]interface{}{
			"err": err.Error(),
		})
		return obsResult{
			shutdown: false,
			backoff:  true,
		}
	}
	defer to.closeSpanClient(spanClient)
	to.markInitialConnSuccessful()

	responseError := make(chan error, 1)

	go to.rcvResponses(spanClient, responseError)

	for {
		select {
		case msg := <-to.messages:
			result, success := to.trySendSpan(spanClient, msg, responseError)
			if !success {
				return result
			}
		case <-to.restartChan:
			return obsResult{
				shutdown: false,
				backoff:  false,
			}
		case err := <-responseError:
			return obsResult{
				shutdown: errShouldShutdown(err),
				backoff:  errShouldBackoff(err),
			}
		case <-to.initiateShutdown:
			to.drainQueue(spanClient)
			return obsResult{
				shutdown: true,
				backoff:  false,
			}
		}
	}
}

func (to *gRPCtraceObserver) rcvResponses(spanClient v1.IngestService_RecordSpanClient, responseError chan error) {
	for {
		s, err := spanClient.Recv()
		if nil != err {
			// (issue 213) These two specific errors were reported as nuisance
			// but are really harmless so we'll report them as DEBUG level events
			// instead of ERROR.
			// This error comes from our Infinite Tracing load balancers.
			// We believe the EOF error comes from the gRPC getting reset every 30 seconds
			// from the same cause (rebalancing 8T)
			if err.Error() == "rpc error: code = Internal desc = stream terminated by RST_STREAM with error code: NO_ERROR" || err.Error() == "EOF" {
				to.log.Debug("trace observer response error", map[string]interface{}{
					"err": err.Error(),
				})
			} else {
				to.log.Error("trace observer response error", map[string]interface{}{
					"err": err.Error(),
				})
			}

			// NOTE: even when the trace observer is shutting down
			// properly, an EOF error will be received here and a
			// supportability metric created.
			to.supportabilityError(err)
			responseError <- err
			return
		}
		to.log.Debug("trace observer response", map[string]interface{}{
			"messages_seen": s.GetMessagesSeen(),
		})
	}
}

func (to *gRPCtraceObserver) drainQueue(spanClient v1.IngestService_RecordSpanClient) {
	numSpans := len(to.messages)
	for i := 0; i < numSpans; i++ {
		msg := <-to.messages
		if err := to.sendSpan(spanClient, msg); err != nil {
			// if we fail to send a span, do not send the rest
			break
		}
	}
}

func (to *gRPCtraceObserver) trySendSpan(spanClient v1.IngestService_RecordSpanClient, msg *spanEvent, responseError chan error) (obsResult, bool) {
	if sendErr := to.sendSpan(spanClient, msg); sendErr != nil {
		// When send closes so does recv. Check the error on recv
		// because it could be a shutdown request when the error from
		// send was not.
		var respErr error
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		select {
		case respErr = <-responseError:
		case <-ticker.C:
			to.log.Debug("timeout waiting for response error from trace observer", nil)
		}
		return obsResult{
			shutdown: errShouldShutdown(sendErr) || errShouldShutdown(respErr),
			backoff:  errShouldBackoff(sendErr) || errShouldBackoff(respErr),
		}, false
	}
	return obsResult{}, true
}

func (to *gRPCtraceObserver) closeSpanClient(spanClient v1.IngestService_RecordSpanClient) {
	to.log.Debug("closing trace observer sender", map[string]interface{}{})
	if err := spanClient.CloseSend(); err != nil {
		to.log.Debug("error closing trace observer sender", map[string]interface{}{
			"err": err.Error(),
		})
	}
}

// restart enqueues a request to restart with a new run ID
func (to *gRPCtraceObserver) restart(runID internal.AgentRunID, requestHeadersMap map[string]string) {
	to.metadataLock.Lock()
	to.metadata = newMetadata(runID, to.license, requestHeadersMap)
	to.metadataLock.Unlock()

	// If there is already a restart on the channel, we don't need to add another
	select {
	case to.restartChan <- struct{}{}:
	default:
	}
}

var errTimeout = errors.New("timeout exceeded while waiting for trace observer shutdown to complete")

// shutdown initiates a shutdown of the trace observer and blocks until either
// shutdown is complete (including draining existing spans from the messages channel)
// or the given timeout is hit.
func (to *gRPCtraceObserver) shutdown(timeout time.Duration) error {
	to.startShutdown()
	ticker := time.NewTicker(timeout)
	defer ticker.Stop()
	// Block until the observer shutdown is complete or timeout hit
	select {
	case <-to.shutdownComplete:
		return nil
	case <-ticker.C:
		return errTimeout
	}
}

// initialConnCompleted indicates that the initial connection to the remote trace
// observer was made, but it does NOT indicate anything about the current state of the
// connection
func (to *gRPCtraceObserver) initialConnCompleted() bool {
	select {
	case <-to.initialConnSuccess:
		return true
	default:
		return false
	}
}

// errShouldShutdown returns true if the given error is an Unimplemented error
// meaning the connection to the trace observer should be shutdown.
func errShouldShutdown(err error) bool {
	return status.Code(err) == codes.Unimplemented
}

// errShouldBackoff returns true if the given error should cause the trace
// observer to retry the connection after a backoff period.
func errShouldBackoff(err error) bool {
	return status.Code(err) != codes.OK && err != io.EOF
}

func (to *gRPCtraceObserver) sendSpan(spanClient v1.IngestService_RecordSpanClient, msg *spanEvent) error {
	span := transformEvent(msg)
	to.supportability.increment <- observerSent
	if err := spanClient.Send(span); err != nil {
		to.log.Error("trace observer send error", map[string]interface{}{
			"err": err.Error(),
		})
		to.supportabilityError(err)
		return err
	}
	return nil
}

func (to *gRPCtraceObserver) handleSupportability() {
	metrics := newSupportMetrics()
	for {
		select {
		case <-to.appShutdown:
			// Only close this goroutine once the application _and_ the trace
			// observer have shutdown. This is because we will want to continue
			// to increment the Seen/Sent metrics when the application is
			// running but the trace observer is not.
			return
		case key := <-to.supportability.increment:
			metrics[key]++
		case to.supportability.dump <- metrics:
			// reset the metrics map
			metrics = newSupportMetrics()
		}
	}
}

func newSupportMetrics() map[string]float64 {
	// grpc codes, plus 2 for seen/sent, plus 1 for response errs
	metrics := make(map[string]float64, numCodes+3)
	// these two metrics must always be sent
	metrics[observerSeen] = 0
	metrics[observerSent] = 0
	return metrics
}

func newObserverSupport() *observerSupport {
	return &observerSupport{
		increment: make(chan string),
		dump:      make(chan map[string]float64),
	}
}

// dumpSupportabilityMetrics reads the current supportability metrics off of
// the channel and resets them to 0.
func (to *gRPCtraceObserver) dumpSupportabilityMetrics() map[string]float64 {
	if to.isAppShutdownComplete() {
		return nil
	}
	return <-to.supportability.dump
}

func errToCodeString(err error) string {
	code := status.Code(err)
	str, ok := codeStrings[code]
	if !ok {
		str = strings.ToUpper(code.String())
	}
	return str
}

func (to *gRPCtraceObserver) supportabilityError(err error) {
	to.supportability.increment <- observerCodeErr + errToCodeString(err)
	to.supportability.increment <- observerResponseErr
}

func obsvString(s string) *v1.AttributeValue {
	return &v1.AttributeValue{Value: &v1.AttributeValue_StringValue{StringValue: s}}
}

func obsvBool(b bool) *v1.AttributeValue {
	return &v1.AttributeValue{Value: &v1.AttributeValue_BoolValue{BoolValue: b}}
}

func obsvInt(x int64) *v1.AttributeValue {
	return &v1.AttributeValue{Value: &v1.AttributeValue_IntValue{IntValue: x}}
}

func obsvDouble(x float64) *v1.AttributeValue {
	return &v1.AttributeValue{Value: &v1.AttributeValue_DoubleValue{DoubleValue: x}}
}

func transformEvent(e *spanEvent) *v1.Span {
	span := &v1.Span{
		TraceId:         e.TraceID,
		Intrinsics:      make(map[string]*v1.AttributeValue),
		UserAttributes:  make(map[string]*v1.AttributeValue),
		AgentAttributes: make(map[string]*v1.AttributeValue),
	}

	span.Intrinsics["type"] = obsvString("Span")
	span.Intrinsics["traceId"] = obsvString(e.TraceID)
	span.Intrinsics["guid"] = obsvString(e.GUID)
	if "" != e.ParentID {
		span.Intrinsics["parentId"] = obsvString(e.ParentID)
	}
	span.Intrinsics["transactionId"] = obsvString(e.TransactionID)
	span.Intrinsics["sampled"] = obsvBool(e.Sampled)
	span.Intrinsics["priority"] = obsvDouble(float64(e.Priority.Float32()))
	span.Intrinsics["timestamp"] = obsvInt(e.Timestamp.UnixNano() / (1000 * 1000)) // in milliseconds
	span.Intrinsics["duration"] = obsvDouble(e.Duration.Seconds())
	span.Intrinsics["name"] = obsvString(e.Name)
	span.Intrinsics["category"] = obsvString(string(e.Category))
	if e.IsEntrypoint {
		span.Intrinsics["nr.entryPoint"] = obsvBool(true)
	}
	if e.Component != "" {
		span.Intrinsics["component"] = obsvString(e.Component)
	}
	if e.Kind != "" {
		span.Intrinsics["span.kind"] = obsvString(e.Kind)
	}
	if "" != e.TrustedParentID {
		span.Intrinsics["trustedParentId"] = obsvString(e.TrustedParentID)
	}
	if "" != e.TracingVendors {
		span.Intrinsics["tracingVendors"] = obsvString(e.TracingVendors)
	}
	if "" != e.TxnName {
		span.Intrinsics["transaction.name"] = obsvString(e.TxnName)
	}

	copyAttrs(e.AgentAttributes, span.AgentAttributes)
	copyAttrs(e.UserAttributes, span.UserAttributes)

	return span
}

func copyAttrs(source spanAttributeMap, dest map[string]*v1.AttributeValue) {
	for key, val := range source {
		switch v := val.(type) {
		case stringJSONWriter:
			dest[key] = obsvString(string(v))
		case intJSONWriter:
			dest[key] = obsvInt(int64(v))
		case boolJSONWriter:
			dest[key] = obsvBool(bool(v))
		case floatJSONWriter:
			dest[key] = obsvDouble(float64(v))
		default:
			b := bytes.Buffer{}
			val.WriteJSON(&b)
			s := strings.Trim(b.String(), `"`)
			dest[key] = obsvString(s)
		}
	}
}

// consumeSpan enqueues the span to be sent to the remote trace observer
func (to *gRPCtraceObserver) consumeSpan(span *spanEvent) {
	if to.isAppShutdownComplete() {
		return
	}

	to.supportability.increment <- observerSeen

	if to.isShutdownInitiated() {
		return
	}

	select {
	case to.messages <- span:
	default:
		if to.log.DebugEnabled() {
			to.log.Debug("could not send span to trace observer because channel is full", map[string]interface{}{
				"channel size": to.queueSize,
			})
		}
	}

	return
}

// isShutdownComplete returns a bool if the trace observer has been shutdown.
func (to *gRPCtraceObserver) isShutdownComplete() bool {
	return isChanClosed(to.shutdownComplete)
}

// isShutdownInitiated returns a bool if the trace observer has started
// shutting down.
func (to *gRPCtraceObserver) isShutdownInitiated() bool {
	return isChanClosed(to.initiateShutdown)
}

// isAppShutdownComplete returns a bool if the trace observer's application has
// been shutdown.
func (to *gRPCtraceObserver) isAppShutdownComplete() bool {
	return isChanClosed(to.appShutdown)
}

func isChanClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
	}
	return false
}

// The following functions are only used in testing, but are required during compile time in
// expect_implementation.go, so they are included here rather than in trace_observer_impl_test.go

func expectObserverEvents(v internal.Validator, events *analyticsEvents, expect []internal.WantEvent, extraAttributes map[string]interface{}) {
	for i, e := range expect {
		if nil != e.Intrinsics {
			e.Intrinsics = mergeAttributes(extraAttributes, e.Intrinsics)
		}
		event := events.events[i].jsonWriter.(*spanEvent)
		expectObserverEvent(v, event, e)
	}
}

func expectObserverEvent(v internal.Validator, e *spanEvent, expect internal.WantEvent) {
	span := transformEvent(e)
	if nil != expect.Intrinsics {
		expectObserverAttributes(v, span.Intrinsics, expect.Intrinsics)
	}
	if nil != expect.UserAttributes {
		expectObserverAttributes(v, span.UserAttributes, expect.UserAttributes)
	}
	if nil != expect.AgentAttributes {
		expectObserverAttributes(v, span.AgentAttributes, expect.AgentAttributes)
	}
}

func expectObserverAttributes(v internal.Validator, actual map[string]*v1.AttributeValue, expect map[string]interface{}) {
	if len(actual) != len(expect) {
		v.Error("attributes length difference in trace observer. actual:", len(actual), "expect:", len(expect))
	}
	for key, val := range expect {
		found, ok := actual[key]
		if !ok {
			v.Error("expected attribute not found in trace observer: ", key)
			continue
		}
		if val == internal.MatchAnything {
			continue
		}
		switch exp := val.(type) {
		case bool:
			if f := found.GetBoolValue(); f != exp {
				v.Error("incorrect bool value for key", key, "in trace observer. actual:", f, "expect:", exp)
			}
		case string:
			if f := found.GetStringValue(); f != exp {
				v.Error("incorrect string value for key", key, "in trace observer. actual:", f, "expect:", exp)
			}
		case float64:
			plusOrMinus := 0.0000001 // with floating point math we can only get so close
			if f := found.GetDoubleValue(); f-exp > plusOrMinus || exp-f > plusOrMinus {
				v.Error("incorrect double value for key", key, "in trace observer. actual:", f, "expect:", exp)
			}
		case int:
			if f := found.GetIntValue(); f != int64(exp) {
				v.Error("incorrect int value for key", key, "in trace observer. actual:", f, "expect:", exp)
			}
		default:
			v.Error("unknown type for key", key, "in trace observer. expected:", exp)
		}
	}
	for key, val := range actual {
		_, ok := expect[key]
		if !ok {
			v.Error("unexpected attribute present in trace observer. key:", key, "value:", val)
			continue
		}
	}
}

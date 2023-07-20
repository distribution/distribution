// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

// This file contains the names of the automatically captured attributes.
// Attributes are key value pairs attached to transaction events, error events,
// traced errors, and spans.  You may add your own attributes using the
// Transaction.AddAttribute method (see transaction.go).
//
// These attribute names are exposed here to facilitate configuration.
//
// For more information, see:
// https://docs.newrelic.com/docs/agents/manage-apm-agents/agent-metrics/agent-attributes

// Attributes destined for Transaction Events, Errors, and Transaction Traces:
const (
	// AttributeResponseCode is the response status code for a web request.
	AttributeResponseCode = "http.statusCode"
	// AttributeResponseCodeDeprecated is the response status code for a web
	// request, the same value as AttributeResponseCode. To completely exclude
	// this value from a destination, both AttributeResponseCode and
	// AttributeResponseCodeDeprecated must be specified.
	//
	// Deprecated: This attribute is currently deprecated and will be removed
	// in a later release.
	AttributeResponseCodeDeprecated = "httpResponseCode"
	// AttributeRequestMethod is the request's method.
	AttributeRequestMethod = "request.method"
	// AttributeRequestAccept is the request's "Accept" header.
	AttributeRequestAccept = "request.headers.accept"
	// AttributeRequestContentType is the request's "Content-Type" header.
	AttributeRequestContentType = "request.headers.contentType"
	// AttributeRequestContentLength is the request's "Content-Length" header.
	AttributeRequestContentLength = "request.headers.contentLength"
	// AttributeRequestHost is the request's "Host" header.
	AttributeRequestHost = "request.headers.host"
	// AttributeRequestURI is the request's URL without query parameters,
	// fragment, user, or password.
	AttributeRequestURI = "request.uri"
	// AttributeResponseContentType is the response "Content-Type" header.
	AttributeResponseContentType = "response.headers.contentType"
	// AttributeResponseContentLength is the response "Content-Length" header.
	AttributeResponseContentLength = "response.headers.contentLength"
	// AttributeHostDisplayName contains the value of Config.HostDisplayName.
	AttributeHostDisplayName = "host.displayName"
	// AttributeCodeFunction contains the Code Level Metrics function name.
	AttributeCodeFunction = "code.function"
	// AttributeCodeNamespace contains the Code Level Metrics namespace name.
	AttributeCodeNamespace = "code.namespace"
	// AttributeCodeFilepath contains the Code Level Metrics source file path name.
	AttributeCodeFilepath = "code.filepath"
	// AttributeCodeLineno contains the Code Level Metrics source file line number name.
	AttributeCodeLineno = "code.lineno"
	// AttributeErrorGroupName contains the error group name set by the user defined callback function.
	AttributeErrorGroupName = "error.group.name"
	// AttributeUserID tracks the user a transaction and its child events are impacting
	AttributeUserID = "enduser.id"
)

// Attributes destined for Errors and Transaction Traces:
const (
	// AttributeRequestUserAgent is the request's "User-Agent" header.
	AttributeRequestUserAgent = "request.headers.userAgent"
	// AttributeRequestUserAgentDeprecated is the request's "User-Agent"
	// header, the same value as AttributeRequestUserAgent. To completely
	// exclude this value from a destination, both AttributeRequestUserAgent
	// and AttributeRequestUserAgentDeprecated must be specified.
	//
	// Deprecated: This attribute is currently deprecated and will be removed
	// in a later release.
	AttributeRequestUserAgentDeprecated = "request.headers.User-Agent"
	// AttributeRequestReferer is the request's "Referer" header.  Query
	// string parameters are removed.
	AttributeRequestReferer = "request.headers.referer"
)

// AWS Lambda specific attributes:
const (
	AttributeAWSRequestID            = "aws.requestId"
	AttributeAWSLambdaARN            = "aws.lambda.arn"
	AttributeAWSLambdaColdStart      = "aws.lambda.coldStart"
	AttributeAWSLambdaEventSourceARN = "aws.lambda.eventSource.arn"
)

// Attributes for consumed message transactions:
//
// When a message is consumed (for example from Kafka or RabbitMQ), supported
// instrumentation packages -- i.e. those found in the v3/integrations
// (https://godoc.org/github.com/newrelic/go-agent/v3/integrations) directory --
// will add these attributes automatically.  AttributeMessageExchangeType,
// AttributeMessageReplyTo, and AttributeMessageCorrelationID are disabled
// by default.  To see these attributes added to all destinations, you must add
// include them in your config settings:
//
//	cfg.Attributes.Include = append(cfg.Attributes.Include,
//		newrelic.AttributeMessageExchangeType,
//		newrelic.AttributeMessageReplyTo,
//		newrelic.AttributeMessageCorrelationID,
//	)
//
// When not using a supported instrumentation package, you can add these
// attributes manually using the Transaction.AddAttribute
// (https://godoc.org/github.com/newrelic/go-agent/v3/newrelic#Transaction.AddAttribute)
// API.  In this case, these attributes will be included on all destintations
// by default.
//
//	txn := app.StartTransaction("Message/RabbitMQ/Exchange/Named/MyExchange")
//	txn.AddAttribute(newrelic.AttributeMessageRoutingKey, "myRoutingKey")
//	txn.AddAttribute(newrelic.AttributeMessageQueueName, "myQueueName")
//	txn.AddAttribute(newrelic.AttributeMessageExchangeType, "myExchangeType")
//	txn.AddAttribute(newrelic.AttributeMessageReplyTo, "myReplyTo")
//	txn.AddAttribute(newrelic.AttributeMessageCorrelationID, "myCorrelationID")
//	// ... consume a message ...
//	txn.End()
//
// It is recommended that at most one message is consumed per transaction.
const (
	// The routing key of the consumed message.
	AttributeMessageRoutingKey = "message.routingKey"
	// The name of the queue the message was consumed from.
	AttributeMessageQueueName = "message.queueName"
	// The type of exchange used for the consumed message (direct, fanout,
	// topic, or headers).
	AttributeMessageExchangeType = "message.exchangeType"
	// The callback queue used in RPC configurations.
	AttributeMessageReplyTo = "message.replyTo"
	// The application-generated identifier used in RPC configurations.
	AttributeMessageCorrelationID = "message.correlationId"
)

// Attributes destined for Span Events. These attributes appear only on Span
// Events and are not available to transaction events, error events, or traced
// errors.
//
// To disable the capture of one of these span event attributes, "db.statement"
// for example, modify your Config like this:
//
//	cfg.SpanEvents.Attributes.Exclude = append(cfg.SpanEvents.Attributes.Exclude,
//		newrelic.SpanAttributeDBStatement)
const (
	SpanAttributeDBStatement             = "db.statement"
	SpanAttributeDBInstance              = "db.instance"
	SpanAttributeDBCollection            = "db.collection"
	SpanAttributePeerAddress             = "peer.address"
	SpanAttributePeerHostname            = "peer.hostname"
	SpanAttributeHTTPURL                 = "http.url"
	SpanAttributeHTTPMethod              = "http.method"
	SpanAttributeAWSOperation            = "aws.operation"
	SpanAttributeAWSRegion               = "aws.region"
	SpanAttributeErrorClass              = "error.class"
	SpanAttributeErrorMessage            = "error.message"
	SpanAttributeParentType              = "parent.type"
	SpanAttributeParentApp               = "parent.app"
	SpanAttributeParentAccount           = "parent.account"
	SpanAttributeParentTransportDuration = "parent.transportDuration"
	SpanAttributeParentTransportType     = "parent.transportType"

	// Deprecated: This attribute is a duplicate of AttributeResponseCode and
	// will be removed in a later release.
	SpanAttributeHTTPStatusCode = "http.statusCode"
	// Deprecated: This attribute is a duplicate of AttributeAWSRequestID and
	// will be removed in a later release.
	SpanAttributeAWSRequestID = "aws.requestId"
)

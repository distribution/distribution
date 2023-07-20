// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import "io"

// This file contains interfaces that are implemented by Transaction and
// Application but not exposed as public methods so they will only be used in
// integration packages.

// ServerlessWriter is implemented by newrelic.Application.
type ServerlessWriter interface {
	ServerlessWrite(arn string, writer io.Writer)
}

// ServerlessWrite exists to avoid type assertion in the nrlambda integration
// package.
func ServerlessWrite(app interface{}, arn string, writer io.Writer) {
	if s, ok := app.(ServerlessWriter); ok {
		s.ServerlessWrite(arn, writer)
	}
}

// AddAgentAttributer allows instrumentation to add agent attributes without
// exposing a Transaction method.
type AddAgentAttributer interface {
	AddAgentAttribute(name string, stringVal string, otherVal interface{})
}

// AddAgentSpanAttributer should be implemented by the Transaction.
type AddAgentSpanAttributer interface {
	AddAgentSpanAttribute(key string, val string)
}

// AddAgentSpanAttribute allows instrumentation packages to add span attributes.
func AddAgentSpanAttribute(txn interface{}, key string, val string) {
	if aa, ok := txn.(AddAgentSpanAttributer); ok {
		aa.AddAgentSpanAttribute(key, val)
	}
}

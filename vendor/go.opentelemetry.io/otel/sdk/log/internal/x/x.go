// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package x contains support for Logs SDK experimental features.
package x // import "go.opentelemetry.io/otel/sdk/log/internal/x"

import (
	"context"

	"go.opentelemetry.io/otel/log"
)

// FilterProcessor is a [go.opentelemetry.io/otel/sdk/log.Processor] that knows,
// and can identify, what [log.Record] it will process or drop when it is
// passed to OnEmit.
//
// This is useful for users of logging libraries that want to know if a [log.Record]
// will be processed or dropped before they perform complex operations to
// construct the [log.Record].
//
// Processor implementations that choose to support this by satisfying this
// interface are expected to re-evaluate the [log.Record]s passed to OnEmit, it is
// not expected that the caller to OnEmit will use the functionality from this
// interface prior to calling OnEmit.
//
// This should only be implemented for Processors that can make reliable
// enough determination of this prior to processing a [log.Record] and where
// the result is dynamic.
//
// [Processor]: https://pkg.go.dev/go.opentelemetry.io/otel/sdk/log#Processor
type FilterProcessor interface {
	// Enabled returns whether the Processor will process for the given context
	// and param.
	//
	// The passed param is likely to be a partial record with only the
	// bridge-relevant information being provided (e.g a record with only the
	// Severity set). If a Logger needs more information than is provided, it
	// is said to be in an indeterminate state (see below).
	//
	// The returned value will be true when the Processor will process for the
	// provided context and param, and will be false if the Processor will not
	// process. An implementation should default to returning true for an
	// indeterminate state.
	//
	// Implementations should not modify the param.
	Enabled(ctx context.Context, param log.EnabledParameters) bool
}

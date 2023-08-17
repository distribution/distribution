// Copyright The OpenTelemetry Authors
// Copyright 2016 Michal Witkowski. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package configgrpc // import "go.opentelemetry.io/collector/config/configgrpc"

import (
	"context"

	"google.golang.org/grpc"
)

// this functionality was originally copied from grpc-ecosystem/go-grpc-middleware project

// wrappedServerStream is a thin wrapper around grpc.ServerStream that allows modifying context.
type wrappedServerStream struct {
	grpc.ServerStream
	// wrappedContext is the wrapper's own Context. You can assign it.
	wrappedCtx context.Context
}

// Context returns the wrapper's wrappedContext, overwriting the nested grpc.ServerStream.Context()
func (w *wrappedServerStream) Context() context.Context {
	return w.wrappedCtx
}

// wrapServerStream returns a ServerStream with the new context.
func wrapServerStream(wrappedCtx context.Context, stream grpc.ServerStream) *wrappedServerStream {
	if existing, ok := stream.(*wrappedServerStream); ok {
		existing.wrappedCtx = wrappedCtx
		return existing
	}
	return &wrappedServerStream{ServerStream: stream, wrappedCtx: wrappedCtx}
}

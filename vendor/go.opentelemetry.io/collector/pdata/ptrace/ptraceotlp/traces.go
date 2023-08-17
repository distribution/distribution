// Copyright The OpenTelemetry Authors
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

package ptraceotlp // import "go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

import (
	"bytes"
	"context"

	"github.com/gogo/protobuf/jsonpb"
	"google.golang.org/grpc"

	"go.opentelemetry.io/collector/pdata/internal"
	otlpcollectortrace "go.opentelemetry.io/collector/pdata/internal/data/protogen/collector/trace/v1"
	"go.opentelemetry.io/collector/pdata/internal/otlp"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var jsonMarshaler = &jsonpb.Marshaler{}
var jsonUnmarshaler = &jsonpb.Unmarshaler{}

// Response represents the response for gRPC/HTTP client/server.
type Response struct {
	orig *otlpcollectortrace.ExportTraceServiceResponse
}

// NewResponse returns an empty Response.
func NewResponse() Response {
	return Response{orig: &otlpcollectortrace.ExportTraceServiceResponse{}}
}

// MarshalProto marshals Response into proto bytes.
func (tr Response) MarshalProto() ([]byte, error) {
	return tr.orig.Marshal()
}

// UnmarshalProto unmarshalls Response from proto bytes.
func (tr Response) UnmarshalProto(data []byte) error {
	return tr.orig.Unmarshal(data)
}

// MarshalJSON marshals Response into JSON bytes.
func (tr Response) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := jsonMarshaler.Marshal(&buf, tr.orig); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalJSON unmarshalls Response from JSON bytes.
func (tr Response) UnmarshalJSON(data []byte) error {
	return jsonUnmarshaler.Unmarshal(bytes.NewReader(data), tr.orig)
}

// Request represents the request for gRPC/HTTP client/server.
// It's a wrapper for ptrace.Traces data.
type Request struct {
	orig *otlpcollectortrace.ExportTraceServiceRequest
}

// NewRequest returns an empty Request.
func NewRequest() Request {
	return Request{orig: &otlpcollectortrace.ExportTraceServiceRequest{}}
}

// NewRequestFromTraces returns a Request from ptrace.Traces.
// Because Request is a wrapper for ptrace.Traces,
// any changes to the provided Traces struct will be reflected in the Request and vice versa.
func NewRequestFromTraces(t ptrace.Traces) Request {
	return Request{orig: internal.TracesToOtlp(t)}
}

// MarshalProto marshals Request into proto bytes.
func (tr Request) MarshalProto() ([]byte, error) {
	return tr.orig.Marshal()
}

// UnmarshalProto unmarshalls Request from proto bytes.
func (tr Request) UnmarshalProto(data []byte) error {
	if err := tr.orig.Unmarshal(data); err != nil {
		return err
	}
	otlp.InstrumentationLibrarySpansToScope(tr.orig.ResourceSpans)
	return nil
}

// MarshalJSON marshals Request into JSON bytes.
func (tr Request) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	if err := jsonMarshaler.Marshal(&buf, tr.orig); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalJSON unmarshalls Request from JSON bytes.
func (tr Request) UnmarshalJSON(data []byte) error {
	if err := jsonUnmarshaler.Unmarshal(bytes.NewReader(data), tr.orig); err != nil {
		return err
	}
	otlp.InstrumentationLibrarySpansToScope(tr.orig.ResourceSpans)
	return nil
}

func (tr Request) Traces() ptrace.Traces {
	return internal.TracesFromOtlp(tr.orig)
}

// Client is the client API for OTLP-GRPC Traces service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type Client interface {
	// Export ptrace.Traces to the server.
	//
	// For performance reasons, it is recommended to keep this RPC
	// alive for the entire life of the application.
	Export(ctx context.Context, request Request, opts ...grpc.CallOption) (Response, error)
}

type tracesClient struct {
	rawClient otlpcollectortrace.TraceServiceClient
}

// NewClient returns a new Client connected using the given connection.
func NewClient(cc *grpc.ClientConn) Client {
	return &tracesClient{rawClient: otlpcollectortrace.NewTraceServiceClient(cc)}
}

// Export implements the Client interface.
func (c *tracesClient) Export(ctx context.Context, request Request, opts ...grpc.CallOption) (Response, error) {
	rsp, err := c.rawClient.Export(ctx, request.orig, opts...)
	return Response{orig: rsp}, err
}

// Server is the server API for OTLP gRPC TracesService service.
type Server interface {
	// Export is called every time a new request is received.
	//
	// For performance reasons, it is recommended to keep this RPC
	// alive for the entire life of the application.
	Export(context.Context, Request) (Response, error)
}

// RegisterServer registers the Server to the grpc.Server.
func RegisterServer(s *grpc.Server, srv Server) {
	otlpcollectortrace.RegisterTraceServiceServer(s, &rawTracesServer{srv: srv})
}

type rawTracesServer struct {
	srv Server
}

func (s rawTracesServer) Export(ctx context.Context, request *otlpcollectortrace.ExportTraceServiceRequest) (*otlpcollectortrace.ExportTraceServiceResponse, error) {
	otlp.InstrumentationLibrarySpansToScope(request.ResourceSpans)
	rsp, err := s.srv.Export(ctx, Request{orig: request})
	return rsp.orig, err
}

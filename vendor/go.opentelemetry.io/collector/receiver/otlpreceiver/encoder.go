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

package otlpreceiver // import "go.opentelemetry.io/collector/receiver/otlpreceiver"

import (
	"bytes"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	spb "google.golang.org/genproto/googleapis/rpc/status"

	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

const (
	pbContentType   = "application/x-protobuf"
	jsonContentType = "application/json"
)

var (
	pbEncoder     = &protoEncoder{}
	jsEncoder     = &jsonEncoder{}
	jsonMarshaler = &jsonpb.Marshaler{}
)

type encoder interface {
	unmarshalTracesRequest(buf []byte) (ptraceotlp.Request, error)
	unmarshalMetricsRequest(buf []byte) (pmetricotlp.Request, error)
	unmarshalLogsRequest(buf []byte) (plogotlp.Request, error)

	marshalTracesResponse(ptraceotlp.Response) ([]byte, error)
	marshalMetricsResponse(pmetricotlp.Response) ([]byte, error)
	marshalLogsResponse(plogotlp.Response) ([]byte, error)

	marshalStatus(rsp *spb.Status) ([]byte, error)

	contentType() string
}

type protoEncoder struct{}

func (protoEncoder) unmarshalTracesRequest(buf []byte) (ptraceotlp.Request, error) {
	req := ptraceotlp.NewRequest()
	err := req.UnmarshalProto(buf)
	return req, err
}

func (protoEncoder) unmarshalMetricsRequest(buf []byte) (pmetricotlp.Request, error) {
	req := pmetricotlp.NewRequest()
	err := req.UnmarshalProto(buf)
	return req, err
}

func (protoEncoder) unmarshalLogsRequest(buf []byte) (plogotlp.Request, error) {
	req := plogotlp.NewRequest()
	err := req.UnmarshalProto(buf)
	return req, err
}

func (protoEncoder) marshalTracesResponse(resp ptraceotlp.Response) ([]byte, error) {
	return resp.MarshalProto()
}

func (protoEncoder) marshalMetricsResponse(resp pmetricotlp.Response) ([]byte, error) {
	return resp.MarshalProto()
}

func (protoEncoder) marshalLogsResponse(resp plogotlp.Response) ([]byte, error) {
	return resp.MarshalProto()
}

func (protoEncoder) marshalStatus(resp *spb.Status) ([]byte, error) {
	return proto.Marshal(resp)
}

func (protoEncoder) contentType() string {
	return pbContentType
}

type jsonEncoder struct{}

func (jsonEncoder) unmarshalTracesRequest(buf []byte) (ptraceotlp.Request, error) {
	req := ptraceotlp.NewRequest()
	err := req.UnmarshalJSON(buf)
	return req, err
}

func (jsonEncoder) unmarshalMetricsRequest(buf []byte) (pmetricotlp.Request, error) {
	req := pmetricotlp.NewRequest()
	err := req.UnmarshalJSON(buf)
	return req, err
}

func (jsonEncoder) unmarshalLogsRequest(buf []byte) (plogotlp.Request, error) {
	req := plogotlp.NewRequest()
	err := req.UnmarshalJSON(buf)
	return req, err
}

func (jsonEncoder) marshalTracesResponse(resp ptraceotlp.Response) ([]byte, error) {
	return resp.MarshalJSON()
}

func (jsonEncoder) marshalMetricsResponse(resp pmetricotlp.Response) ([]byte, error) {
	return resp.MarshalJSON()
}

func (jsonEncoder) marshalLogsResponse(resp plogotlp.Response) ([]byte, error) {
	return resp.MarshalJSON()
}

func (jsonEncoder) marshalStatus(resp *spb.Status) ([]byte, error) {
	buf := new(bytes.Buffer)
	err := jsonMarshaler.Marshal(buf, resp)
	return buf.Bytes(), err
}

func (jsonEncoder) contentType() string {
	return jsonContentType
}

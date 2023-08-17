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

package exporterhelper // import "go.opentelemetry.io/collector/exporter/exporterhelper"

import (
	"context"
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exporterhelper/internal"
	"go.opentelemetry.io/collector/pdata/plog"
)

var logsMarshaler = plog.NewProtoMarshaler()
var logsUnmarshaler = plog.NewProtoUnmarshaler()

type logsRequest struct {
	baseRequest
	ld     plog.Logs
	pusher consumer.ConsumeLogsFunc
}

func newLogsRequest(ctx context.Context, ld plog.Logs, pusher consumer.ConsumeLogsFunc) internal.Request {
	return &logsRequest{
		baseRequest: baseRequest{ctx: ctx},
		ld:          ld,
		pusher:      pusher,
	}
}

func newLogsRequestUnmarshalerFunc(pusher consumer.ConsumeLogsFunc) internal.RequestUnmarshaler {
	return func(bytes []byte) (internal.Request, error) {
		logs, err := logsUnmarshaler.UnmarshalLogs(bytes)
		if err != nil {
			return nil, err
		}
		return newLogsRequest(context.Background(), logs, pusher), nil
	}
}

func (req *logsRequest) OnError(err error) internal.Request {
	var logError consumererror.Logs
	if errors.As(err, &logError) {
		return newLogsRequest(req.ctx, logError.GetLogs(), req.pusher)
	}
	return req
}

func (req *logsRequest) Export(ctx context.Context) error {
	return req.pusher(ctx, req.ld)
}

func (req *logsRequest) Marshal() ([]byte, error) {
	return logsMarshaler.MarshalLogs(req.ld)
}

func (req *logsRequest) Count() int {
	return req.ld.LogRecordCount()
}

type logsExporter struct {
	*baseExporter
	consumer.Logs
}

// Deprecated: [v0.58.0] use NewLogsExporterWithContext.
func NewLogsExporter(
	cfg config.Exporter,
	set component.ExporterCreateSettings,
	pusher consumer.ConsumeLogsFunc,
	options ...Option,
) (component.LogsExporter, error) {
	return NewLogsExporterWithContext(context.Background(), set, cfg, pusher, options...)
}

// NewLogsExporterWithContext creates an LogsExporter that records observability metrics and wraps every request with a Span.
func NewLogsExporterWithContext(
	_ context.Context,
	set component.ExporterCreateSettings,
	cfg config.Exporter,
	pusher consumer.ConsumeLogsFunc,
	options ...Option,
) (component.LogsExporter, error) {
	if cfg == nil {
		return nil, errNilConfig
	}

	if set.Logger == nil {
		return nil, errNilLogger
	}

	if pusher == nil {
		return nil, errNilPushLogsData
	}

	bs := fromOptions(options...)
	be := newBaseExporter(cfg, set, bs, config.LogsDataType, newLogsRequestUnmarshalerFunc(pusher))
	be.wrapConsumerSender(func(nextSender requestSender) requestSender {
		return &logsExporterWithObservability{
			obsrep:     be.obsrep,
			nextSender: nextSender,
		}
	})

	lc, err := consumer.NewLogs(func(ctx context.Context, ld plog.Logs) error {
		req := newLogsRequest(ctx, ld, pusher)
		err := be.sender.send(req)
		if errors.Is(err, errSendingQueueIsFull) {
			be.obsrep.recordLogsEnqueueFailure(req.Context(), int64(req.Count()))
		}
		return err
	}, bs.consumerOptions...)

	return &logsExporter{
		baseExporter: be,
		Logs:         lc,
	}, err
}

type logsExporterWithObservability struct {
	obsrep     *obsExporter
	nextSender requestSender
}

func (lewo *logsExporterWithObservability) send(req internal.Request) error {
	req.SetContext(lewo.obsrep.StartLogsOp(req.Context()))
	err := lewo.nextSender.send(req)
	lewo.obsrep.EndLogsOp(req.Context(), req.Count(), err)
	return err
}

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
	"go.opentelemetry.io/collector/pdata/pmetric"
)

var metricsMarshaler = pmetric.NewProtoMarshaler()
var metricsUnmarshaler = pmetric.NewProtoUnmarshaler()

type metricsRequest struct {
	baseRequest
	md     pmetric.Metrics
	pusher consumer.ConsumeMetricsFunc
}

func newMetricsRequest(ctx context.Context, md pmetric.Metrics, pusher consumer.ConsumeMetricsFunc) internal.Request {
	return &metricsRequest{
		baseRequest: baseRequest{ctx: ctx},
		md:          md,
		pusher:      pusher,
	}
}

func newMetricsRequestUnmarshalerFunc(pusher consumer.ConsumeMetricsFunc) internal.RequestUnmarshaler {
	return func(bytes []byte) (internal.Request, error) {
		metrics, err := metricsUnmarshaler.UnmarshalMetrics(bytes)
		if err != nil {
			return nil, err
		}
		return newMetricsRequest(context.Background(), metrics, pusher), nil
	}
}

func (req *metricsRequest) OnError(err error) internal.Request {
	var metricsError consumererror.Metrics
	if errors.As(err, &metricsError) {
		return newMetricsRequest(req.ctx, metricsError.GetMetrics(), req.pusher)
	}
	return req
}

func (req *metricsRequest) Export(ctx context.Context) error {
	return req.pusher(ctx, req.md)
}

// Marshal provides serialization capabilities required by persistent queue
func (req *metricsRequest) Marshal() ([]byte, error) {
	return metricsMarshaler.MarshalMetrics(req.md)
}

func (req *metricsRequest) Count() int {
	return req.md.DataPointCount()
}

type metricsExporter struct {
	*baseExporter
	consumer.Metrics
}

// Deprecated: [v0.58.0] use NewMetricsExporterWithContext.
func NewMetricsExporter(
	cfg config.Exporter,
	set component.ExporterCreateSettings,
	pusher consumer.ConsumeMetricsFunc,
	options ...Option,
) (component.MetricsExporter, error) {
	return NewMetricsExporterWithContext(context.Background(), set, cfg, pusher, options...)
}

// NewMetricsExporterWithContext creates an MetricsExporter that records observability metrics and wraps every request with a Span.
func NewMetricsExporterWithContext(
	_ context.Context,
	set component.ExporterCreateSettings,
	cfg config.Exporter,
	pusher consumer.ConsumeMetricsFunc,
	options ...Option,
) (component.MetricsExporter, error) {
	if cfg == nil {
		return nil, errNilConfig
	}

	if set.Logger == nil {
		return nil, errNilLogger
	}

	if pusher == nil {
		return nil, errNilPushMetricsData
	}

	bs := fromOptions(options...)
	be := newBaseExporter(cfg, set, bs, config.MetricsDataType, newMetricsRequestUnmarshalerFunc(pusher))
	be.wrapConsumerSender(func(nextSender requestSender) requestSender {
		return &metricsSenderWithObservability{
			obsrep:     be.obsrep,
			nextSender: nextSender,
		}
	})

	mc, err := consumer.NewMetrics(func(ctx context.Context, md pmetric.Metrics) error {
		req := newMetricsRequest(ctx, md, pusher)
		err := be.sender.send(req)
		if errors.Is(err, errSendingQueueIsFull) {
			be.obsrep.recordMetricsEnqueueFailure(req.Context(), int64(req.Count()))
		}
		return err
	}, bs.consumerOptions...)

	return &metricsExporter{
		baseExporter: be,
		Metrics:      mc,
	}, err
}

type metricsSenderWithObservability struct {
	obsrep     *obsExporter
	nextSender requestSender
}

func (mewo *metricsSenderWithObservability) send(req internal.Request) error {
	req.SetContext(mewo.obsrep.StartMetricsOp(req.Context()))
	err := mewo.nextSender.send(req)
	mewo.obsrep.EndMetricsOp(req.Context(), req.Count(), err)
	return err
}

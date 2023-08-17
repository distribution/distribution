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

package batchprocessor // import "go.opentelemetry.io/collector/processor/batchprocessor"

import (
	"context"
	"runtime"
	"sync"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"go.uber.org/zap"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// batch_processor is a component that accepts spans and metrics, places them
// into batches and sends downstream.
//
// batch_processor implements consumer.Traces and consumer.Metrics
//
// Batches are sent out with any of the following conditions:
// - batch size reaches cfg.SendBatchSize
// - cfg.Timeout is elapsed since the timestamp when the previous batch was sent out.
type batchProcessor struct {
	logger           *zap.Logger
	exportCtx        context.Context
	timer            *time.Timer
	timeout          time.Duration
	sendBatchSize    int
	sendBatchMaxSize int

	newItem chan interface{}
	batch   batch

	shutdownC  chan struct{}
	goroutines sync.WaitGroup

	telemetryLevel configtelemetry.Level
}

type batch interface {
	// export the current batch
	export(ctx context.Context, sendBatchMaxSize int, returnBytes bool) (sentBatchSize int, sentBatchBytes int, err error)

	// itemCount returns the size of the current batch
	itemCount() int

	// add item to the current batch
	add(item interface{})
}

var _ consumer.Traces = (*batchProcessor)(nil)
var _ consumer.Metrics = (*batchProcessor)(nil)
var _ consumer.Logs = (*batchProcessor)(nil)

func newBatchProcessor(set component.ProcessorCreateSettings, cfg *Config, batch batch, telemetryLevel configtelemetry.Level) (*batchProcessor, error) {
	exportCtx, err := tag.New(context.Background(), tag.Insert(processorTagKey, cfg.ID().String()))
	if err != nil {
		return nil, err
	}
	return &batchProcessor{
		logger:         set.Logger,
		exportCtx:      exportCtx,
		telemetryLevel: telemetryLevel,

		sendBatchSize:    int(cfg.SendBatchSize),
		sendBatchMaxSize: int(cfg.SendBatchMaxSize),
		timeout:          cfg.Timeout,
		newItem:          make(chan interface{}, runtime.NumCPU()),
		batch:            batch,
		shutdownC:        make(chan struct{}, 1),
	}, nil
}

func (bp *batchProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

// Start is invoked during service startup.
func (bp *batchProcessor) Start(context.Context, component.Host) error {
	bp.goroutines.Add(1)
	go bp.startProcessingCycle()
	return nil
}

// Shutdown is invoked during service shutdown.
func (bp *batchProcessor) Shutdown(context.Context) error {
	close(bp.shutdownC)

	// Wait until all goroutines are done.
	bp.goroutines.Wait()
	return nil
}

func (bp *batchProcessor) startProcessingCycle() {
	defer bp.goroutines.Done()
	bp.timer = time.NewTimer(bp.timeout)
	for {
		select {
		case <-bp.shutdownC:
		DONE:
			for {
				select {
				case item := <-bp.newItem:
					bp.processItem(item)
				default:
					break DONE
				}
			}
			// This is the close of the channel
			if bp.batch.itemCount() > 0 {
				// TODO: Set a timeout on sendTraces or
				// make it cancellable using the context that Shutdown gets as a parameter
				bp.sendItems(statTimeoutTriggerSend)
			}
			return
		case item := <-bp.newItem:
			if item == nil {
				continue
			}
			bp.processItem(item)
		case <-bp.timer.C:
			if bp.batch.itemCount() > 0 {
				bp.sendItems(statTimeoutTriggerSend)
			}
			bp.resetTimer()
		}
	}
}

func (bp *batchProcessor) processItem(item interface{}) {
	bp.batch.add(item)
	sent := false
	for bp.batch.itemCount() >= bp.sendBatchSize {
		sent = true
		bp.sendItems(statBatchSizeTriggerSend)
	}

	if sent {
		bp.stopTimer()
		bp.resetTimer()
	}
}

func (bp *batchProcessor) stopTimer() {
	if !bp.timer.Stop() {
		<-bp.timer.C
	}
}

func (bp *batchProcessor) resetTimer() {
	bp.timer.Reset(bp.timeout)
}

func (bp *batchProcessor) sendItems(triggerMeasure *stats.Int64Measure) {
	detailed := bp.telemetryLevel == configtelemetry.LevelDetailed
	sent, bytes, err := bp.batch.export(bp.exportCtx, bp.sendBatchMaxSize, detailed)
	if err != nil {
		bp.logger.Warn("Sender failed", zap.Error(err))
	} else {
		// Add that it came form the trace pipeline?
		stats.Record(bp.exportCtx, triggerMeasure.M(1), statBatchSendSize.M(int64(sent)))
		if detailed {
			stats.Record(bp.exportCtx, statBatchSendSizeBytes.M(int64(bytes)))
		}
	}
}

// ConsumeTraces implements TracesProcessor
func (bp *batchProcessor) ConsumeTraces(_ context.Context, td ptrace.Traces) error {
	bp.newItem <- td
	return nil
}

// ConsumeMetrics implements MetricsProcessor
func (bp *batchProcessor) ConsumeMetrics(_ context.Context, md pmetric.Metrics) error {
	// First thing is convert into a different internal format
	bp.newItem <- md
	return nil
}

// ConsumeLogs implements LogsProcessor
func (bp *batchProcessor) ConsumeLogs(_ context.Context, ld plog.Logs) error {
	bp.newItem <- ld
	return nil
}

// newBatchTracesProcessor creates a new batch processor that batches traces by size or with timeout
func newBatchTracesProcessor(set component.ProcessorCreateSettings, next consumer.Traces, cfg *Config, telemetryLevel configtelemetry.Level) (*batchProcessor, error) {
	return newBatchProcessor(set, cfg, newBatchTraces(next), telemetryLevel)
}

// newBatchMetricsProcessor creates a new batch processor that batches metrics by size or with timeout
func newBatchMetricsProcessor(set component.ProcessorCreateSettings, next consumer.Metrics, cfg *Config, telemetryLevel configtelemetry.Level) (*batchProcessor, error) {
	return newBatchProcessor(set, cfg, newBatchMetrics(next), telemetryLevel)
}

// newBatchLogsProcessor creates a new batch processor that batches logs by size or with timeout
func newBatchLogsProcessor(set component.ProcessorCreateSettings, next consumer.Logs, cfg *Config, telemetryLevel configtelemetry.Level) (*batchProcessor, error) {
	return newBatchProcessor(set, cfg, newBatchLogs(next), telemetryLevel)
}

type batchTraces struct {
	nextConsumer consumer.Traces
	traceData    ptrace.Traces
	spanCount    int
	sizer        ptrace.Sizer
}

func newBatchTraces(nextConsumer consumer.Traces) *batchTraces {
	return &batchTraces{nextConsumer: nextConsumer, traceData: ptrace.NewTraces(), sizer: ptrace.NewProtoMarshaler().(ptrace.Sizer)}
}

// add updates current batchTraces by adding new TraceData object
func (bt *batchTraces) add(item interface{}) {
	td := item.(ptrace.Traces)
	newSpanCount := td.SpanCount()
	if newSpanCount == 0 {
		return
	}

	bt.spanCount += newSpanCount
	td.ResourceSpans().MoveAndAppendTo(bt.traceData.ResourceSpans())
}

func (bt *batchTraces) export(ctx context.Context, sendBatchMaxSize int, returnBytes bool) (int, int, error) {
	var req ptrace.Traces
	var sent int
	var bytes int
	if sendBatchMaxSize > 0 && bt.itemCount() > sendBatchMaxSize {
		req = splitTraces(sendBatchMaxSize, bt.traceData)
		bt.spanCount -= sendBatchMaxSize
		sent = sendBatchMaxSize
	} else {
		req = bt.traceData
		sent = bt.spanCount
		bt.traceData = ptrace.NewTraces()
		bt.spanCount = 0
	}
	if returnBytes {
		bytes = bt.sizer.TracesSize(req)
	}
	return sent, bytes, bt.nextConsumer.ConsumeTraces(ctx, req)
}

func (bt *batchTraces) itemCount() int {
	return bt.spanCount
}

type batchMetrics struct {
	nextConsumer   consumer.Metrics
	metricData     pmetric.Metrics
	dataPointCount int
	sizer          pmetric.Sizer
}

func newBatchMetrics(nextConsumer consumer.Metrics) *batchMetrics {
	return &batchMetrics{nextConsumer: nextConsumer, metricData: pmetric.NewMetrics(), sizer: pmetric.NewProtoMarshaler().(pmetric.Sizer)}
}

func (bm *batchMetrics) export(ctx context.Context, sendBatchMaxSize int, returnBytes bool) (int, int, error) {
	var req pmetric.Metrics
	var sent int
	var bytes int
	if sendBatchMaxSize > 0 && bm.dataPointCount > sendBatchMaxSize {
		req = splitMetrics(sendBatchMaxSize, bm.metricData)
		bm.dataPointCount -= sendBatchMaxSize
		sent = sendBatchMaxSize
	} else {
		req = bm.metricData
		sent = bm.dataPointCount
		bm.metricData = pmetric.NewMetrics()
		bm.dataPointCount = 0
	}
	if returnBytes {
		bytes = bm.sizer.MetricsSize(req)
	}
	return sent, bytes, bm.nextConsumer.ConsumeMetrics(ctx, req)
}

func (bm *batchMetrics) itemCount() int {
	return bm.dataPointCount
}

func (bm *batchMetrics) add(item interface{}) {
	md := item.(pmetric.Metrics)

	newDataPointCount := md.DataPointCount()
	if newDataPointCount == 0 {
		return
	}
	bm.dataPointCount += newDataPointCount
	md.ResourceMetrics().MoveAndAppendTo(bm.metricData.ResourceMetrics())
}

type batchLogs struct {
	nextConsumer consumer.Logs
	logData      plog.Logs
	logCount     int
	sizer        plog.Sizer
}

func newBatchLogs(nextConsumer consumer.Logs) *batchLogs {
	return &batchLogs{nextConsumer: nextConsumer, logData: plog.NewLogs(), sizer: plog.NewProtoMarshaler().(plog.Sizer)}
}

func (bl *batchLogs) export(ctx context.Context, sendBatchMaxSize int, returnBytes bool) (int, int, error) {
	var req plog.Logs
	var sent int
	var bytes int
	if sendBatchMaxSize > 0 && bl.logCount > sendBatchMaxSize {
		req = splitLogs(sendBatchMaxSize, bl.logData)
		bl.logCount -= sendBatchMaxSize
		sent = sendBatchMaxSize
	} else {
		req = bl.logData
		sent = bl.logCount
		bl.logData = plog.NewLogs()
		bl.logCount = 0
	}
	if returnBytes {
		bytes = bl.sizer.LogsSize(req)
	}
	return sent, bytes, bl.nextConsumer.ConsumeLogs(ctx, req)
}

func (bl *batchLogs) itemCount() int {
	return bl.logCount
}

func (bl *batchLogs) add(item interface{}) {
	ld := item.(plog.Logs)

	newLogsCount := ld.LogRecordCount()
	if newLogsCount == 0 {
		return
	}
	bl.logCount += newLogsCount
	ld.ResourceLogs().MoveAndAppendTo(bl.logData.ResourceLogs())
}

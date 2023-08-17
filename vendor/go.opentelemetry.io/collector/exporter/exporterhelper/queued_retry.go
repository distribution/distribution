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
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"go.opencensus.io/metric/metricdata"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter/exporterhelper/internal"
	"go.opentelemetry.io/collector/extension/experimental/storage"
	"go.opentelemetry.io/collector/internal/obsreportconfig/obsmetrics"
)

var (
	errSendingQueueIsFull = errors.New("sending_queue is full")
	errNoStorageClient    = errors.New("no storage client extension found")
	errWrongExtensionType = errors.New("requested extension is not a storage extension")
)

// QueueSettings defines configuration for queueing batches before sending to the consumerSender.
type QueueSettings struct {
	// Enabled indicates whether to not enqueue batches before sending to the consumerSender.
	Enabled bool `mapstructure:"enabled"`
	// NumConsumers is the number of consumers from the queue.
	NumConsumers int `mapstructure:"num_consumers"`
	// QueueSize is the maximum number of batches allowed in queue at a given time.
	QueueSize int `mapstructure:"queue_size"`
	// StorageID if not empty, enables the persistent storage and uses the component specified
	// as a storage extension for the persistent queue
	StorageID *config.ComponentID `mapstructure:"storage"`
}

// NewDefaultQueueSettings returns the default settings for QueueSettings.
func NewDefaultQueueSettings() QueueSettings {
	return QueueSettings{
		Enabled:      true,
		NumConsumers: 10,
		// For 5000 queue elements at 100 requests/sec gives about 50 sec of survival of destination outage.
		// This is a pretty decent value for production.
		// User should calculate this from the perspective of how many seconds to buffer in case of a backend outage,
		// multiply that by the number of requests per seconds.
		QueueSize: 5000,
	}
}

// Validate checks if the QueueSettings configuration is valid
func (qCfg *QueueSettings) Validate() error {
	if !qCfg.Enabled {
		return nil
	}

	if qCfg.QueueSize <= 0 {
		return errors.New("queue size must be positive")
	}

	return nil
}

type queuedRetrySender struct {
	fullName           string
	id                 config.ComponentID
	signal             config.DataType
	cfg                QueueSettings
	consumerSender     requestSender
	queue              internal.ProducerConsumerQueue
	retryStopCh        chan struct{}
	traceAttribute     attribute.KeyValue
	logger             *zap.Logger
	requeuingEnabled   bool
	requestUnmarshaler internal.RequestUnmarshaler
}

func newQueuedRetrySender(id config.ComponentID, signal config.DataType, qCfg QueueSettings, rCfg RetrySettings, reqUnmarshaler internal.RequestUnmarshaler, nextSender requestSender, logger *zap.Logger) *queuedRetrySender {
	retryStopCh := make(chan struct{})
	sampledLogger := createSampledLogger(logger)
	traceAttr := attribute.String(obsmetrics.ExporterKey, id.String())

	qrs := &queuedRetrySender{
		fullName:           id.String(),
		id:                 id,
		signal:             signal,
		cfg:                qCfg,
		retryStopCh:        retryStopCh,
		traceAttribute:     traceAttr,
		logger:             sampledLogger,
		requestUnmarshaler: reqUnmarshaler,
	}

	qrs.consumerSender = &retrySender{
		traceAttribute: traceAttr,
		cfg:            rCfg,
		nextSender:     nextSender,
		stopCh:         retryStopCh,
		logger:         sampledLogger,
		// Following three functions actually depend on queuedRetrySender
		onTemporaryFailure: qrs.onTemporaryFailure,
	}

	if qCfg.StorageID == nil {
		qrs.queue = internal.NewBoundedMemoryQueue(qrs.cfg.QueueSize)
	}
	// The Persistent Queue is initialized separately as it needs extra information about the component

	return qrs
}

func getStorageExtension(extensions map[config.ComponentID]component.Extension, storageID config.ComponentID) (storage.Extension, error) {
	if ext, found := extensions[storageID]; found {
		if storageExt, ok := ext.(storage.Extension); ok {
			return storageExt, nil
		}
		return nil, errWrongExtensionType
	}
	return nil, errNoStorageClient
}

func toStorageClient(ctx context.Context, storageID config.ComponentID, host component.Host, ownerID config.ComponentID, signal config.DataType) (storage.Client, error) {
	extension, err := getStorageExtension(host.GetExtensions(), storageID)
	if err != nil {
		return nil, err
	}

	client, err := extension.GetClient(ctx, component.KindExporter, ownerID, string(signal))
	if err != nil {
		return nil, err
	}

	return client, err
}

// initializePersistentQueue uses extra information for initialization available from component.Host
func (qrs *queuedRetrySender) initializePersistentQueue(ctx context.Context, host component.Host) error {
	if qrs.cfg.StorageID == nil {
		return nil
	}

	storageClient, err := toStorageClient(ctx, *qrs.cfg.StorageID, host, qrs.id, qrs.signal)
	if err != nil {
		return err
	}

	qrs.queue = internal.NewPersistentQueue(ctx, qrs.fullName, qrs.signal, qrs.cfg.QueueSize, qrs.logger, storageClient, qrs.requestUnmarshaler)

	// TODO: this can be further exposed as a config param rather than relying on a type of queue
	qrs.requeuingEnabled = true

	return nil
}

func (qrs *queuedRetrySender) onTemporaryFailure(logger *zap.Logger, req internal.Request, err error) error {
	if !qrs.requeuingEnabled || qrs.queue == nil {
		logger.Error(
			"Exporting failed. No more retries left. Dropping data.",
			zap.Error(err),
			zap.Int("dropped_items", req.Count()),
		)
		return err
	}

	if qrs.queue.Produce(req) {
		logger.Error(
			"Exporting failed. Putting back to the end of the queue.",
			zap.Error(err),
		)
	} else {
		logger.Error(
			"Exporting failed. Queue did not accept requeuing request. Dropping data.",
			zap.Error(err),
			zap.Int("dropped_items", req.Count()),
		)
	}
	return err
}

// start is invoked during service startup.
func (qrs *queuedRetrySender) start(ctx context.Context, host component.Host) error {
	if err := qrs.initializePersistentQueue(ctx, host); err != nil {
		return err
	}

	qrs.queue.StartConsumers(qrs.cfg.NumConsumers, func(item internal.Request) {
		_ = qrs.consumerSender.send(item)
		item.OnProcessingFinished()
	})

	// Start reporting queue length metric
	if qrs.cfg.Enabled {
		err := globalInstruments.queueSize.UpsertEntry(func() int64 {
			return int64(qrs.queue.Size())
		}, metricdata.NewLabelValue(qrs.fullName))
		if err != nil {
			return fmt.Errorf("failed to create retry queue size metric: %w", err)
		}
		err = globalInstruments.queueCapacity.UpsertEntry(func() int64 {
			return int64(qrs.cfg.QueueSize)
		}, metricdata.NewLabelValue(qrs.fullName))
		if err != nil {
			return fmt.Errorf("failed to create retry queue capacity metric: %w", err)
		}
	}

	return nil
}

// shutdown is invoked during service shutdown.
func (qrs *queuedRetrySender) shutdown() {
	// Cleanup queue metrics reporting
	if qrs.cfg.Enabled {
		_ = globalInstruments.queueSize.UpsertEntry(func() int64 {
			return int64(0)
		}, metricdata.NewLabelValue(qrs.fullName))
	}

	// First Stop the retry goroutines, so that unblocks the queue numWorkers.
	close(qrs.retryStopCh)

	// Stop the queued sender, this will drain the queue and will call the retry (which is stopped) that will only
	// try once every request.
	if qrs.queue != nil {
		qrs.queue.Stop()
	}
}

// RetrySettings defines configuration for retrying batches in case of export failure.
// The current supported strategy is exponential backoff.
type RetrySettings struct {
	// Enabled indicates whether to not retry sending batches in case of export failure.
	Enabled bool `mapstructure:"enabled"`
	// InitialInterval the time to wait after the first failure before retrying.
	InitialInterval time.Duration `mapstructure:"initial_interval"`
	// MaxInterval is the upper bound on backoff interval. Once this value is reached the delay between
	// consecutive retries will always be `MaxInterval`.
	MaxInterval time.Duration `mapstructure:"max_interval"`
	// MaxElapsedTime is the maximum amount of time (including retries) spent trying to send a request/batch.
	// Once this value is reached, the data is discarded.
	MaxElapsedTime time.Duration `mapstructure:"max_elapsed_time"`
}

// NewDefaultRetrySettings returns the default settings for RetrySettings.
func NewDefaultRetrySettings() RetrySettings {
	return RetrySettings{
		Enabled:         true,
		InitialInterval: 5 * time.Second,
		MaxInterval:     30 * time.Second,
		MaxElapsedTime:  5 * time.Minute,
	}
}

func createSampledLogger(logger *zap.Logger) *zap.Logger {
	if logger.Core().Enabled(zapcore.DebugLevel) {
		// Debugging is enabled. Don't do any sampling.
		return logger
	}

	// Create a logger that samples all messages to 1 per 10 seconds initially,
	// and 1/100 of messages after that.
	opts := zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(
			core,
			10*time.Second,
			1,
			100,
		)
	})
	return logger.WithOptions(opts)
}

// send implements the requestSender interface
func (qrs *queuedRetrySender) send(req internal.Request) error {
	if !qrs.cfg.Enabled {
		err := qrs.consumerSender.send(req)
		if err != nil {
			qrs.logger.Error(
				"Exporting failed. Dropping data. Try enabling sending_queue to survive temporary failures.",
				zap.Int("dropped_items", req.Count()),
			)
		}
		return err
	}

	// Prevent cancellation and deadline to propagate to the context stored in the queue.
	// The grpc/http based receivers will cancel the request context after this function returns.
	req.SetContext(noCancellationContext{Context: req.Context()})

	span := trace.SpanFromContext(req.Context())
	if !qrs.queue.Produce(req) {
		qrs.logger.Error(
			"Dropping data because sending_queue is full. Try increasing queue_size.",
			zap.Int("dropped_items", req.Count()),
		)
		span.AddEvent("Dropped item, sending_queue is full.", trace.WithAttributes(qrs.traceAttribute))
		return errSendingQueueIsFull
	}

	span.AddEvent("Enqueued item.", trace.WithAttributes(qrs.traceAttribute))
	return nil
}

// TODO: Clean this by forcing all exporters to return an internal error type that always include the information about retries.
type throttleRetry struct {
	err   error
	delay time.Duration
}

func (t throttleRetry) Error() string {
	return "Throttle (" + t.delay.String() + "), error: " + t.err.Error()
}

func (t throttleRetry) Unwrap() error {
	return t.err
}

// NewThrottleRetry creates a new throttle retry error.
func NewThrottleRetry(err error, delay time.Duration) error {
	return throttleRetry{
		err:   err,
		delay: delay,
	}
}

type onRequestHandlingFinishedFunc func(*zap.Logger, internal.Request, error) error

type retrySender struct {
	traceAttribute     attribute.KeyValue
	cfg                RetrySettings
	nextSender         requestSender
	stopCh             chan struct{}
	logger             *zap.Logger
	onTemporaryFailure onRequestHandlingFinishedFunc
}

// send implements the requestSender interface
func (rs *retrySender) send(req internal.Request) error {
	if !rs.cfg.Enabled {
		err := rs.nextSender.send(req)
		if err != nil {
			rs.logger.Error(
				"Exporting failed. Try enabling retry_on_failure config option to retry on retryable errors",
				zap.Error(err),
			)
		}
		return err
	}

	// Do not use NewExponentialBackOff since it calls Reset and the code here must
	// call Reset after changing the InitialInterval (this saves an unnecessary call to Now).
	expBackoff := backoff.ExponentialBackOff{
		InitialInterval:     rs.cfg.InitialInterval,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         rs.cfg.MaxInterval,
		MaxElapsedTime:      rs.cfg.MaxElapsedTime,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}
	expBackoff.Reset()
	span := trace.SpanFromContext(req.Context())
	retryNum := int64(0)
	for {
		span.AddEvent(
			"Sending request.",
			trace.WithAttributes(rs.traceAttribute, attribute.Int64("retry_num", retryNum)))

		err := rs.nextSender.send(req)
		if err == nil {
			return nil
		}

		// Immediately drop data on permanent errors.
		if consumererror.IsPermanent(err) {
			rs.logger.Error(
				"Exporting failed. The error is not retryable. Dropping data.",
				zap.Error(err),
				zap.Int("dropped_items", req.Count()),
			)
			return err
		}

		// Give the request a chance to extract signal data to retry if only some data
		// failed to process.
		req = req.OnError(err)

		backoffDelay := expBackoff.NextBackOff()
		if backoffDelay == backoff.Stop {
			// throw away the batch
			err = fmt.Errorf("max elapsed time expired %w", err)
			return rs.onTemporaryFailure(rs.logger, req, err)
		}

		throttleErr := throttleRetry{}
		isThrottle := errors.As(err, &throttleErr)
		if isThrottle {
			backoffDelay = max(backoffDelay, throttleErr.delay)
		}

		backoffDelayStr := backoffDelay.String()
		span.AddEvent(
			"Exporting failed. Will retry the request after interval.",
			trace.WithAttributes(
				rs.traceAttribute,
				attribute.String("interval", backoffDelayStr),
				attribute.String("error", err.Error())))
		rs.logger.Info(
			"Exporting failed. Will retry the request after interval.",
			zap.Error(err),
			zap.String("interval", backoffDelayStr),
		)
		retryNum++

		// back-off, but get interrupted when shutting down or request is cancelled or timed out.
		select {
		case <-req.Context().Done():
			return fmt.Errorf("Request is cancelled or timed out %w", err)
		case <-rs.stopCh:
			return fmt.Errorf("interrupted due to shutdown %w", err)
		case <-time.After(backoffDelay):
		}
	}
}

// max returns the larger of x or y.
func max(x, y time.Duration) time.Duration {
	if x < y {
		return y
	}
	return x
}

type noCancellationContext struct {
	context.Context
}

func (noCancellationContext) Deadline() (deadline time.Time, ok bool) {
	return
}

func (noCancellationContext) Done() <-chan struct{} {
	return nil
}

func (noCancellationContext) Err() error {
	return nil
}

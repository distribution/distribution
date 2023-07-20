// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/newrelic/go-agent/v3/internal"
)

type appData struct {
	id   internal.AgentRunID
	data harvestable
}

type app struct {
	Logger
	config      config
	rpmControls rpmControls
	testHarvest *harvest

	trObserver traceObserver

	// placeholderRun is used when the application is not connected.
	placeholderRun *appRun

	// initiateShutdown is used to tell the processor to shutdown.
	initiateShutdown chan time.Duration

	// shutdownStarted and shutdownComplete are closed by the processor
	// goroutine to indicate the shutdown status.  Two channels are used so
	// that the call of app.Shutdown() can block until shutdown has
	// completed but other goroutines can exit when shutdown has started.
	// This is not just an optimization:  This prevents a deadlock if
	// harvesting data during the shutdown fails and an attempt is made to
	// merge the data into the next harvest.
	shutdownStarted  chan struct{}
	shutdownComplete chan struct{}

	// Sends to these channels should not occur without a <-shutdownStarted
	// select option to prevent deadlock.
	dataChan           chan appData
	collectorErrorChan chan rpmResponse
	connectChan        chan *appRun

	// This mutex protects both `run` and `err`, both of which should only
	// be accessed using getState and setState.
	sync.RWMutex
	// run is non-nil when the app is successfully connected.  It is
	// immutable.
	run *appRun
	// err is non-nil if the application will never be connected again
	// (disconnect, license exception, shutdown).
	err error

	serverless *serverlessHarvest
}

func (app *app) doHarvest(h *harvest, harvestStart time.Time, run *appRun) {
	h.CreateFinalMetrics(run, app.getObserver())

	payloads := h.Payloads(app.config.DistributedTracer.Enabled)
	for _, p := range payloads {
		cmd := p.EndpointMethod()
		var data []byte

		defer func() {
			if r := recover(); r != nil {
				app.Warn("panic occured when creating harvest data", map[string]interface{}{
					"cmd":   cmd,
					"panic": r,
				})

				// make sure the loop continues
				data = nil
			}
		}()

		data, err := p.Data(run.Reply.RunID.String(), harvestStart)

		if err != nil {
			app.Warn("unable to create harvest data", map[string]interface{}{
				"cmd":   cmd,
				"error": err.Error(),
			})
			continue
		}
		if data == nil {
			continue
		}

		call := rpmCmd{
			Collector:         run.Reply.Collector,
			RunID:             run.Reply.RunID.String(),
			Name:              cmd,
			Data:              data,
			RequestHeadersMap: run.Reply.RequestHeadersMap,
			MaxPayloadSize:    run.Reply.MaxPayloadSizeInBytes,
		}

		resp := collectorRequest(call, app.rpmControls)

		if resp.IsDisconnect() || resp.IsRestartException() {
			select {
			case app.collectorErrorChan <- resp:
			case <-app.shutdownStarted:
			}
			return
		}

		if resp.Err != nil {
			app.Warn("harvest failure", map[string]interface{}{
				"cmd":         cmd,
				"error":       resp.Err.Error(),
				"retain_data": resp.ShouldSaveHarvestData(),
			})
		}

		if resp.ShouldSaveHarvestData() {
			app.Consume(run.Reply.RunID, p)
		}
	}
}

func (app *app) connectRoutine() {
	attempts := 0
	for {
		reply, resp := connectAttempt(app.config, app.rpmControls)

		if reply != nil {
			select {
			case app.connectChan <- newAppRun(app.config, reply):
			case <-app.shutdownStarted:
			}
			return
		}

		if resp.IsDisconnect() {
			select {
			case app.collectorErrorChan <- resp:
			case <-app.shutdownStarted:
			}
			return
		}

		if nil != resp.Err {
			app.Warn("application connect failure", map[string]interface{}{
				"error": resp.Err.Error(),
			})
		}

		backoff := getConnectBackoffTime(attempts)
		time.Sleep(time.Duration(backoff) * time.Second)
		attempts++
	}
}

func (app *app) connectTraceObserver(reply *internal.ConnectReply) {
	if obs := app.getObserver(); obs != nil {
		obs.restart(reply.RunID, reply.RequestHeadersMap)
		return
	}

	var endpoint observerURL
	if nil != app.config.traceObserverURL {
		endpoint = *app.config.traceObserverURL
	}

	observer, err := newTraceObserver(reply.RunID, reply.RequestHeadersMap, observerConfig{
		endpoint:    endpoint,
		license:     app.config.License,
		log:         app.config.Logger,
		queueSize:   app.config.InfiniteTracing.SpanEvents.QueueSize,
		appShutdown: app.shutdownComplete,
		dialer:      reply.TraceObsDialer,
	})
	if nil != err {
		app.Error("unable to create trace observer", map[string]interface{}{
			"err": err.Error(),
		})
		return
	}
	app.Debug("trace observer connected", map[string]interface{}{
		"url": app.config.traceObserverURL.host,
	})
	app.setObserver(observer)
}

// Connect backoff time follows the sequence defined at
// https://source.datanerd.us/agents/agent-specs/blob/master/Collector-Response-Handling.md#retries-and-backoffs
func getConnectBackoffTime(attempt int) int {
	connectBackoffTimes := [...]int{15, 15, 30, 60, 120, 300}
	l := len(connectBackoffTimes)
	if (attempt < 0) || (attempt >= l) {
		return connectBackoffTimes[l-1]
	}
	return connectBackoffTimes[attempt]
}

func processConnectMessages(run *appRun, lg Logger) {
	for _, msg := range run.Reply.Messages {
		event := "collector message"
		cn := map[string]interface{}{"msg": msg.Message}

		switch strings.ToLower(msg.Level) {
		case "error":
			lg.Error(event, cn)
		case "warn":
			lg.Warn(event, cn)
		case "info":
			lg.Info(event, cn)
		case "debug", "verbose":
			lg.Debug(event, cn)
		}
	}
}

func (app *app) process() {
	// Both the harvest and the run are non-nil when the app is connected,
	// and nil otherwise.
	var h *harvest
	var run *appRun

	harvestTicker := time.NewTicker(time.Second)
	defer harvestTicker.Stop()

	for {
		select {
		case <-harvestTicker.C:
			if nil != run {
				now := time.Now()
				if ready := h.Ready(now); nil != ready {
					go app.doHarvest(ready, now, run)
				}
			}
		case d := <-app.dataChan:
			if nil != run && run.Reply.RunID == d.id {
				d.data.MergeIntoHarvest(h)
			}
		case timeout := <-app.initiateShutdown:
			close(app.shutdownStarted)

			// Remove the run before merging any final data to
			// ensure a bounded number of receives from dataChan.
			app.setState(nil, errors.New("application shut down"))

			if obs := app.getObserver(); obs != nil {
				if err := obs.shutdown(timeout); err != nil {
					app.Error("trace observer shutdown timeout exceeded", map[string]interface{}{
						"err": err.Error(),
					})
				}
			}

			if nil != run {
				for done := false; !done; {
					select {
					case d := <-app.dataChan:
						if run.Reply.RunID == d.id {
							d.data.MergeIntoHarvest(h)
						}
					default:
						done = true
					}
				}
				app.doHarvest(h, time.Now(), run)
			}

			close(app.shutdownComplete)
			app.setObserver(nil)
			secureAgent.DeactivateSecurity()
			return
		case resp := <-app.collectorErrorChan:
			run = nil
			h = nil
			app.setState(nil, nil)

			if resp.IsDisconnect() {
				app.setState(nil, resp.Err)
				app.Error("application disconnected", map[string]interface{}{
					"app": app.config.AppName,
				})
				secureAgent.DeactivateSecurity()
			} else if resp.IsRestartException() {
				app.Info("application restarted", map[string]interface{}{
					"app": app.config.AppName,
				})
				go app.connectRoutine()
			}
		case run = <-app.connectChan:
			if shouldUseTraceObserver(run.Config) {
				app.connectTraceObserver(run.Reply)
			} else if shouldUseTraceObserver(app.config) {
				app.Debug("trace observer disabled via backend", map[string]interface{}{
					"local-DistributedTracer.Enabled":  app.config.DistributedTracer.Enabled,
					"server-DistributedTracer.Enabled": run.Config.DistributedTracer.Enabled,
					"local-SpanEvents.Enabled":         app.config.SpanEvents.Enabled,
					"server-SpanEvents.Enabled":        run.Config.SpanEvents.Enabled,
				})
			}

			run.harvestConfig.CommonAttributes = commonAttributes{
				hostname:   app.config.hostname,
				entityName: app.config.AppName,
				entityGUID: run.Reply.EntityGUID,
			}

			h = newHarvest(time.Now(), run.harvestConfig)
			app.setState(run, nil)

			app.Info("application connected", map[string]interface{}{
				"app": app.config.AppName,
				"run": run.Reply.RunID.String(),
			})
			processConnectMessages(run, app)
			secureAgent.RefreshState(getLinkedMetaData(app))
		}
	}
}

func (app *app) Shutdown(timeout time.Duration) {
	if nil == app {
		return
	}
	if !app.config.Enabled {
		return
	}
	if app.config.ServerlessMode.Enabled {
		return
	}

	select {
	case app.initiateShutdown <- timeout:
	default:
	}

	// Block until shutdown is done or timeout occurs.
	t := time.NewTimer(timeout)
	select {
	case <-app.shutdownComplete:
	case <-t.C:
	}
	t.Stop()

	app.Info("application shutdown", map[string]interface{}{
		"app": app.config.AppName,
	})
}

func runSampler(app *app, period time.Duration) {
	previous := getSystemSample(time.Now(), app)
	t := time.NewTicker(period)
	for {
		select {
		case now := <-t.C:
			current := getSystemSample(now, app)
			run, _ := app.getState()
			app.Consume(run.Reply.RunID, getSystemStats(systemSamples{
				Previous: previous,
				Current:  current,
			}))
			previous = current
		case <-app.shutdownStarted:
			t.Stop()
			return
		}
	}
}

func (app *app) WaitForConnection(timeout time.Duration) error {
	if nil == app {
		return nil
	}
	if !app.config.Enabled {
		return nil
	}
	if app.config.ServerlessMode.Enabled {
		return nil
	}
	deadline := time.Now().Add(timeout)
	pollPeriod := 50 * time.Millisecond

	for {
		run, err := app.getState()
		if nil != err {
			return err
		}
		if run.Reply.RunID != "" {
			if shouldUseTraceObserver(run.Config) {
				if obs := app.getObserver(); obs != nil && obs.initialConnCompleted() {
					return nil
				}
			} else {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout out after %s", timeout.String())
		}
		time.Sleep(pollPeriod)
	}
}

func newApp(c config) *app {
	transport := c.Transport
	if nil == transport {
		transport = collectorDefaultTransport
	}
	app := &app{
		Logger:         c.Logger,
		config:         c,
		placeholderRun: newPlaceholderAppRun(c),

		// This channel must be buffered since Shutdown makes a
		// non-blocking send attempt.
		initiateShutdown: make(chan time.Duration, 1),

		shutdownStarted:    make(chan struct{}),
		shutdownComplete:   make(chan struct{}),
		connectChan:        make(chan *appRun, 1),
		collectorErrorChan: make(chan rpmResponse, 1),
		dataChan:           make(chan appData, appDataChanSize),
		rpmControls: rpmControls{
			License: c.License,
			Client: &http.Client{
				Transport: transport,
				Timeout:   collectorTimeout,
			},
			Logger: c.Logger,
			GzipWriterPool: &sync.Pool{
				New: func() interface{} {
					return gzip.NewWriter(io.Discard)
				},
			},
		},
	}

	app.Info("application created", map[string]interface{}{
		"app":          app.config.AppName,
		"version":      Version,
		"enabled":      app.config.Enabled,
		"grpc-version": grpcVersion,
	})

	if app.config.Enabled {
		if app.config.ServerlessMode.Enabled {
			reply := newServerlessConnectReply(c)
			app.run = newAppRun(c, reply)
			app.serverless = newServerlessHarvest(c.Logger, os.Getenv)
		} else {
			go app.process()
			go app.connectRoutine()
			if app.config.RuntimeSampler.Enabled {
				go runSampler(app, runtimeSamplerPeriod)
			}
		}
	}

	return app
}

func shouldUseTraceObserver(c config) bool {
	return nil != c.traceObserverURL && c.SpanEvents.Enabled && c.DistributedTracer.Enabled
}

var (
	_ internal.HarvestTestinger = &app{}
	_ internal.Expect           = &app{}
)

func (app *app) HarvestTesting(replyfn func(*internal.ConnectReply)) {
	if nil != replyfn {
		reply := internal.ConnectReplyDefaults()
		replyfn(reply)
		app.placeholderRun = newAppRun(app.config, reply)
	}
	app.testHarvest = newHarvest(time.Now(), app.placeholderRun.harvestConfig)
}

func (app *app) getState() (*appRun, error) {
	app.RLock()
	defer app.RUnlock()

	run := app.run
	if nil == run {
		run = app.placeholderRun
	}
	return run, app.err
}

func (app *app) setState(run *appRun, err error) {
	app.Lock()
	defer app.Unlock()

	app.run = run
	app.err = err
}

func (app *app) getObserver() traceObserver {
	app.RLock()
	defer app.RUnlock()
	return app.trObserver
}

func (app *app) setObserver(observer traceObserver) {
	app.Lock()
	defer app.Unlock()
	app.trObserver = observer
}

func newTransaction(thd *thread) *Transaction {
	return &Transaction{
		Private: thd,
		thread:  thd,
	}
}

// StartTransaction implements newrelic.Application's StartTransaction.
func (app *app) StartTransaction(name string, opts ...TraceOption) *Transaction {
	if nil == app {
		return nil
	}
	run, _ := app.getState()
	return newTransaction(newTxn(app, run, name, opts...))
}

var (
	errHighSecurityEnabled        = errors.New("high security enabled")
	errCustomEventsDisabled       = errors.New("custom events disabled")
	errCustomEventsRemoteDisabled = errors.New("custom events disabled by server")
)

// RecordCustomEvent implements newrelic.Application's RecordCustomEvent.
func (app *app) RecordCustomEvent(eventType string, params map[string]interface{}) error {
	if nil == app {
		return nil
	}
	if app.config.Config.HighSecurity {
		return errHighSecurityEnabled
	}

	if !app.config.CustomInsightsEvents.Enabled {
		return errCustomEventsDisabled
	}

	event, e := createCustomEvent(eventType, params, time.Now())
	if nil != e {
		return e
	}

	run, _ := app.getState()
	if !run.Reply.CollectCustomEvents {
		return errCustomEventsRemoteDisabled
	}

	if !run.Reply.SecurityPolicies.CustomEvents.Enabled() {
		return errSecurityPolicy
	}

	app.Consume(run.Reply.RunID, event)

	return nil
}

var (
	errMetricInf        = errors.New("invalid metric value: inf")
	errMetricNaN        = errors.New("invalid metric value: NaN")
	errMetricNameEmpty  = errors.New("missing metric name")
	errMetricServerless = errors.New("custom metrics are not currently supported in serverless mode")
)

// RecordCustomMetric implements newrelic.Application's RecordCustomMetric.
func (app *app) RecordCustomMetric(name string, value float64) error {
	if nil == app {
		return nil
	}
	if app.config.ServerlessMode.Enabled {
		return errMetricServerless
	}
	if math.IsNaN(value) {
		return errMetricNaN
	}
	if math.IsInf(value, 0) {
		return errMetricInf
	}
	if "" == name {
		return errMetricNameEmpty
	}
	run, _ := app.getState()
	app.Consume(run.Reply.RunID, customMetric{
		RawInputName: name,
		Value:        value,
	})
	return nil
}

var (
	errAppLoggingDisabled = errors.New("log data can not be recorded when application logging is disabled")
)

// RecordLog implements newrelic.Application's RecordLog.
func (app *app) RecordLog(log *LogData) error {
	if !app.config.ApplicationLogging.Enabled {
		return errAppLoggingDisabled
	}

	event, err := log.toLogEvent()
	if err != nil {
		return err
	}

	run, _ := app.getState()
	app.Consume(run.Reply.RunID, &event)
	return nil
}

var (
	_ internal.ServerlessWriter = &app{}
)

func (app *app) ServerlessWrite(arn string, writer io.Writer) {
	app.serverless.Write(arn, writer)
}

func (app *app) Consume(id internal.AgentRunID, data harvestable) {

	app.serverless.Consume(data)

	if nil != app.testHarvest {
		data.MergeIntoHarvest(app.testHarvest)
		return
	}

	if "" == id {
		return
	}

	select {
	case app.dataChan <- appData{id, data}:
	case <-app.shutdownStarted:
	}
}

func (app *app) ExpectCustomEvents(t internal.Validator, want []internal.WantEvent) {
	expectCustomEvents(extendValidator(t, "custom events"), app.testHarvest.CustomEvents, want)
}

// ExpectLogEvents from app checks that the contents of the logs test harvest matches the list of WantLogs.
func (app *app) ExpectLogEvents(t internal.Validator, want []internal.WantLog) {
	expectLogEvents(extendValidator(t, "log events"), app.testHarvest.LogEvents, want)
}

// ExpectLogEvents from transactions dumps all the log events from a transaction into the test harvest
// then checks that the contents of the logs harvest matches the list of WantLogs.
func (txn *Transaction) ExpectLogEvents(t internal.Validator, want []internal.WantLog) {
	txn.thread.MergeIntoHarvest(txn.Application().app.testHarvest)
	expectLogEvents(extendValidator(t, "log events"), txn.Application().app.testHarvest.LogEvents, want)
}

func (app *app) ExpectErrors(t internal.Validator, want []internal.WantError) {
	t = extendValidator(t, "traced errors")
	expectErrors(t, app.testHarvest.ErrorTraces, want)
}

func (app *app) ExpectErrorEvents(t internal.Validator, want []internal.WantEvent) {
	t = extendValidator(t, "error events")
	expectErrorEvents(t, app.testHarvest.ErrorEvents, want)
}

func (app *app) ExpectSpanEvents(t internal.Validator, want []internal.WantEvent) {
	t = extendValidator(t, "spans events")
	expectSpanEvents(t, app.testHarvest.SpanEvents, want)
}

func (app *app) ExpectTxnEvents(t internal.Validator, want []internal.WantEvent) {
	t = extendValidator(t, "txn events")
	expectTxnEvents(t, app.testHarvest.TxnEvents, want)
}

func (app *app) ExpectMetrics(t internal.Validator, want []internal.WantMetric) {
	t = extendValidator(t, "metrics")
	expectMetrics(t, app.testHarvest.Metrics, want)
}

func (app *app) ExpectMetricsPresent(t internal.Validator, want []internal.WantMetric) {
	t = extendValidator(t, "metrics")
	expectMetricsPresent(t, app.testHarvest.Metrics, want)
}

func (app *app) ExpectTxnMetrics(t internal.Validator, want internal.WantTxn) {
	t = extendValidator(t, "metrics")
	expectTxnMetrics(t, app.testHarvest.Metrics, want)
}

func (app *app) ExpectTxnTraces(t internal.Validator, want []internal.WantTxnTrace) {
	t = extendValidator(t, "txn traces")
	expectTxnTraces(t, app.testHarvest.TxnTraces, want)
}

func (app *app) ExpectSlowQueries(t internal.Validator, want []internal.WantSlowQuery) {
	t = extendValidator(t, "slow queries")
	expectSlowQueries(t, app.testHarvest.SlowSQLs, want)
}

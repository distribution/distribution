// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"os"
	"time"
)

// Application represents your application.  All methods on Application are nil
// safe.  Therefore, a nil Application pointer can be safely used as a mock.
type Application struct {
	Private interface{}
	app     *app
}

// StartTransaction begins a Transaction with the given name.
func (app *Application) StartTransaction(name string, opts ...TraceOption) *Transaction {
	if app == nil {
		return nil
	}
	return app.app.StartTransaction(name, opts...)
}

// RecordCustomEvent adds a custom event.
//
// eventType must consist of alphanumeric characters, underscores, and
// colons, and must contain fewer than 255 bytes.
//
// Each value in the params map must be a number, string, or boolean.
// Keys must be less than 255 bytes.  The params map may not contain
// more than 64 attributes.  For more information, and a set of
// restricted keywords, see:
// https://docs.newrelic.com/docs/insights/new-relic-insights/adding-querying-data/inserting-custom-events-new-relic-apm-agents
//
// An error is logged if eventType or params is invalid.
func (app *Application) RecordCustomEvent(eventType string, params map[string]interface{}) {
	if app == nil || app.app == nil {
		return
	}
	err := app.app.RecordCustomEvent(eventType, params)
	if err != nil {
		app.app.Error("unable to record custom event", map[string]interface{}{
			"event-type": eventType,
			"reason":     err.Error(),
		})
	}
}

// RecordCustomMetric records a custom metric.  The metric name you
// provide will be prefixed by "Custom/".  Custom metrics are not
// currently supported in serverless mode.
//
// See
// https://docs.newrelic.com/docs/agents/manage-apm-agents/agent-data/collect-custom-metrics
// for more information on custom events.
func (app *Application) RecordCustomMetric(name string, value float64) {
	if app == nil || app.app == nil {
		return
	}
	err := app.app.RecordCustomMetric(name, value)
	if err != nil {
		app.app.Error("unable to record custom metric", map[string]interface{}{
			"metric-name": name,
			"reason":      err.Error(),
		})
	}
}

// RecordLog records the data from a single log line.
// This consumes a LogData object that should be configured
// with data taken from a logging framework.
//
// Certian parts of this feature can be turned off based on your
// config settings. Record log is capable of recording log events,
// as well as log metrics depending on how your application is
// configured.
func (app *Application) RecordLog(logEvent LogData) {
	if app == nil || app.app == nil {
		return
	}
	err := app.app.RecordLog(&logEvent)
	if err != nil {
		app.app.Error("unable to record log", map[string]interface{}{
			"reason": err.Error(),
		})
	}
}

// WaitForConnection blocks until the application is connected, is
// incapable of being connected, or the timeout has been reached.  This
// method is useful for short-lived processes since the application will
// not gather data until it is connected.  nil is returned if the
// application is connected successfully.
//
// If Infinite Tracing is enabled, WaitForConnection will block until a
// connection to the Trace Observer is made, a fatal error is reached, or the
// timeout is hit.
//
// Note that in most cases, it is not necesary nor recommended to call
// WaitForConnection() at all, particularly for any but the most trivial, short-lived
// processes. It is better to simply start the application and allow the
// instrumentation code to handle its connections on its own, which it will do
// as needed in the background (and will continue attempting to connect
// if it wasn't immediately successful, all while allowing your application
// to proceed with its primary function).
func (app *Application) WaitForConnection(timeout time.Duration) error {
	if app == nil || app.app == nil {
		return nil
	}
	return app.app.WaitForConnection(timeout)
}

// Shutdown flushes data to New Relic's servers and stops all
// agent-related goroutines managing this application.  After Shutdown
// is called, the Application is disabled and will never collect data
// again.  This method blocks until all final data is sent to New Relic
// or the timeout has elapsed.  Increase the timeout and check debug
// logs if you aren't seeing data.
//
// If Infinite Tracing is enabled, Shutdown will block until all queued span
// events have been sent to the Trace Observer or the timeout has been reached.
func (app *Application) Shutdown(timeout time.Duration) {
	if app == nil || app.app == nil {
		return
	}
	app.app.Shutdown(timeout)
}

// Config returns a copy of the application's configuration data in case
// that information is needed (but since it is a copy, this function cannot
// be used to alter the application's configuration).
//
// If the Config data could be copied from the application successfully,
// a boolean true value is returned as the second return value.  If it is
// false, then the Config data returned is the standard default configuration.
// This usually occurs if the Application is not yet fully initialized.
//
func (app *Application) Config() (Config, bool) {
	if app == nil || app.app == nil {
		return defaultConfig(), false
	}
	return app.app.config.Config, true
}
func newApplication(app *app) *Application {
	return &Application{
		app:     app,
		Private: app,
	}
}

// NewApplication creates an Application and spawns goroutines to manage the
// aggregation and harvesting of data.  On success, a non-nil Application and a
// nil error are returned. On failure, a nil Application and a non-nil error
// are returned. All methods on an Application are nil safe. Therefore, a nil
// Application pointer can be safely used.  Applications do not share global
// state, therefore it is safe to create multiple applications.
//
// The ConfigOption arguments allow for configuration of the Application.  They
// are applied in order from first to last, i.e. latter ConfigOptions may
// overwrite the Config fields already set.
func NewApplication(opts ...ConfigOption) (*Application, error) {
	c := defaultConfig()
	for _, fn := range opts {
		if fn != nil {
			fn(&c)
			if c.Error != nil {
				return nil, c.Error
			}
		}
	}
	cfg, err := newInternalConfig(c, os.Getenv, os.Environ())
	if err != nil {
		return nil, err
	}
	return newApplication(newApp(cfg)), nil
}

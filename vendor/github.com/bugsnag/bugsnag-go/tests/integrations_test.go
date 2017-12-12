// +build !appengine

package tests

import (
	"os/exec"
	"testing"
)

// Starts an app, sends a request, and tests that the resulting bugsnag
// error report has the correct values.

func TestNegroniRequestPanic(t *testing.T) {
	startTestServer()
	body := startPanickingApp(t,
		"./fixtures/negroni.go", "http://localhost:9078", "default")
	assertSeverityReasonEqual(t, body, "error", "unhandledErrorMiddleware", true)
	pkill("negroni")
}

func TestNegroniRequestPanicCallbackAltered(t *testing.T) {
	startTestServer()
	body := startPanickingApp(t,
		"./fixtures/negroni.go", "http://localhost:9078", "beforenotify")
	assertSeverityReasonEqual(t, body, "info", "userCallbackSetSeverity", true)
	pkill("negroni")
}

func TestGinRequestPanic(t *testing.T) {
	startTestServer()
	body := startPanickingApp(t, "./fixtures/gin.go", "http://localhost:9079", "default")
	assertSeverityReasonEqual(t, body, "error", "unhandledErrorMiddleware", true)
	pkill("gin")
}

func TestGinRequestPanicCallbackAltered(t *testing.T) {
	startTestServer()
	body := startPanickingApp(t, "./fixtures/gin.go", "http://localhost:9079", "beforenotify")
	assertSeverityReasonEqual(t, body, "info", "userCallbackSetSeverity", true)
	pkill("gin")
}

func TestMartiniRequestPanic(t *testing.T) {
	startTestServer()
	body := startPanickingApp(t, "./fixtures/martini.go", "http://localhost:3000", "default")
	assertSeverityReasonEqual(t, body, "error", "unhandledErrorMiddleware", true)
	pkill("martini")
}

func TestMartiniRequestPanicCallbackAltered(t *testing.T) {
	startTestServer()
	body := startPanickingApp(t, "./fixtures/martini.go", "http://localhost:3000", "beforenotify")
	assertSeverityReasonEqual(t, body, "info", "userCallbackSetSeverity", true)
	pkill("martini")
}

func TestRevelRequestPanic(t *testing.T) {
	startTestServer()
	body := startRevelApp(t, "default")
	assertSeverityReasonEqual(t, body, "error", "unhandledErrorMiddleware", true)
	pkill("revel")
}

func TestRevelRequestPanicCallbackAltered(t *testing.T) {
	startTestServer()
	body := startRevelApp(t, "beforenotify")
	assertSeverityReasonEqual(t, body, "info", "userCallbackSetSeverity", true)
	pkill("revel")
}

func pkill(process string) {
	cmd := exec.Command("pkill", "-x", process)
	cmd.Start()
	cmd.Wait()
}

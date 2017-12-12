package main

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/bugsnag/bugsnag-go/gin"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
)

func main() {
	g := gin.Default()

	g.Use(bugsnaggin.AutoNotify(bugsnag.Configuration{
		APIKey:   "166f5ad3590596f9aa8d601ea89af845",
		Endpoint: os.Getenv("BUGSNAG_ENDPOINT"),
	}))

	if os.Getenv("BUGSNAG_TEST_VARIANT") == "beforenotify" {
		bugsnag.OnBeforeNotify(func(event *bugsnag.Event, config *bugsnag.Configuration) error {
			event.Severity = bugsnag.SeverityInfo
			return nil
		})
	}
	g.GET("/", performUnhandledCrash)

	g.Run(":9079")
}

func performUnhandledCrash(c *gin.Context) {
	c.String(http.StatusOK, "OK")
	var a struct{}
	crash(a)
}

func crash(a interface{}) string {
	return a.(string)
}

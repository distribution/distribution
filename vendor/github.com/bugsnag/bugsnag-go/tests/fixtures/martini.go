package main

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/bugsnag/bugsnag-go/martini"
	"github.com/go-martini/martini"
	"os"
)

func main() {
	if os.Getenv("BUGSNAG_TEST_VARIANT") == "beforenotify" {
		bugsnag.OnBeforeNotify(func(event *bugsnag.Event, config *bugsnag.Configuration) error {
			event.Severity = bugsnag.SeverityInfo
			return nil
		})
	}
	m := martini.Classic()
	m.Get("/", func() string {
		var a struct{}
		crash(a)
		return "Hello world!"
	})
	m.Use(martini.Recovery())
	m.Use(bugsnagmartini.AutoNotify(bugsnag.Configuration{
		APIKey:   "166f5ad3590596f9aa8d601ea89af845",
		Endpoint: os.Getenv("BUGSNAG_ENDPOINT"),
	}))
	m.Run()
}

func crash(a interface{}) string {
	return a.(string)
}

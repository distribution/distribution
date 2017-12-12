package main

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/bugsnag/bugsnag-go/negroni"
	"github.com/urfave/negroni"
	"net/http"
	"os"
)

func main() {
	errorReporterConfig := bugsnag.Configuration{
		APIKey:   "166f5ad3590596f9aa8d601ea89af845",
		Endpoint: os.Getenv("BUGSNAG_ENDPOINT"),
	}
	if os.Getenv("BUGSNAG_TEST_VARIANT") == "beforenotify" {
		bugsnag.OnBeforeNotify(func(event *bugsnag.Event, config *bugsnag.Configuration) error {
			event.Severity = bugsnag.SeverityInfo
			return nil
		})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("OK\n"))

		var a struct{}
		crash(a)
	})

	n := negroni.New()
	n.Use(negroni.NewRecovery())
	n.Use(bugsnagnegroni.AutoNotify(errorReporterConfig))
	n.UseHandler(mux)

	http.ListenAndServe(":9078", n)
}

func crash(a interface{}) string {
	return a.(string)
}

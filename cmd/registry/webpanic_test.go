package main

import (
	"encoding/json"
	"errors"
	log "github.com/Sirupsen/logrus"
	"github.com/bugsnag/bugsnag-go"
	"github.com/docker/distribution/registry/handlers"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

var (
	exceptPanic = "It is test Panic!"
	exceptError = "It is test Error!"
)

func TestWebPanic(t *testing.T) {
	var app handlers.App
	app.Config.Reporting.Bugsnag.APIKey = "12345678901234567890123456789012"
	app.Config.Reporting.Bugsnag.Endpoint = "localhost:7092"
	app.Config.Reporting.Bugsnag.ReleaseStage = "production"
	if err := configureReportingBugsnag(&app); err != nil {
		t.Fatalf("%v", err)
	}
	portMap := strings.Split(app.Config.Reporting.Bugsnag.Endpoint, ":")
	port := ":" + portMap[1]
	go http.ListenAndServe(port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var notice struct {
			Events []struct {
				Exceptions []struct {
					Message string `json:"message"`
				} `json:"exceptions"`
			} `json:"events"`
		}
		data, _ := ioutil.ReadAll(r.Body)
		if err := json.Unmarshal(data, &notice); err != nil {
			log.Error(err)
			return
		}
		_ = r.Body.Close()
		if notice.Events[0].Exceptions[0].Message == exceptError {
			t.Fatalf("Out of Panic Level!")
		} else if notice.Events[0].Exceptions[0].Message != exceptPanic {
			t.Fatalf("Unexcepted Info!")
		}
	}))

	log.WithFields(log.Fields{
		"error": errors.New(exceptError),
	}).Error("Won't dispplay in handler")
	defer bugsnag.Recover()
	panic(exceptPanic)
}

package handlers

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/health"
)

func TestFileHealthCheck(t *testing.T) {
	// In case other tests registered checks before this one
	health.UnregisterAll()

	interval := time.Second

	tmpfile, err := ioutil.TempFile(os.TempDir(), "healthcheck")
	if err != nil {
		t.Fatalf("could not create temporary file: %v", err)
	}
	defer tmpfile.Close()

	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
		Health: configuration.Health{
			FileCheckers: []configuration.FileChecker{
				{
					Interval: interval,
					File:     tmpfile.Name(),
				},
			},
		},
	}

	ctx := context.Background()

	app := NewApp(ctx, config)
	app.RegisterHealthChecks()

	debugServer := httptest.NewServer(nil)

	// Wait for health check to happen
	<-time.After(2 * interval)

	resp, err := http.Get(debugServer.URL + "/debug/health")
	if err != nil {
		t.Fatalf("error performing HTTP GET: %v", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading HTTP body: %v", err)
	}
	resp.Body.Close()
	var decoded map[string]string
	err = json.Unmarshal(body, &decoded)
	if err != nil {
		t.Fatalf("error unmarshaling json: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatal("expected 1 item in returned json")
	}
	if decoded[tmpfile.Name()] != "file exists" {
		t.Fatal(`did not get "file exists" result for health check`)
	}

	os.Remove(tmpfile.Name())

	<-time.After(2 * interval)
	resp, err = http.Get(debugServer.URL + "/debug/health")
	if err != nil {
		t.Fatalf("error performing HTTP GET: %v", err)
	}
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading HTTP body: %v", err)
	}
	resp.Body.Close()
	var decoded2 map[string]string
	err = json.Unmarshal(body, &decoded2)
	if err != nil {
		t.Fatalf("error unmarshaling json: %v", err)
	}
	if len(decoded2) != 0 {
		t.Fatal("expected 0 items in returned json")
	}
}

func TestHTTPHealthCheck(t *testing.T) {
	// In case other tests registered checks before this one
	health.UnregisterAll()

	interval := time.Second
	threshold := 3

	stopFailing := make(chan struct{})

	checkedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Fatalf("expected HEAD request, got %s", r.Method)
		}
		select {
		case <-stopFailing:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))

	config := configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
		},
		Health: configuration.Health{
			HTTPCheckers: []configuration.HTTPChecker{
				{
					Interval:  interval,
					URI:       checkedServer.URL,
					Threshold: threshold,
				},
			},
		},
	}

	ctx := context.Background()

	app := NewApp(ctx, config)
	app.RegisterHealthChecks()

	debugServer := httptest.NewServer(nil)

	for i := 0; ; i++ {
		<-time.After(interval)

		resp, err := http.Get(debugServer.URL + "/debug/health")
		if err != nil {
			t.Fatalf("error performing HTTP GET: %v", err)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("error reading HTTP body: %v", err)
		}
		resp.Body.Close()
		var decoded map[string]string
		err = json.Unmarshal(body, &decoded)
		if err != nil {
			t.Fatalf("error unmarshaling json: %v", err)
		}

		if i < threshold-1 {
			// definitely shouldn't have hit the threshold yet
			if len(decoded) != 0 {
				t.Fatal("expected 1 items in returned json")
			}
			continue
		}
		if i < threshold+1 {
			// right on the threshold - don't expect a failure yet
			continue
		}

		if len(decoded) != 1 {
			t.Fatal("expected 1 item in returned json")
		}
		if decoded[checkedServer.URL] != "downstream service returned unexpected status: 500" {
			t.Fatal("did not get expected result for health check")
		}

		break
	}

	// Signal HTTP handler to start returning 200
	close(stopFailing)

	<-time.After(2 * interval)
	resp, err := http.Get(debugServer.URL + "/debug/health")
	if err != nil {
		t.Fatalf("error performing HTTP GET: %v", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading HTTP body: %v", err)
	}
	resp.Body.Close()
	var decoded map[string]string
	err = json.Unmarshal(body, &decoded)
	if err != nil {
		t.Fatalf("error unmarshaling json: %v", err)
	}
	if len(decoded) != 0 {
		t.Fatal("expected 0 items in returned json")
	}
}

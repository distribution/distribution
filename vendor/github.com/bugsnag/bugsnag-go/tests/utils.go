package tests

import (
	"github.com/bitly/go-simplejson"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

var postedJSON = make(chan []byte, 10)
var testOnce sync.Once
var testEndpoint string

func startTestServer() {
	testOnce.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				panic(err)
			}
			postedJSON <- body
		})

		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(err)
		}
		testEndpoint = "http://" + l.Addr().String() + "/"

		go http.Serve(l, mux)
	})
}

func assertSeverityReasonEqual(t *testing.T, body *simplejson.Json, expSeverity string, reasonType string, expUnhandled bool) {
	event := body.Get("events").GetIndex(0)
	reason := event.GetPath("severityReason", "type").MustString()
	severity := event.Get("severity").MustString()
	unhandled := event.Get("unhandled").MustBool()

	if reason != reasonType {
		t.Errorf("Wrong severity reason, expected '%s', received '%s'", reasonType, reason)
	}

	if severity != expSeverity {
		t.Errorf("Wrong severity, expected '%s', received '%s'", expSeverity, severity)
	}

	if unhandled != expUnhandled {
		t.Errorf("Wrong unhandled value, expected '%d', received '%d'", expUnhandled, unhandled)
	}
}

func startPanickingApp(t *testing.T,
	filename string, host string, variant string) *simplejson.Json {

	cmd := exec.Command("go", "run", filename)
	cmd.Env = append(os.Environ(),
		"BUGSNAG_ENDPOINT="+testEndpoint,
		"BUGSNAG_TEST_VARIANT="+variant)

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	_, err := http.Get(host)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second)
	body, err := simplejson.NewJson(<-postedJSON)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func startRevelApp(t *testing.T, variant string) *simplejson.Json {
	cmd := exec.Command("revel", "run", "github.com/bugsnag/bugsnag-go/tests/fixtures/revel")
	cmd.Env = append(os.Environ(),
		"BUGSNAG_ENDPOINT="+testEndpoint,
		"BUGSNAG_TEST_VARIANT="+variant)

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	_, err := http.Get("http://localhost:9091")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second)
	body, err := simplejson.NewJson(<-postedJSON)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

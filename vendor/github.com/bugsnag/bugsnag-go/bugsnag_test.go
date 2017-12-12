package bugsnag

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bitly/go-simplejson"
)

func TestConfigure(t *testing.T) {
	Configure(Configuration{
		APIKey: testAPIKey,
	})

	if Config.APIKey != testAPIKey {
		t.Errorf("Setting APIKey didn't work")
	}

	if New().Config.APIKey != testAPIKey {
		t.Errorf("Setting APIKey didn't work for new notifiers")
	}
}

var postedJSON = make(chan []byte, 10)
var testOnce sync.Once
var testEndpoint string
var testAPIKey = "166f5ad3590596f9aa8d601ea89af845"

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

type _recurse struct {
	*_recurse
}

func TestNotify(t *testing.T) {
	startTestServer()

	recurse := _recurse{}
	recurse._recurse = &recurse

	OnBeforeNotify(func(event *Event, config *Configuration) error {
		if event.Context == "testing" {
			event.GroupingHash = "lol"
		}
		return nil
	})

	Notify(fmt.Errorf("hello world"),
		Configuration{
			APIKey:          testAPIKey,
			Endpoint:        testEndpoint,
			ReleaseStage:    "test",
			AppType:         "foo",
			AppVersion:      "1.2.3",
			Hostname:        "web1",
			ProjectPackages: []string{"github.com/bugsnag/bugsnag-go"},
		},
		User{Id: "123", Name: "Conrad", Email: "me@cirw.in"},
		Context{"testing"},
		MetaData{"test": {
			"password": "sneaky",
			"value":    "able",
			"broken":   complex(1, 2),
			"recurse":  recurse,
		}},
	)

	json, err := simplejson.NewJson(<-postedJSON)

	if err != nil {
		t.Fatal(err)
	}

	if json.Get("apiKey").MustString() != testAPIKey {
		t.Errorf("Wrong api key in payload")
	}

	if json.GetPath("notifier", "name").MustString() != "Bugsnag Go" {
		t.Errorf("Wrong notifier name in payload")
	}

	event := json.Get("events").GetIndex(0)

	for k, value := range map[string]string{
		"payloadVersion":                 "2",
		"severity":                       "warning",
		"context":                        "testing",
		"groupingHash":                   "lol",
		"app.releaseStage":               "test",
		"app.type":                       "foo",
		"app.version":                    "1.2.3",
		"device.hostname":                "web1",
		"user.id":                        "123",
		"user.name":                      "Conrad",
		"user.email":                     "me@cirw.in",
		"metaData.test.password":         "[REDACTED]",
		"metaData.test.value":            "able",
		"metaData.test.broken":           "[complex128]",
		"metaData.test.recurse._recurse": "[RECURSION]",
	} {
		key := strings.Split(k, ".")
		if event.GetPath(key...).MustString() != value {
			t.Errorf("Wrong %v: %v != %v", key, event.GetPath(key...).MustString(), value)
		}
	}

	exception := event.Get("exceptions").GetIndex(0)

	if exception.Get("message").MustString() != "hello world" {
		t.Errorf("Wrong message in payload")
	}

	if exception.Get("errorClass").MustString() != "*errors.errorString" {
		t.Errorf("Wrong errorClass in payload: %v", exception.Get("errorClass").MustString())
	}

	frame0 := exception.Get("stacktrace").GetIndex(0)
	if frame0.Get("file").MustString() != "bugsnag_test.go" ||
		frame0.Get("method").MustString() != "TestNotify" ||
		frame0.Get("inProject").MustBool() != true ||
		frame0.Get("lineNumber").MustInt() == 0 {
		t.Errorf("Wrong frame0")
	}

	frame1 := exception.Get("stacktrace").GetIndex(1)

	if frame1.Get("file").MustString() != "testing/testing.go" ||
		frame1.Get("method").MustString() != "tRunner" ||
		frame1.Get("inProject").MustBool() != false ||
		frame1.Get("lineNumber").MustInt() == 0 {
		t.Errorf("Wrong frame1")
	}
}

func crashyHandler(w http.ResponseWriter, r *http.Request) {
	c := make(chan int)
	close(c)
	c <- 1
}

func runCrashyServer(rawData ...interface{}) (net.Listener, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", crashyHandler)
	srv := http.Server{
		Addr:     l.Addr().String(),
		Handler:  Handler(mux, rawData...),
		ErrorLog: log.New(ioutil.Discard, log.Prefix(), 0),
	}

	go srv.Serve(l)
	return l, err
}

func TestHandler(t *testing.T) {
	startTestServer()

	l, err := runCrashyServer(Configuration{
		APIKey:          testAPIKey,
		Endpoint:        testEndpoint,
		ProjectPackages: []string{"github.com/bugsnag/bugsnag-go"},
		Logger:          log.New(ioutil.Discard, log.Prefix(), log.Flags()),
	}, SeverityInfo)
	if err != nil {
		t.Fatal(err)
	}
	http.Get("http://" + l.Addr().String() + "/ok?foo=bar")
	l.Close()

	json, err := simplejson.NewJson(<-postedJSON)
	if err != nil {
		t.Fatal(err)
	}

	if json.Get("apiKey").MustString() != testAPIKey {
		t.Errorf("Wrong api key in payload")
	}

	if json.GetPath("notifier", "name").MustString() != "Bugsnag Go" {
		t.Errorf("Wrong notifier name in payload")
	}

	event := json.Get("events").GetIndex(0)

	for k, value := range map[string]string{
		"payloadVersion":              "2",
		"severity":                    "info",
		"user.id":                     "127.0.0.1",
		"metaData.request.url":        "http://" + l.Addr().String() + "/ok?foo=bar",
		"metaData.request.httpMethod": "GET",
	} {
		key := strings.Split(k, ".")
		if event.GetPath(key...).MustString() != value {
			t.Errorf("Wrong %v: %v != %v", key, event.GetPath(key...).MustString(), value)
		}
	}

	if event.GetPath("metaData", "request", "params", "foo").GetIndex(0).MustString() != "bar" {
		t.Errorf("missing GET params in request metadata")
	}

	if event.GetPath("metaData", "request", "headers", "Accept-Encoding").GetIndex(0).MustString() != "gzip" {
		t.Errorf("missing GET params in request metadata: %v", event.GetPath("metaData", "request", "headers"))
	}

	exception := event.Get("exceptions").GetIndex(0)

	if !strings.Contains(exception.Get("message").MustString(), "send on closed channel") {
		t.Errorf("Wrong message in payload: %v '%v'", exception.Get("message").MustString(), "runtime error: send on closed channel")
	}

	errorClass := exception.Get("errorClass").MustString()
	if errorClass != "runtime.errorCString" && errorClass != "*errors.errorString" && errorClass != "runtime.plainError" {
		t.Errorf("Wrong errorClass in payload: %v, expected '%v', '%v', '%v'",
			exception.Get("errorClass").MustString(),
			"runtime.errorCString", "*errors.errorString", "runtime.plainError")
	}

	frame0 := exception.Get("stacktrace").GetIndex(0)

	file0 := frame0.Get("file").MustString()
	if !strings.HasPrefix(file0, "runtime/panic") ||
		frame0.Get("inProject").MustBool() != false {
		t.Errorf("Wrong frame0: %v", frame0)
	}

	frame3 := exception.Get("stacktrace").GetIndex(3)

	if frame3.Get("file").MustString() != "bugsnag_test.go" ||
		frame3.Get("method").MustString() != "crashyHandler" ||
		frame3.Get("inProject").MustBool() != true ||
		frame3.Get("lineNumber").MustInt() == 0 {
		t.Errorf("Wrong frame3: %v", frame3)
	}
}

func TestAutoNotify(t *testing.T) {

	var panicked interface{}

	func() {
		defer func() {
			panicked = recover()
		}()
		defer AutoNotify(Configuration{Endpoint: testEndpoint, APIKey: testAPIKey})

		panic("eggs")
	}()

	if panicked.(string) != "eggs" {
		t.Errorf("didn't re-panic")
	}

	json, err := simplejson.NewJson(<-postedJSON)
	if err != nil {
		t.Fatal(err)
	}

	event := json.Get("events").GetIndex(0)

	if event.Get("severity").MustString() != "error" {
		t.Errorf("severity should be error")
	}
	exception := event.Get("exceptions").GetIndex(0)

	if exception.Get("message").MustString() != "eggs" {
		t.Errorf("caught wrong panic")
	}
	assertSeverityReasonEqual(t, json, "error", "handledPanic", true)
}

func TestRecover(t *testing.T) {
	var panicked interface{}

	func() {
		defer func() {
			panicked = recover()
		}()
		defer Recover(Configuration{Endpoint: testEndpoint, APIKey: testAPIKey})

		panic("ham")
	}()

	if panicked != nil {
		t.Errorf("re-panick'd")
	}

	json, err := simplejson.NewJson(<-postedJSON)
	if err != nil {
		t.Fatal(err)
	}

	event := json.Get("events").GetIndex(0)

	if event.Get("severity").MustString() != "warning" {
		t.Errorf("severity should be warning")
	}
	exception := event.Get("exceptions").GetIndex(0)

	if exception.Get("message").MustString() != "ham" {
		t.Errorf("caught wrong panic")
	}
	assertSeverityReasonEqual(t, json, "warning", "handledPanic", false)
}

func handleGet(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Handling GET")
}

var createAccount = handleGet

type _job struct {
	Name    string
	Process func()
}

func ExampleAutoNotify() {
	f := func(w http.ResponseWriter, request *http.Request) {
		defer AutoNotify(request, Context{"createAccount"})

		createAccount(w, request)
	}
	var w http.ResponseWriter
	var request *http.Request
	f(w, request)
	// Output:
	// Handling GET
}

func ExampleRecover() {
	job := _job{
		Name: "Example",
		Process: func() {
			fmt.Println("About to panic")
			panic("Oh noes")
		},
	}

	func() {
		defer Recover(Configuration{Endpoint: testEndpoint, APIKey: testAPIKey})
		job.Process()
	}()
	fmt.Println("Panic recovered")
	// Output:
	// About to panic
	// Panic recovered
}

func ExampleConfigure() {
	Configure(Configuration{
		APIKey: "YOUR_API_KEY_HERE",

		ReleaseStage: "production",

		// See Configuration{} for other fields
	})
}

func ExampleHandler() {
	// Set up your http handlers as usual
	http.HandleFunc("/", handleGet)

	// use bugsnag.Handler(nil) to wrap the default http handlers
	// so that Bugsnag is automatically notified about panics.
	http.ListenAndServe(":1234", Handler(nil))
}

func ExampleHandler_customServer() {
	// If you're using a custom server, set the handlers explicitly.
	http.HandleFunc("/", handleGet)

	srv := http.Server{
		Addr:        ":1234",
		ReadTimeout: 10 * time.Second,
		// use bugsnag.Handler(nil) to wrap the default http handlers
		// so that Bugsnag is automatically notified about panics.
		Handler: Handler(nil),
	}
	srv.ListenAndServe()
}

func ExampleHandler_customHandlers() {
	// If you're using custom handlers, wrap the handlers explicitly.
	handler := http.NewServeMux()
	http.HandleFunc("/", handleGet)
	// use bugsnag.Handler(handler) to wrap the handlers so that Bugsnag is
	// automatically notified about panics
	http.ListenAndServe(":1234", Handler(handler))
}

func ExampleNotify() {
	_, err := net.Listen("tcp", ":80")

	if err != nil {
		Notify(err)
	}
}

func ExampleNotify_details() {
	_, err := net.Listen("tcp", ":80")
	userID := "123456789"

	if err != nil {
		Notify(err,
			// show as low-severity
			SeverityInfo,
			// set the context
			Context{"createlistener"},
			// pass the user id in to count users affected.
			User{Id: userID},
			// custom meta-data tab
			MetaData{
				"Listen": {
					"Protocol": "tcp",
					"Port":     "80",
				},
			},
		)
	}

}

type Job struct {
	Retry     bool
	UserId    string
	UserEmail string
	Name      string
	Params    map[string]string
}

func ExampleOnBeforeNotify() {
	OnBeforeNotify(func(event *Event, config *Configuration) error {

		// Search all the RawData for any *Job pointers that we're passed in
		// to bugsnag.Notify() and friends.
		for _, datum := range event.RawData {
			if job, ok := datum.(*Job); ok {
				// don't notify bugsnag about errors in retries
				if job.Retry {
					return fmt.Errorf("bugsnag middleware: not notifying about job retry")
				}

				// add the job as a tab on Bugsnag.com
				event.MetaData.AddStruct("Job", job)

				// set the user correctly
				event.User = &User{Id: job.UserId, Email: job.UserEmail}
			}
		}

		// continue notifying as normal
		return nil
	})
}

func TestSeverityReasonNotifyErr(t *testing.T) {
	startTestServer()

	Notify(fmt.Errorf("hello world"), generateSampleConfig())

	json, _ := simplejson.NewJson(<-postedJSON)
	assertSeverityReasonEqual(t, json, "warning", "handledError", false)
}

func TestSeverityReasonNotifyCallback(t *testing.T) {
	startTestServer()

	OnBeforeNotify(func(event *Event, config *Configuration) error {
		event.Severity = SeverityInfo
		return nil
	})

	Notify(fmt.Errorf("hello world"), generateSampleConfig())

	json, _ := simplejson.NewJson(<-postedJSON)
	assertSeverityReasonEqual(t, json, "info", "userCallbackSetSeverity", false)
}

func assertSeverityReasonEqual(t *testing.T, json *simplejson.Json, expSeverity string, reasonType string, expUnhandled bool) {
	event := json.Get("events").GetIndex(0)
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

func generateSampleConfig() Configuration {
	return Configuration{
		APIKey:          testAPIKey,
		Endpoint:        testEndpoint,
		ReleaseStage:    "test",
		AppType:         "foo",
		AppVersion:      "1.2.3",
		Hostname:        "web1",
		ProjectPackages: []string{"github.com/bugsnag/bugsnag-go"},
	}
}

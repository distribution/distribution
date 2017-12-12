package mellow

import (
	"fmt"
	"github.com/bugsnag/bugsnag-go"
	"net/http"
	"os"
)

func init() {
	bugsnag.OnBeforeNotify(func(event *bugsnag.Event, config *bugsnag.Configuration) error {
		event.MetaData.AddStruct("original", event.Error.StackFrames())
		return nil
	})

	// Insert your API key
	bugsnag.Configure(bugsnag.Configuration{
		APIKey: "YOUR-API-KEY-HERE",
	})

	http.HandleFunc("/", bugsnag.HandlerFunc(handler))
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "welcome")
	notifier := bugsnag.New(r)
	notifier.Notify(fmt.Errorf("oh hia"), bugsnag.MetaData{"env": {"values": os.Environ()}})
	fmt.Fprint(w, "welcome\n")

	panic("zoomg")
}

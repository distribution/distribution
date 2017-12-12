package bugsnagnegroni

import (
	"github.com/bugsnag/bugsnag-go"
	"net/http"
)

const FrameworkName string = "Negroni"

type handler struct {
	rawData []interface{}
}

func AutoNotify(rawData ...interface{}) *handler {
	state := bugsnag.HandledState{
		bugsnag.SeverityReasonUnhandledMiddlewareError,
		bugsnag.SeverityError,
		true,
		FrameworkName,
	}
	rawData = append(rawData, state)
	return &handler{
		rawData: rawData,
	}
}

func (h *handler) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	notifier := bugsnag.New(append(h.rawData, r)...)
	defer notifier.AutoNotify(r)
	next(rw, r)
}

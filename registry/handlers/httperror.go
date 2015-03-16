package handlers

import (
	"github.com/docker/distribution/registry/api/v2"
	"net/http"
)

// httpError is a wrapper for v2.Errors that adds a Status int to hold
// an HTTP Status code which will be used to set the status code on
// the response.
type httpError struct {
	v2.Errors
	Status int
}

// NewHTTPError create a new httpError using the given ErrorCode, detail and http status.
// detail must be json serializable.
func NewHTTPError(errCode v2.ErrorCode, detail interface{}, status int) error {
	errs := v2.Errors{}
	if errCode > 0 {
		errs.Push(errCode, detail)
	}
	newErr := httpError{
		errs,
		status,
	}
	return newErr
}

// ServeError is currently just a pass through to serveJSONStatus but its use will
// allow us to easily make changes to how errors are served in the future.
func (err *httpError) ServeError(w http.ResponseWriter) {
	serveJSONStatus(w, err.Errors, err.Status)
}

func (err httpError) Error() string {
	return err.Errors.Error()
}

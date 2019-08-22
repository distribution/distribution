package errcode

import (
	"encoding/json"
	"net/http"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/docker/distribution/registry/storage/driver"
)

// ServeJSON attempts to serve the errcode in a JSON envelope. It marshals err
// and sets the content-type header to 'application/json'. It will handle
// ErrorCoder and Errors, and if necessary will create an envelope.
func ServeJSON(w http.ResponseWriter, err error) error {
	w.Header().Set("Content-Type", "application/json")
	var sc int

	switch errs := err.(type) {
	case Errors:
		if len(errs) < 1 {
			break
		}

		for i := range errs {
			if err2, ok := errs[i].(Error); ok {
				errs[i] = replaceError(err2)
			}
		}

		if err, ok := errs[0].(ErrorCoder); ok {
			sc = err.ErrorCode().Descriptor().HTTPStatusCode
		}
	case ErrorCoder:
		if err2, ok := errs.(Error); ok {
			errs = replaceError(err2)
		}

		sc = errs.ErrorCode().Descriptor().HTTPStatusCode
		err = Errors{err} // create an envelope.
	default:
		if err2, ok := err.(Error); ok {
			err = replaceError(err2)
		}

		// We just have an unhandled error type, so just place in an envelope
		// and move along.
		err = Errors{err}
	}

	if sc == 0 {
		sc = http.StatusInternalServerError
	}

	w.WriteHeader(sc)

	return json.NewEncoder(w).Encode(err)
}

func replaceError(e Error) Error {
	serr, ok := e.Detail.(driver.Error)
	if !ok {
		return e
	}

	err, ok := serr.Enclosed.(awserr.RequestFailure)
	if !ok {
		return e
	}

	code := ErrorCodeUnknown
	switch err.StatusCode() {
	case http.StatusForbidden:
		code = ErrorCodeDenied
	case http.StatusServiceUnavailable:
		code = ErrorCodeUnavailable
	case http.StatusUnauthorized:
		code = ErrorCodeUnauthorized
	case http.StatusTooManyRequests:
		code = ErrorCodeTooManyRequests
	}

	return code.WithDetail(err.Code())
}

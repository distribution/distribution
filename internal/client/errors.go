package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/distribution/distribution/v3/internal/client/auth/challenge"
	"github.com/distribution/distribution/v3/registry/api/errcode"
)

// ErrNoErrorsInBody is returned when an HTTP response body parses to an empty
// errcode.Errors slice.
var ErrNoErrorsInBody = errors.New("no error details found in HTTP response body")

// UnexpectedHTTPStatusError is returned when an unexpected HTTP status is
// returned when making a registry api call.
type UnexpectedHTTPStatusError struct {
	Status string
}

func (e *UnexpectedHTTPStatusError) Error() string {
	return fmt.Sprintf("received unexpected HTTP status: %s", e.Status)
}

// UnexpectedHTTPResponseError is returned when an expected HTTP status code
// is returned, but the content was unexpected and failed to be parsed.
type UnexpectedHTTPResponseError struct {
	ParseErr   error
	StatusCode int
	Response   []byte
}

func (e *UnexpectedHTTPResponseError) Error() string {
	return fmt.Sprintf("error parsing HTTP %d response body: %s: %q", e.StatusCode, e.ParseErr.Error(), string(e.Response))
}

func parseHTTPErrorResponse(resp *http.Response) error {
	var errors errcode.Errors
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	statusCode := resp.StatusCode

	// A HEAD request for example validly does not contain any body, while
	// still returning a JSON content-type.
	if len(body) == 0 {
		return makeError(statusCode, "")
	}

	ctHeader := resp.Header.Get("Content-Type")
	if ctHeader == "" {
		return makeError(statusCode, string(body))
	}

	contentType, _, err := mime.ParseMediaType(ctHeader)
	if err != nil {
		return fmt.Errorf("failed parsing content-type: %w", err)
	}

	if contentType != "application/json" && contentType != "application/vnd.api+json" {
		return makeError(statusCode, string(body))
	}

	// For backward compatibility, handle irregularly formatted
	// messages that contain a "details" field.
	var detailsErr struct {
		Details string `json:"details"`
	}
	err = json.Unmarshal(body, &detailsErr)
	if err == nil && detailsErr.Details != "" {
		return makeError(statusCode, detailsErr.Details)
	}

	if err := json.Unmarshal(body, &errors); err != nil {
		return &UnexpectedHTTPResponseError{
			ParseErr:   err,
			StatusCode: statusCode,
			Response:   body,
		}
	}

	if len(errors) == 0 {
		// If there was no error specified in the body, return
		// UnexpectedHTTPResponseError.
		return &UnexpectedHTTPResponseError{
			ParseErr:   ErrNoErrorsInBody,
			StatusCode: statusCode,
			Response:   body,
		}
	}

	return errors
}

func makeError(statusCode int, details string) error {
	switch statusCode {
	case http.StatusUnauthorized:
		return errcode.ErrorCodeUnauthorized.WithMessage(details)
	case http.StatusForbidden:
		return errcode.ErrorCodeDenied.WithMessage(details)
	case http.StatusTooManyRequests:
		return errcode.ErrorCodeTooManyRequests.WithMessage(details)
	default:
		return errcode.ErrorCodeUnknown.WithMessage(details)
	}
}

func makeErrorList(err error) []error {
	if errL, ok := err.(errcode.Errors); ok {
		return []error(errL)
	}
	return []error{err}
}

func mergeErrors(err1, err2 error) error {
	return errcode.Errors(append(makeErrorList(err1), makeErrorList(err2)...))
}

// HandleHTTPResponseError returns error parsed from HTTP response, if any.
// It returns nil if no error occurred (HTTP status 200-399), or an error
// for unsuccessful HTTP response codes (in the range 400 - 499 inclusive).
// If possible, it returns a typed error, but an UnexpectedHTTPStatusError
// is returned for response code outside the expected range (HTTP status < 200
// and > 500).
func HandleHTTPResponseError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
		return nil
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Check for OAuth errors within the `WWW-Authenticate` header first
		// See https://tools.ietf.org/html/rfc6750#section-3
		for _, c := range challenge.ResponseChallenges(resp) {
			if c.Scheme == "bearer" {
				var err errcode.Error
				// codes defined at https://tools.ietf.org/html/rfc6750#section-3.1
				switch c.Parameters["error"] {
				case "invalid_token":
					err.Code = errcode.ErrorCodeUnauthorized
				case "insufficient_scope":
					err.Code = errcode.ErrorCodeDenied
				default:
					continue
				}
				if description := c.Parameters["error_description"]; description != "" {
					err.Message = description
				} else {
					err.Message = err.Code.Message()
				}
				return mergeErrors(err, parseHTTPErrorResponse(resp))
			}
		}
		err := parseHTTPErrorResponse(resp)
		if uErr, ok := err.(*UnexpectedHTTPResponseError); ok && resp.StatusCode == 401 {
			return errcode.ErrorCodeUnauthorized.WithDetail(uErr.Response)
		}
		return err
	}
	return &UnexpectedHTTPStatusError{Status: resp.Status}
}

// HandleErrorResponse returns error parsed from HTTP response for an
// unsuccessful HTTP response code (in the range 400 - 499 inclusive). An
// UnexpectedHTTPStatusError returned for response code outside of expected
// range.
//
// Deprecated: use [HandleHTTPResponseError] and check the error.
func HandleErrorResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
		return &UnexpectedHTTPStatusError{Status: resp.Status}
	}
	return HandleHTTPResponseError(resp)
}

// SuccessStatus returns true if the argument is a successful HTTP response
// code (in the range 200 - 399 inclusive).
//
// Deprecated: use [HandleHTTPResponseError] and check the error.
func SuccessStatus(status int) bool {
	return status >= 200 && status <= 399
}

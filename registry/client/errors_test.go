package client

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func TestHandleHTTPResponseError200ValidBody(t *testing.T) {
	response := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
	}
	err := HandleHTTPResponseError(response)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
}

func TestHandleHTTPResponseError401ValidBody(t *testing.T) {
	json := `{"errors":[{"code":"UNAUTHORIZED","message":"action requires authentication"}]}`
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       nopCloser{bytes.NewBufferString(json)},
		Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
	}
	err := HandleHTTPResponseError(response)

	expectedMsg := "unauthorized: action requires authentication"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected %q, got: %q", expectedMsg, err.Error())
	}
}

func TestHandleHTTPResponseError401WithInvalidBody(t *testing.T) {
	json := "{invalid json}"
	response := &http.Response{
		Status:     "401 Unauthorized",
		StatusCode: 401,
		Body:       nopCloser{bytes.NewBufferString(json)},
		Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
	}
	err := HandleHTTPResponseError(response)

	expectedMsg := "unauthorized: authentication required"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected %q, got: %q", expectedMsg, err.Error())
	}
}

func TestHandleHTTPResponseErrorExpectedStatusCode400ValidBody(t *testing.T) {
	json := `{"errors":[{"code":"DIGEST_INVALID","message":"provided digest does not match"}]}`
	response := &http.Response{
		Status:     "400 Bad Request",
		StatusCode: 400,
		Body:       nopCloser{bytes.NewBufferString(json)},
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	err := HandleHTTPResponseError(response)

	expectedMsg := "digest invalid: provided digest does not match"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected %q, got: %q", expectedMsg, err.Error())
	}
}

func TestHandleHTTPResponseErrorExpectedStatusCode404EmptyErrorSlice(t *testing.T) {
	json := `{"randomkey": "randomvalue"}`
	response := &http.Response{
		Status:     "404 Not Found",
		StatusCode: 404,
		Body:       nopCloser{bytes.NewBufferString(json)},
		Header:     http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
	}
	err := HandleHTTPResponseError(response)

	expectedMsg := `error parsing HTTP 404 response body: no error details found in HTTP response body: "{\"randomkey\": \"randomvalue\"}"`
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected %q, got: %q", expectedMsg, err.Error())
	}
}

func TestHandleHTTPResponseErrorExpectedStatusCode404InvalidBody(t *testing.T) {
	json := "{invalid json}"
	response := &http.Response{
		Status:     "404 Not Found",
		StatusCode: 404,
		Body:       nopCloser{bytes.NewBufferString(json)},
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	err := HandleHTTPResponseError(response)

	expectedMsg := "error parsing HTTP 404 response body: invalid character 'i' looking for beginning of object key string: \"{invalid json}\""
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected %q, got: %q", expectedMsg, err.Error())
	}
}

func TestHandleHTTPResponseErrorUnexpectedStatusCode501(t *testing.T) {
	response := &http.Response{
		Status:     "501 Not Implemented",
		StatusCode: 501,
		Body:       nopCloser{bytes.NewBufferString("{\"Error Encountered\" : \"Function not implemented.\"}")},
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	err := HandleHTTPResponseError(response)

	expectedMsg := "received unexpected HTTP status: 501 Not Implemented"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected %q, got: %q", expectedMsg, err.Error())
	}
}

func TestHandleHTTPResponseErrorInsufficientPrivileges403(t *testing.T) {
	json := `{"details":"requesting higher privileges than access token allows"}`
	response := &http.Response{
		Status:     "403 Forbidden",
		StatusCode: 403,
		Body:       nopCloser{bytes.NewBufferString(json)},
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	err := HandleHTTPResponseError(response)

	expectedMsg := "denied: requesting higher privileges than access token allows"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected %q, got: %q", expectedMsg, err.Error())
	}
}

func TestHandleHTTPResponseErrorNonJson(t *testing.T) {
	msg := `{"details":"requesting higher privileges than access token allows"}`
	response := &http.Response{
		Status:     "403 Forbidden",
		StatusCode: 403,
		Body:       nopCloser{bytes.NewBufferString(msg)},
	}
	err := HandleHTTPResponseError(response)

	if !strings.Contains(err.Error(), msg) {
		t.Errorf("Expected %q, got: %q", msg, err.Error())
	}
}

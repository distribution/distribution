package test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
)

// RequestResponseMap is a mapping from Requests to Responses
type RequestResponseMap []RequestResponseMapping

// RequestResponseMapping defines an ordered list of Responses to be sent in
// response to a given Request
type RequestResponseMapping struct {
	Request   Request
	Responses []Response
}

// TODO(bbland): add support for request headers

// Request is a simplified http.Request object
type Request struct {
	// Method is the http method of the request, for example GET
	Method string

	// Route is the http route of this request
	Route string

	// Body is the byte contents of the http request
	Body []byte
}

func (r Request) String() string {
	return fmt.Sprintf("%s %s\n%s", r.Method, r.Route, r.Body)
}

// Response is a simplified http.Response object
type Response struct {
	// Statuscode is the http status code of the Response
	StatusCode int

	// Headers are the http headers of this Response
	Headers http.Header

	// Body is the response body
	Body []byte
}

// testHandler is an http.Handler with a defined mapping from Request to an
// ordered list of Response objects
type testHandler struct {
	responseMap map[string][]Response
}

// NewHandler returns a new test handler that responds to defined requests
// with specified responses
// Each time a Request is received, the next Response is returned in the
// mapping, until no Responses are defined, at which point a 404 is sent back
func NewHandler(requestResponseMap RequestResponseMap) http.Handler {
	responseMap := make(map[string][]Response)
	for _, mapping := range requestResponseMap {
		responseMap[mapping.Request.String()] = mapping.Responses
	}
	return &testHandler{responseMap: responseMap}
}

func (app *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	requestBody, _ := ioutil.ReadAll(r.Body)
	request := Request{
		Method: r.Method,
		Route:  r.URL.Path,
		Body:   requestBody,
	}

	responses, ok := app.responseMap[request.String()]

	if !ok || len(responses) == 0 {
		http.NotFound(w, r)
		return
	}

	response := responses[0]
	app.responseMap[request.String()] = responses[1:]

	responseHeader := w.Header()
	for k, v := range response.Headers {
		responseHeader[k] = v
	}

	w.WriteHeader(response.StatusCode)

	io.Copy(w, bytes.NewReader(response.Body))
}

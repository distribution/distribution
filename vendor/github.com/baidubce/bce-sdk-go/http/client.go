/*
 * Copyright 2017 Baidu, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file
 * except in compliance with the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the
 * License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions
 * and limitations under the License.
 */

// client.go - define the execute function to send http request and get response

// Package http defines the structure of request and response which used to access the BCE services
// as well as the http constant headers and and methods. And finally implement the `Execute` funct-
// ion to do the work.
package http

import (
	"net/http"
	"net/url"
	"time"
)

// The httpClient is the global variable to send the request and get response
// for reuse and the Client provided by the Go standard library is thread safe.
var httpClient = &http.Client{}

// Execute - do the http requset and get the response
//
// PARAMS:
//     - request: the http request instance to be sent
// RETURNS:
//     - response: the http response returned from the server
//     - error: nil if ok otherwise the specific error
func Execute(request *Request) (*Response, error) {
	// Build the request object for the current requesting
	httpRequest := &http.Request{}
	defaultTr := http.DefaultTransport
	tr, _ := defaultTr.(*http.Transport)

	// Set the connection timeout for current request
	httpClient.Timeout = time.Duration(request.Timeout()) * time.Second

	// Set the request method
	httpRequest.Method = request.Method()

	// Set the request url
	internalUrl := &url.URL{
		Scheme:   request.Protocol(),
		Host:     request.Host(),
		Path:     request.Uri(),
		RawQuery: request.QueryString()}
	httpRequest.URL = internalUrl

	// Set the request headers
	internalHeader := make(http.Header)
	for k, v := range request.Headers() {
		val := make([]string, 0, 1)
		val = append(val, v)
		internalHeader[k] = val
	}
	httpRequest.Header = internalHeader

	// Set the reqeust body and content length if needed
	// Variable body's type is `*BodyStream`. If its value is nil, the `Body` field must be
	// explicitly assigned `nil` value, otherwise nil pointer dereference will arise.
	body := request.Body()
	if body != nil {
		httpRequest.Body = body
		httpRequest.ContentLength = request.Length()
	}

	// Set the proxy setting if needed
	if len(request.ProxyUrl()) != 0 {
		tr.Proxy = func(_ *http.Request) (*url.URL, error) {
			return url.Parse(request.ProxyUrl())
		}
	}

	// Perform the http request and get response
	// It needs to explicitly close the keep-alive connections when error occurs for the request
	// that may continue sending request's data subsequently.
	httpClient.Transport = tr
	start := time.Now()
	httpResponse, err := httpClient.Do(httpRequest)
	end := time.Now()
	if err != nil {
		tr.CloseIdleConnections()
		return nil, err
	}
	if httpResponse.StatusCode >= 400 &&
		(httpRequest.Method == PUT || httpRequest.Method == POST) {
		tr.CloseIdleConnections()
	}
	response := &Response{httpResponse, end.Sub(start)}
	return response, nil
}

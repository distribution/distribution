/**
 * Copyright (c) 2015 Guoyao Wu, All Rights Reserved
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
 * the License. You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
 * an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 *
 * @file request.go
 * @author guoyao
 */

// Package bce define a set of core data structure and functions for baidubce.
package bce

import (
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/guoyao/baidubce-sdk-go/util"
)

var canonicalHeaders = []string{
	"host",
	"content-length",
	"content-type",
	"content-md5",
}

// Request is http request, but has some custom functions.
type Request http.Request

// NewRequest returns an instance of type `Request`
func NewRequest(method, url string, body io.Reader) (*Request, error) {
	method = strings.ToUpper(method)

	rawRequest, err := http.NewRequest(method, url, body)

	if file, ok := body.(*os.File); ok {
		fileInfo, err := file.Stat()

		if err != nil {
			return nil, err
		}

		rawRequest.ContentLength = fileInfo.Size()
	}

	req := (*Request)(rawRequest)

	return req, err
}

// Add headers to http request
func (req *Request) AddHeaders(headerMap map[string]string) {
	for key, value := range headerMap {
		req.addHeader(key, value)
	}
}

func (req *Request) addHeader(key, value string) {
	req.Header.Add(key, value)
}

// Set headers to http request
func (req *Request) SetHeaders(headerMap map[string]string) {
	for key, value := range headerMap {
		req.setHeader(key, value)
	}
}

func (req *Request) setHeader(key, value string) {
	req.Header.Set(key, value)
}

// clear all existed headers
func (req *Request) clearHeaders() {
	for key := range req.Header {
		delete(req.Header, key)
	}
}

func (req *Request) prepareHeaders(option *SignOption) {
	req.SetHeaders(option.Headers)

	if !util.MapContains(option.Headers, generateHeaderValidCompareFunc("host")) {
		option.Headers["host"] = req.URL.Host
		req.addHeader("Host", req.URL.Host)
	}

	host := req.Header.Get("Host")
	if host != req.URL.Host {
		req.setHeader("Host", req.URL.Host)
	}
}

func (req *Request) toCanonicalHeaderString(option *SignOption) string {
	headerMap := make(map[string]string, len(req.Header))
	for key, value := range req.Header {
		if !option.headersToSignSpecified {
			if isCanonicalHeader(key) {
				headerMap[key] = value[0]
			}
		} else if util.Contains(option.HeadersToSign, key, true) {
			headerMap[key] = value[0]
		}
	}

	result := util.ToCanonicalHeaderString(headerMap)
	return result
}

func (req *Request) canonical(option *SignOption) string {
	canonicalStrings := make([]string, 0, 4)

	canonicalStrings = append(canonicalStrings, req.Method)

	canonicalURI := util.URIEncodeExceptSlash(req.URL.Path)
	canonicalStrings = append(canonicalStrings, canonicalURI)

	canonicalStrings = append(canonicalStrings, req.URL.RawQuery)

	canonicalHeader := req.toCanonicalHeaderString(option)
	canonicalStrings = append(canonicalStrings, canonicalHeader)

	return strings.Join(canonicalStrings, "\n")
}

func (req *Request) raw() *http.Request {
	return (*http.Request)(req)
}

func isCanonicalHeader(key string) bool {
	key = strings.ToLower(key)
	return util.Contains(canonicalHeaders, key, true) || strings.Index(key, "x-bce-") == 0
}

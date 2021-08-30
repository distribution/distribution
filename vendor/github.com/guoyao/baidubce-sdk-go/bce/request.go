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

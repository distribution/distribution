package challenge

import (
	"net/url"
	"strings"
)

// FROM: https://golang.org/src/net/http/http.go
// Given a string of the form "host", "host:port", or "[ipv6::address]:port",
// return true if the string includes a port.
func hasPort(s string) bool { return strings.LastIndex(s, ":") > strings.LastIndex(s, "]") }

// FROM: http://golang.org/src/net/http/transport.go
var portMap = map[string]string{
	"http":  "80",
	"https": "443",
}

// canonicalAddr returns url.Host but always lower-cased with a ":port" suffix
// FROM: http://golang.org/src/net/http/transport.go
func canonicalAddr(url *url.URL) string {
	addr := strings.ToLower(url.Host)
	if !hasPort(addr) {
		return addr + ":" + portMap[url.Scheme]
	}
	return addr
}

// normalizedURL returns the endpoint URL in canonical form.
func normalizedURL(endpoint url.URL) string {
	endpoint.Host = canonicalAddr(&endpoint)
	return endpoint.String()
}

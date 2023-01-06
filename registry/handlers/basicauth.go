//go:build go1.4
// +build go1.4

package handlers

import (
	"net/http"
)

func basicAuth(r *http.Request) (username, password string, ok bool) {
	return r.BasicAuth()
}

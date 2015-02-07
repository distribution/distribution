// +build go1.4

package registry

import (
	"net/http"
)

func basicAuth(r *http.Request) (username, password string, ok bool) {
	return r.BasicAuth()
}

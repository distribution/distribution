package registry

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/felixge/httpsnoop"
)

// accessLogHandler logs requests to out in Apache Combined Log Format, like
// gorilla/handlers.CombinedLoggingHandler. Gorilla counts response bytes only
// in Write and logs zero bytes for responses copied through io.ReaderFrom,
// which net/http uses for sendfile. httpsnoop.Wrap preserves ReadFrom while
// the hooks count bytes and track the first committed response status.
func accessLogHandler(out io.Writer, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts, u := time.Now(), *r.URL
		status, written := http.StatusOK, int64(0)
		var wroteHeader bool
		w = httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					next(code)
					// A 1xx status is interim: net/http sends it and another status
					// follows. The exception is 101 Switching Protocols, after which
					// net/http sends no further status. So 101 becomes the logged
					// status, and other 1xx statuses do not.
					if !wroteHeader && (code >= 200 || code == http.StatusSwitchingProtocols) {
						status, wroteHeader = code, true
					}
				}
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(p []byte) (int, error) {
					n, err := next(p)
					written += int64(n)
					wroteHeader = true
					return n, err
				}
			},
			ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(src io.Reader) (int64, error) {
					n, err := next(src)
					written += n
					// Copying bytes implies HTTP 200 if no status was set earlier.
					// With no bytes copied, headers may still be unwritten, allowing
					// the handler to send 204 or an error with a later WriteHeader call.
					wroteHeader = wroteHeader || n > 0
					return n, err
				}
			},
		})
		h.ServeHTTP(w, r)
		_, _ = io.WriteString(out, combinedLogLine(r, u, ts, status, written))
	})
}

// combinedLogLine produces the same line as Gorilla's writeCombinedLog. It is
// adapted from writeCombinedLog and buildCommonLogLine in logging.go of
// github.com/gorilla/handlers, BSD 3-Clause license:
// Copyright (c) 2023 The Gorilla Authors. All rights reserved.
// The license conditions and the disclaimer are in
// vendor/github.com/gorilla/handlers/LICENSE.
func combinedLogLine(req *http.Request, u url.URL, ts time.Time, status int, size int64) string {
	username := "-"
	if u.User != nil {
		if name := u.User.Username(); name != "" {
			username = name
		}
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	uri := req.RequestURI
	// CONNECT over HTTP/2 identifies the target with the authority field.
	if req.ProtoMajor == 2 && req.Method == http.MethodConnect {
		uri = req.Host
	}
	if uri == "" {
		uri = u.RequestURI()
	}
	return host + " - " + username + " [" + ts.Format("02/Jan/2006:15:04:05 -0700") +
		`] "` + req.Method + " " + quoteLogField(uri) + " " + req.Proto + `" ` +
		strconv.Itoa(status) + " " + strconv.FormatInt(size, 10) +
		` "` + quoteLogField(req.Referer()) + `" "` + quoteLogField(req.UserAgent()) + "\"\n"
}

// quoteLogField escapes s like Gorilla's appendQuoted. That matches
// strconv.Quote without the outer quotes, except that Gorilla escapes DEL as
// \u007f where strconv.Quote uses \x7f.
func quoteLogField(s string) string {
	parts := strings.Split(s, "\x7f")
	for i, p := range parts {
		q := strconv.Quote(p)
		parts[i] = q[1 : len(q)-1]
	}
	return strings.Join(parts, `\u007f`)
}

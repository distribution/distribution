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
					if !wroteHeader && code >= 200 {
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
					// A zero-byte copy can leave the response uncommitted.
					// See https://github.com/felixge/httpsnoop/issues/42.
					wroteHeader = wroteHeader || n > 0
					return n, err
				}
			},
		})
		h.ServeHTTP(w, r)
		_, _ = out.Write(combinedLogLine(r, u, ts, status, written))
	})
}

// combinedLogLine produces the same line as Gorilla's writeCombinedLog.
func combinedLogLine(req *http.Request, u url.URL, ts time.Time, status int, size int64) []byte {
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
	return []byte(host + " - " + username + " [" + ts.Format("02/Jan/2006:15:04:05 -0700") +
		`] "` + req.Method + " " + quoteLogField(uri) + " " + req.Proto + `" ` +
		strconv.Itoa(status) + " " + strconv.FormatInt(size, 10) +
		` "` + quoteLogField(req.Referer()) + `" "` + quoteLogField(req.UserAgent()) + "\"\n")
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

package registry

import (
	"io"
	"net/http"
	"net/url"
	"time"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

// jsonLogEntry represents a log entry in JSON format.
type jsonLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Status    int       `json:"status"`
	Size      int       `json:"size"`
	Referer   string    `json:"referer"`
	UserAgent string    `json:"user_agent"`
}

// jsonLogFormatterParams holds the parameters required for JSON logging.
type jsonLogFormatterParams struct {
	Request   *http.Request
	URL       *url.URL
	Timestamp time.Time
	Status    int
	Size      int
}

type responseLogger struct {
	w      http.ResponseWriter
	status int
	size   int
}

func (l *responseLogger) Write(p []byte) (int, error) {
	size, err := l.w.Write(p)
	l.size += size
	return size, err
}

func (l *responseLogger) Status() int {
	if l.status == 0 {
		return http.StatusOK
	}
	return l.status
}

func (l *responseLogger) Size() int {
	return l.size
}

func makeLogger(w http.ResponseWriter) *responseLogger {
	return &responseLogger{w: w, status: http.StatusOK}
}

// writeJSONCombinedLog writes a log entry for req to w in JSON format similar to Combined Log Format.
func writeJSONCombinedLog(enc *jsoniter.Encoder, params jsonLogFormatterParams) {
	_ = enc.Encode(&jsonLogEntry{
		Timestamp: params.Timestamp.UTC(),
		Method:    params.Request.Method,
		Path:      params.URL.Path,
		Status:    params.Status,
		Size:      params.Size,
		Referer:   params.Request.Referer(),
		UserAgent: params.Request.UserAgent(),
	})
}

// JSONLoggingHandler returns a http.Handler that wraps h and logs requests in JSON
// format similar to Combined Log Format.
func JSONLoggingHandler(out io.Writer, h http.Handler) http.Handler {
	enc := json.NewEncoder(out)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		h.ServeHTTP(w, req)

		if req.MultipartForm != nil {
			if err := req.MultipartForm.RemoveAll(); err != nil {
				return
			}
		}

		logger := makeLogger(w)

		writeJSONCombinedLog(enc, jsonLogFormatterParams{
			Request:   req,
			URL:       req.URL,
			Timestamp: time.Now(),
			Status:    logger.Status(),
			Size:      logger.Size(),
		})
	})
}

package handlers

import (
	"encoding/json"
	"io"
	"net/http"
)

// serveJSON marshals v and sets the content-type header to
// 'application/json'. If a different status code is required, call
// ResponseWriter.WriteHeader before this function.
func serveJSON(w http.ResponseWriter, v interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)

	if err := enc.Encode(v); err != nil {
		return err
	}

	return nil
}

func serveJSONStatus(w http.ResponseWriter, v interface{}, status int) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if status > 0 {
		w.WriteHeader(status)
	}
	enc := json.NewEncoder(w)

	if err := enc.Encode(v); err != nil {
		return err
	}

	return nil
}

func serveError(err error, w http.ResponseWriter) error {
	switch err := err.(type) {
	case httpError:
		err.ServeError(w)
		return nil
	default:
		return serveJSON(w, err)

	}
}

// closeResources closes all the provided resources after running the target
// handler.
func closeResources(handler http.Handler, closers ...io.Closer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, closer := range closers {
			defer closer.Close()
		}
		handler.ServeHTTP(w, r)
	})
}

package registry

import (
	"encoding/json"
	"net/http"
)

// serveJSON marshals v and sets the content-type header to
// 'application/json'. If a different status code is required, call
// ResponseWriter.WriteHeader before this function.
func serveJSON(w http.ResponseWriter, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)

	if err := enc.Encode(v); err != nil {
		return err
	}

	return nil
}

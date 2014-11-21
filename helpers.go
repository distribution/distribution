package registry

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/mux"
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

// clondedRoute returns a clone of the named route from the router.
func clonedRoute(router *mux.Router, name string) *mux.Route {
	route := new(mux.Route)
	*route = *router.GetRoute(name) // clone the route
	return route
}

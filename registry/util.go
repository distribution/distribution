package registry

import (
	"net/http"
	"reflect"
	"runtime"

	"github.com/gorilla/handlers"
)

// functionName returns the name of the function fn.
func functionName(fn interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
}

// resolveHandlerName attempts to resolve a nice, pretty name for the passed
// in handler.
func resolveHandlerName(method string, handler http.Handler) string {
	switch v := handler.(type) {
	case handlers.MethodHandler:
		return functionName(v[method])
	case http.HandlerFunc:
		return functionName(v)
	default:
		return functionName(handler.ServeHTTP)
	}
}

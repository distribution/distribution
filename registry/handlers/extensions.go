package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/gorilla/handlers"
)

// extensionsDispatcher constructs the extensions handler api endpoint.
func extensionsDispatcher(ctx *Context, r *http.Request) http.Handler {
	extensionsHandler := &extensionsHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(extensionsHandler.GetExtensions),
	}
}

// extensionsHandler handles requests for lists of extensions under a repository name.
type extensionsHandler struct {
	*Context
}
type extensionResponse struct {
	Name string `json:"name,omitempty"`
}

type extensionsAPIResponse struct {
	Name       string              `json:"name,omitempty"`
	Extensions []extensionResponse `json:"extensions"`
}

// GetExtensions returns a json list of extensions for a specific repo name.
func (eh *extensionsHandler) GetExtensions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// TODO: pagination support.

	w.Header().Set("Content-Type", "application/json")

	extensionNames := eh.registryExtensions
	if eh.Repository != nil {
		extensionNames = eh.repositoryExtensions
	}

	var extensions []extensionResponse
	for _, ext := range extensionNames {
		extensions = append(extensions, extensionResponse{Name: ext})
	}

	var resp extensionsAPIResponse
	if eh.Repository != nil {
		resp = extensionsAPIResponse{
			Name:       eh.Repository.Named().Name(),
			Extensions: extensions,
		}
	} else {
		resp = extensionsAPIResponse{
			Extensions: extensions,
		}
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(resp); err != nil {
		eh.Errors = append(eh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

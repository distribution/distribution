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

type extensionsAPIResponse struct {
	Name       string   `json:"name,omitempty"`
	Extensions []string `json:"extensions"`
}

// GetExtensions returns a json list of extensions for a specific image name.
func (eh *extensionsHandler) GetExtensions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// TODO(shizh): pagination support.

	w.Header().Set("Content-Type", "application/json")

	var name string
	if eh.Repository != nil {
		name = eh.Repository.Named().Name()
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(extensionsAPIResponse{
		Name:       name,
		Extensions: eh.repositoryExtensions,
	}); err != nil {
		eh.Errors = append(eh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

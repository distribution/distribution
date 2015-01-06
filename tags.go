package registry

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution/api/v2"
	"github.com/docker/distribution/storage"
	"github.com/gorilla/handlers"
)

// tagsDispatcher constructs the tags handler api endpoint.
func tagsDispatcher(ctx *Context, r *http.Request) http.Handler {
	tagsHandler := &tagsHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(tagsHandler.GetTags),
	}
}

// tagsHandler handles requests for lists of tags under a repository name.
type tagsHandler struct {
	*Context
}

type tagsAPIResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// GetTags returns a json list of tags for a specific image name.
func (th *tagsHandler) GetTags(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	manifests := th.services.Manifests()

	tags, err := manifests.Tags(th.Name)
	if err != nil {
		switch err := err.(type) {
		case storage.ErrUnknownRepository:
			w.WriteHeader(404)
			th.Errors.Push(v2.ErrorCodeNameUnknown, map[string]string{"name": th.Name})
		default:
			th.Errors.PushErr(err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	enc := json.NewEncoder(w)
	if err := enc.Encode(tagsAPIResponse{
		Name: th.Name,
		Tags: tags,
	}); err != nil {
		th.Errors.PushErr(err)
		return
	}
}

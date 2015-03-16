package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution"
	"github.com/docker/distribution/registry/api/v2"
)

func tagsHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (httpErr error) {
	switch r.Method {
	case "GET":
		httpErr = GetTags(ctx, w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
	return httpErr
}

type tagsAPIResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// GetTags returns a json list of tags for a specific image name.
func GetTags(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	defer r.Body.Close()
	manifests := ctx.Repository.Manifests()

	tags, err := manifests.Tags()
	if err != nil {
		switch err := err.(type) {
		case distribution.ErrRepositoryUnknown:
			return NewHTTPError(v2.ErrorCodeNameUnknown, map[string]string{"name": ctx.Repository.Name()}, http.StatusNotFound)
		default:
			return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	// TODO (endophage): we might want to attempt the encoding first,
	// especially for something like this we expect to be small. That
	// was we can still serve a status header if an error occurs
	enc := json.NewEncoder(w)
	if err := enc.Encode(tagsAPIResponse{
		Name: ctx.Repository.Name(),
		Tags: tags,
	}); err != nil {
		return NewHTTPError(v2.ErrorCodeUnknown, err, http.StatusInternalServerError)
	}
	return nil
}

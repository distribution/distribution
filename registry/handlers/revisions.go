package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

// tagsDispatcher constructs the tags handler api endpoint.
func revisionsDispatcher(ctx *Context, r *http.Request) http.Handler {
	revisionsHandler := &revisionsHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(revisionsHandler.GetRevisions),
	}
}

// tagsHandler handles requests for lists of tags under a repository name.
type revisionsHandler struct {
	*Context
}

type revisionsAPIResponse struct {
	Name      string   `json:"name"`
	Revisions []string `json:"revisions"`
}

// GetTags returns a json list of tags for a specific image name.
func (th *revisionsHandler) GetRevisions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	named, err := reference.WithName(th.Repository.Named().Name())
	if err != nil {
		th.Errors = append(th.Errors, err)
	}

	repository, err := th.registry.Repository(th, named)
	if err != nil {
		th.Errors = append(th.Errors, err)
	}

	manifestService, err := repository.Manifests(th)
	if err != nil {
		th.Errors = append(th.Errors, err)
	}
	manifestEnumerator, ok := manifestService.(distribution.ManifestEnumerator)

	if !ok {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail("unable to convert ManifestService into ManifestEnumerator"))
	}
	revisions := []string{}
	err = manifestEnumerator.Enumerate(th, func(dgst digest.Digest) error {
		revisions = append(revisions, string(dgst))
		return nil
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if err := enc.Encode(revisionsAPIResponse{
		Name:      th.Repository.Named().Name(),
		Revisions: revisions,
	}); err != nil {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

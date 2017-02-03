package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/docker/distribution/registry/api/v2"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/gorilla/handlers"
)

func tagsDispatcher(ctx *Context, r *http.Request) http.Handler {
	tagsHandler := &tagsHandler{
		Context: ctx,
		Tag:     getTag(ctx),
	}

	thandler := handlers.MethodHandler{
		"GET": http.HandlerFunc(tagsHandler.GetTagsCompat),
	}

	if !ctx.readOnly {
		thandler["DELETE"] = http.HandlerFunc(tagsHandler.DeleteTag)
	}

	return thandler
}

type tagsHandler struct {
	*Context
	Tag string
}

// tagsDispatcher constructs the tags handler api endpoint.
func tagsListDispatcher(ctx *Context, r *http.Request) http.Handler {
	tagsListHandler := &tagsListHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(tagsListHandler.GetTags),
	}
}

// tagsHandler handles requests for lists of tags under a repository name.
type tagsListHandler struct {
	*Context
}

type tagsListAPIResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// GetTagsCompat handles only 'list' tag to stay compatible with old clients
func (th *tagsHandler) GetTagsCompat(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(th).Debug("GetTags")

	// We need to support old tags URL to be compatible with old clients
	if th.Tag == "list" {
		listURL, err := th.urlBuilder.BuildTagsListURL(th.Repository.Named())
		if err != nil {
			th.Errors = append(th.Errors, err)
			return
		}

		http.Redirect(w, r, listURL, http.StatusMovedPermanently)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (th *tagsHandler) DeleteTag(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(th).Debug("DeleteTag")

	if th.App.isCache {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnsupported)
		return
	}

	tagService := th.Repository.Tags(th)
	err := tagService.Untag(th.Context, th.Tag)
	if err != nil {
		switch err.(type) {
		case distribution.ErrTagUnknown:
		case storagedriver.PathNotFoundError:
			th.Errors = append(th.Errors, v2.ErrorCodeManifestUnknown)
			return
		default:
			th.Errors = append(th.Errors, errcode.ErrorCodeUnknown)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

// GetTags returns a json list of tags for a specific image name.
func (th *tagsListHandler) GetTags(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	tagService := th.Repository.Tags(th)
	tags, err := tagService.All(th)
	if err != nil {
		switch err := err.(type) {
		case distribution.ErrRepositoryUnknown:
			th.Errors = append(th.Errors, v2.ErrorCodeNameUnknown.WithDetail(map[string]string{"name": th.Repository.Named().Name()}))
		case errcode.Error:
			th.Errors = append(th.Errors, err)
		default:
			th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	enc := json.NewEncoder(w)
	if err := enc.Encode(tagsListAPIResponse{
		Name: th.Repository.Named().Name(),
		Tags: tags,
	}); err != nil {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

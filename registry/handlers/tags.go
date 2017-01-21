package handlers

import (
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
		Tag:     getReference(ctx),
	}

	thandler := handlers.MethodHandler{
		"GET": http.HandlerFunc(tagsHandler.GetTags),
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

func (th *tagsHandler) GetTags(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(th).Debug("GetTags")

	if th.Tag == "list" {
		listUrl, err := th.urlBuilder.BuildTagsListURL(th.Repository.Named())
		if err != nil {
			th.Errors = append(th.Errors, err)
			return
		}

		http.Redirect(w, r, listUrl, http.StatusMovedPermanently)
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

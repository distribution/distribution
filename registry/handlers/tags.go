package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/gorilla/handlers"
)

const defaultReturnedTags = 1000

// tagsDispatcher constructs the tags handler api endpoint.
func tagsDispatcher(ctx *Context, r *http.Request) http.Handler {
	tagsHandler := &tagsHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		http.MethodGet: http.HandlerFunc(tagsHandler.GetTags),
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
	var moreEntries = true

	q := r.URL.Query()
	lastEntry := q.Get("last")

	limit := -1

	// parse n, if n unparseable, or negative assign it to defaultReturnedEntries
	if n := q.Get("n"); n != "" {
		limit = defaultReturnedTags
		if th.App.Config.Tags.MaxTags < limit {
			limit = th.App.Config.Tags.MaxTags
		}
		parsedMax, err := strconv.Atoi(n)
		if err != nil || parsedMax > limit || parsedMax < 0 {
			th.Errors = append(th.Errors, errcode.ErrorCodePaginationNumberInvalid.WithDetail(map[string]int{"n": parsedMax}))
			return

		}
		limit = parsedMax
	}

	filled := make([]string, 0)

	if limit == 0 {
		moreEntries = false
	} else {
		tagService := th.Repository.Tags(th)
		// if limit is -1, we want to list all the tags, and receive a io.EOF error
		returnedTags, err := tagService.List(th.Context, limit, lastEntry)
		if err != nil {
			if err != io.EOF {
				switch err := err.(type) {
				case distribution.ErrRepositoryUnknown:
					th.Errors = append(th.Errors, errcode.ErrorCodeNameUnknown.WithDetail(map[string]string{"name": th.Repository.Named().Name()}))
				case errcode.Error:
					th.Errors = append(th.Errors, err)
				default:
					th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
				}
				return
			}
			// err is either io.EOF
			moreEntries = false
		}
		filled = returnedTags
	}

	w.Header().Set("Content-Type", "application/json")

	// Add a link header if there are more entries to retrieve
	if moreEntries {
		lastEntry = filled[len(filled)-1]
		urlStr, err := createLinkEntry(r.URL.String(), limit, lastEntry)
		if err != nil {
			th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			return
		}
		w.Header().Set("Link", urlStr)
	}

	enc := json.NewEncoder(w)
	if err := enc.Encode(tagsAPIResponse{
		Name: th.Repository.Named().Name(),
		Tags: filled,
	}); err != nil {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

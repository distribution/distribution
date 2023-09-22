package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/gorilla/handlers"
)

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
	tagService := th.Repository.Tags(th)
	tags, err := tagService.All(th)
	if err != nil {
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

	// do pagination if requested
	q := r.URL.Query()
	// get entries after latest, if any specified
	if lastEntry := q.Get("last"); lastEntry != "" {
		lastEntryIndex := sort.SearchStrings(tags, lastEntry)

		// as`sort.SearchStrings` can return len(tags), if the
		// specified `lastEntry` is not found, we need to
		// ensure it does not panic when slicing.
		if lastEntryIndex == len(tags) {
			tags = []string{}
		} else {
			tags = tags[lastEntryIndex+1:]
		}
	}

	// if no error, means that the user requested `n` entries
	if n := q.Get("n"); n != "" {
		maxEntries, err := strconv.Atoi(n)
		if err != nil || maxEntries < 0 {
			th.Errors = append(th.Errors, errcode.ErrorCodePaginationNumberInvalid.WithDetail(map[string]string{"n": n}))
			return
		}

		// if there is requested more than or
		// equal to the amount of tags we have,
		// then set the request to equal `len(tags)`.
		// the reason for the `=`, is so the else
		// clause will only activate if there
		// are tags left the user needs.
		if maxEntries >= len(tags) {
			maxEntries = len(tags)
		} else if maxEntries > 0 {
			// defined in `catalog.go`
			urlStr, err := createLinkEntry(r.URL.String(), maxEntries, tags[maxEntries-1])
			if err != nil {
				th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
				return
			}
			w.Header().Set("Link", urlStr)
		}

		tags = tags[:maxEntries]
	}

	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if err := enc.Encode(tagsAPIResponse{
		Name: th.Repository.Named().Name(),
		Tags: tags,
	}); err != nil {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

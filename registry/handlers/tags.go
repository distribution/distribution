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
	q := r.URL.Query()
	tagService := th.Repository.Tags(th)

	switch q.Get("sort") {
	case "":
		// Default path: use List() for efficient storage-level pagination.
	case "updated":
		if ts, ok := tagService.(distribution.TagServiceWithTimestamp); ok {
			th.getTagsSortedByTime(w, r, ts)
			return
		}
		// Fall through to default path if interface is not supported.
	default:
		th.Errors = append(th.Errors, errcode.ErrorCodeQueryParameterInvalid.WithDetail(
			map[string]string{"sort": q.Get("sort")},
		))
		return
	}

	th.getTagsDefault(w, r, tagService)
}

// getTagsSortedByTime handles the sort=updated case using AllWithModifiedTime.
func (th *tagsHandler) getTagsSortedByTime(w http.ResponseWriter, r *http.Request, ts distribution.TagServiceWithTimestamp) {
	q := r.URL.Query()

	tagDescs, err := ts.AllWithModifiedTime(th)
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

	tags := make([]string, len(tagDescs))
	for i, d := range tagDescs {
		tags[i] = d.Name
	}

	// Apply last-based pagination via linear search (list is time-ordered, not alphabetical).
	if lastEntry := q.Get("last"); lastEntry != "" {
		idx := -1
		for i, t := range tags {
			if t == lastEntry {
				idx = i
				break
			}
		}
		if idx == -1 || idx == len(tags)-1 {
			tags = []string{}
		} else {
			tags = tags[idx+1:]
		}
	}

	// Apply n limit and set Link header if there are more entries.
	if n := q.Get("n"); n != "" {
		maxEntries, err := strconv.Atoi(n)
		if err != nil || maxEntries < 0 {
			th.Errors = append(th.Errors, errcode.ErrorCodePaginationNumberInvalid.WithDetail(map[string]string{"n": n}))
			return
		}

		// Per the OCI distribution-spec, a server MAY return fewer than n
		// results when a Link header is provided for continuation. Clamp to
		// MaxTags instead of rejecting oversized requests.
		if maxTags := th.App.Config.Tags.MaxTags; maxTags > 0 && maxEntries > maxTags {
			maxEntries = maxTags
		}

		if maxEntries >= len(tags) {
			maxEntries = len(tags)
		} else if maxEntries > 0 {
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

// getTagsDefault handles the default (unsorted) case using List() for efficient pagination.
func (th *tagsHandler) getTagsDefault(w http.ResponseWriter, r *http.Request, tagService distribution.TagService) {
	q := r.URL.Query()

	var moreEntries = true

	lastEntry := q.Get("last")
	limit := -1

	if n := q.Get("n"); n != "" {
		parsedMax, err := strconv.Atoi(n)
		if err != nil || parsedMax < 0 {
			th.Errors = append(th.Errors, errcode.ErrorCodePaginationNumberInvalid.WithDetail(map[string]int{"n": parsedMax}))
			return
		}
		limit = parsedMax
		// Per the OCI distribution-spec, a server MAY return fewer than n
		// results when a Link header is provided for continuation. Clamp to
		// MaxTags instead of rejecting oversized requests.
		if maxTags := th.App.Config.Tags.MaxTags; maxTags > 0 && limit > maxTags {
			limit = maxTags
		}
	}

	filled := make([]string, 0)

	if limit == 0 {
		moreEntries = false
	} else {
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

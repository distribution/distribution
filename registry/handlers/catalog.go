package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/gorilla/handlers"
)

const defaultReturnedEntries = 100

func catalogDispatcher(ctx *Context, r *http.Request) http.Handler {
	catalogHandler := &catalogHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		http.MethodGet: http.HandlerFunc(catalogHandler.GetCatalog),
	}
}

type catalogHandler struct {
	*Context
}

type catalogAPIResponse struct {
	Repositories []string `json:"repositories"`
}

func (ch *catalogHandler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	moreEntries := true

	q := r.URL.Query()
	lastEntry := q.Get("last")

	entries := defaultReturnedEntries
	maximumConfiguredEntries := ch.App.Config.Catalog.MaxEntries

	// parse n, if n is negative abort with an error
	if n := q.Get("n"); n != "" {
		parsedMax, err := strconv.Atoi(n)
		if err != nil || parsedMax < 0 {
			ch.Errors = append(ch.Errors, errcode.ErrorCodePaginationNumberInvalid.WithDetail(map[string]string{"n": n}))
			return
		}

		// if a client requests more than it's allowed to receive
		if parsedMax > maximumConfiguredEntries {
			ch.Errors = append(ch.Errors, errcode.ErrorCodePaginationNumberInvalid.WithDetail(map[string]int{"n": parsedMax}))
			return
		}
		entries = parsedMax
	}

	// then enforce entries to be between 0 & maximumConfiguredEntries
	// max(0, min(entries, maximumConfiguredEntries))
	if entries < 0 || entries > maximumConfiguredEntries {
		entries = maximumConfiguredEntries
	}

	repos := make([]string, entries)
	filled := 0

	// entries is guaranteed to be >= 0 and < maximumConfiguredEntries
	if entries == 0 {
		moreEntries = false
	} else {
		returnedRepositories, err := ch.App.registry.Repositories(ch.Context, repos, lastEntry)
		if err != nil {
			_, pathNotFound := err.(driver.PathNotFoundError)
			if err != io.EOF && !pathNotFound {
				ch.Errors = append(ch.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
				return
			}
			// err is either io.EOF or not PathNotFoundError
			moreEntries = false
		}
		filled = returnedRepositories
	}

	w.Header().Set("Content-Type", "application/json")

	// Add a link header if there are more entries to retrieve
	if moreEntries {
		lastEntry = repos[filled-1]
		urlStr, err := createLinkEntry(r.URL.String(), entries, lastEntry)
		if err != nil {
			ch.Errors = append(ch.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			return
		}
		w.Header().Set("Link", urlStr)
	}

	enc := json.NewEncoder(w)
	if err := enc.Encode(catalogAPIResponse{
		Repositories: repos[0:filled],
	}); err != nil {
		ch.Errors = append(ch.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

// Use the original URL from the request to create a new URL for
// the link header
func createLinkEntry(origURL string, maxEntries int, lastEntry string) (string, error) {
	calledURL, err := url.Parse(origURL)
	if err != nil {
		return "", err
	}

	v := url.Values{}
	v.Add("n", strconv.Itoa(maxEntries))
	v.Add("last", lastEntry)

	calledURL.RawQuery = v.Encode()

	calledURL.Fragment = ""
	urlStr := fmt.Sprintf("<%s>; rel=\"next\"", calledURL.String())

	return urlStr, nil
}

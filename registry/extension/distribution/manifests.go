package distribution

import (
	"encoding/json"
	"net/http"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/extension"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
)

func manifestDispatcher(ctx *extension.Context, r *http.Request) http.Handler {
	manifestHandler := &manifestHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(manifestHandler.GetManifestDigests),
	}
}

// manifestHandler handles requests for manifests under a manifest name.
type manifestHandler struct {
	*extension.Context
}

type manifestAPIResponse struct {
	Name    string          `json:"name"`
	Tag     string          `json:"tag"`
	Digests []digest.Digest `json:"digests"`
}

func (mh *manifestHandler) GetManifestDigests(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	q := r.URL.Query()
	tag := q.Get("tag")
	if tag == "" {
		mh.Errors = append(mh.Errors, v2.ErrorCodeTagInvalid.WithDetail(tag))
		return
	}

	tags, ok := mh.Repository.Tags(mh.Context).(distribution.TagManifestsProvider)
	if !ok {
		mh.Errors = append(mh.Errors, errcode.ErrorCodeUnsupported.WithDetail(nil))
		return
	}

	digests, err := tags.ManifestDigests(mh.Context, tag)
	if err != nil {
		mh.Errors = append(mh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if err := enc.Encode(manifestAPIResponse{
		Name:    mh.Repository.Named().Name(),
		Tag:     tag,
		Digests: digests,
	}); err != nil {
		mh.Errors = append(mh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

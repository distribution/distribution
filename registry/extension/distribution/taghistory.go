package distribution

import (
	"encoding/json"
	"net/http"

	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/extension"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/opencontainers/go-digest"
)

type tagHistoryAPIResponse struct {
	Name    string          `json:"name"`
	Tag     string          `json:"tag"`
	Digests []digest.Digest `json:"digests"`
}

// manifestHandler handles requests for manifests under a manifest name.
type tagHistoryHandler struct {
	*extension.Context
	storageDriver driver.StorageDriver
}

func (th *tagHistoryHandler) getTagManifestDigests(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	q := r.URL.Query()
	tag := q.Get("tag")
	if tag == "" {
		th.Errors = append(th.Errors, v2.ErrorCodeTagInvalid.WithDetail(tag))
		return
	}

	digests, err := th.manifestDigests(tag)
	if err != nil {
		switch err := err.(type) {
		case driver.PathNotFoundError:
			th.Errors = append(th.Errors, v2.ErrorCodeManifestUnknown.WithDetail(map[string]string{"tag": tag}))
		default:
			th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if err := enc.Encode(tagHistoryAPIResponse{
		Name:    th.Repository.Named().Name(),
		Tag:     tag,
		Digests: digests,
	}); err != nil {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

func (th *tagHistoryHandler) manifestDigests(tag string) ([]digest.Digest, error) {
	tagLinkStore := storage.GetTagLinkReadOnlyBlobStore(
		th.Context,
		th.Repository,
		th.storageDriver,
		tag,
	)

	var dgsts []digest.Digest
	err := tagLinkStore.Enumerate(th.Context, func(dgst digest.Digest) error {
		dgsts = append(dgsts, dgst)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dgsts, nil
}

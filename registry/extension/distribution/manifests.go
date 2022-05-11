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

type manifestsGetAPIResponse struct {
	Name    string          `json:"name"`
	Digests []digest.Digest `json:"digests"`
}

// manifestHandler handles requests for manifests under a manifest name.
type manifestHandler struct {
	*extension.Context
	storageDriver driver.StorageDriver
}

func (th *manifestHandler) getManifests(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	digests, err := th.manifests()
	if err != nil {
		switch err := err.(type) {
		case driver.PathNotFoundError:
			th.Errors = append(th.Errors, v2.ErrorCodeNameUnknown.WithDetail(map[string]string{"name": th.Repository.Named().Name()}))
		default:
			th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")

	enc := json.NewEncoder(w)
	if err := enc.Encode(manifestsGetAPIResponse{
		Name:    th.Repository.Named().Name(),
		Digests: digests,
	}); err != nil {
		th.Errors = append(th.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

func (th *manifestHandler) manifests() ([]digest.Digest, error) {
	manifestLinkStore := storage.GetManifestLinkReadOnlyBlobStore(
		th.Context,
		th.Repository,
		th.storageDriver,
		nil,
	)

	var dgsts []digest.Digest
	err := manifestLinkStore.Enumerate(th.Context, func(dgst digest.Digest) error {
		dgsts = append(dgsts, dgst)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dgsts, nil
}

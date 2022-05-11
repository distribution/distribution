package oci

import (
	"encoding/json"
	"net/http"

	"github.com/distribution/distribution/v3"
	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// referrersResponse describes the response body of the referrers API.
//sajayantony - use the index type here.
// type referrersResponse struct {
// 	Referrers []v1.Descriptor `json:"manifests"`
// }

func (h *referrersHandler) getReferrers(w http.ResponseWriter, r *http.Request) {
	dcontext.GetLogger(h.extContext).Debug("Get")

	// This can be empty
	artifactType := r.FormValue("artifactType")

	if h.Digest == "" {
		h.extContext.Errors = append(h.extContext.Errors, v2.ErrorCodeManifestUnknown.WithDetail("digest not specified"))
		return
	}

	referrers, err := h.Referrers(h.extContext, h.Digest, artifactType)
	if err != nil {
		if _, ok := err.(distribution.ErrManifestUnknownRevision); ok {
			h.extContext.Errors = append(h.extContext.Errors, v2.ErrorCodeManifestUnknown.WithDetail(err))
		} else {
			h.extContext.Errors = append(h.extContext.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	if referrers == nil {
		referrers = []v1.Descriptor{}
	}

	// response := referrersResponse{
	// 	Referrers: referrers,
	// }

	response := v1.Index{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		Manifests:   referrers,
		Annotations: map[string]string{},
	}

	w.Header().Set("Content-Type", "application/json")
	//w.Header().Set("OCI-Api-Version", "oci/2.0")
	enc := json.NewEncoder(w)
	if err = enc.Encode(response); err != nil {
		h.extContext.Errors = append(h.extContext.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}
}

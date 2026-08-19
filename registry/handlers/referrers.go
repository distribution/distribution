package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// referrersDispatcher constructs the referrers handler.
func referrersDispatcher(ctx *Context, r *http.Request) http.Handler {
	referrersHandler := &referrersHandler{
		Context: ctx,
	}

	return handlers.MethodHandler{
		http.MethodGet: http.HandlerFunc(referrersHandler.GetReferrers),
	}
}

// referrersHandler handles requests for the referrers of a manifest digest.
type referrersHandler struct {
	*Context
}

// GetReferrers returns an OCI image index listing manifests that reference
// the given subject digest. Supports optional ?artifactType= filtering per
// the OCI Distribution Spec v1.1.
func (rh *referrersHandler) GetReferrers(w http.ResponseWriter, r *http.Request) {
	dgst, err := getDigest(rh)
	if err != nil {
		rh.Errors = append(rh.Errors, errcode.ErrorCodeDigestInvalid.WithDetail(err))
		return
	}

	// Get the artifact type filter if provided.
	artifactType := r.URL.Query().Get("artifactType")

	// Build a ReferenceService from the storage driver.
	refService := storage.NewReferenceEnumerator(rh.App.driver, rh.Repository)

	descriptors, err := refService.Referrers(rh, dgst, artifactType)
	if err != nil {
		switch err := err.(type) {
		case distribution.ErrRepositoryUnknown:
			rh.Errors = append(rh.Errors, errcode.ErrorCodeNameUnknown.WithDetail(map[string]string{"name": rh.Repository.Named().Name()}))
		case errcode.Error:
			rh.Errors = append(rh.Errors, err)
		default:
			rh.Errors = append(rh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	// Ensure we return an empty array, not null.
	if descriptors == nil {
		descriptors = []v1.Descriptor{}
	}

	// Build the OCI Image Index response per the spec.
	index := v1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: v1.MediaTypeImageIndex,
		Manifests: descriptors,
	}

	w.Header().Set("Content-Type", v1.MediaTypeImageIndex)

	// Per OCI spec, include the applied filter in the response header.
	if artifactType != "" {
		w.Header().Set("OCI-Filters-Applied", "artifactType")
	}

	if err := json.NewEncoder(w).Encode(index); err != nil {
		rh.Errors = append(rh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
	}
}

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/gorilla/handlers"
	"golang.org/x/net/context"
)

// imageManifestDispatcher takes the request context and builds the
// appropriate handler for handling image manifest requests.
func imageManifestDispatcher(ctx *Context, r *http.Request) http.Handler {
	imageManifestHandler := &imageManifestHandler{
		Context: ctx,
	}
	reference := getReference(ctx)
	dgst, err := digest.ParseDigest(reference)
	if err != nil {
		// We just have a tag
		imageManifestHandler.Tag = reference
	} else {
		imageManifestHandler.Digest = dgst
	}

	return handlers.MethodHandler{
		"GET":    http.HandlerFunc(imageManifestHandler.GetImageManifest),
		"PUT":    http.HandlerFunc(imageManifestHandler.PutImageManifest),
		"DELETE": http.HandlerFunc(imageManifestHandler.DeleteImageManifest),
	}
}

// imageManifestHandler handles http operations on image manifests.
type imageManifestHandler struct {
	*Context

	// One of tag or digest gets set, depending on what is present in context.
	Tag    string
	Digest digest.Digest
}

// GetImageManifest fetches the image manifest from the storage backend, if it exists.
func (imh *imageManifestHandler) GetImageManifest(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(imh).Debug("GetImageManifest")
	manifests := imh.Repository.Manifests()

	var (
		sm  *manifest.SignedManifest
		err error
	)

	if imh.Tag != "" {
		sm, err = manifests.GetByTag(imh.Tag)
	} else {
		sm, err = manifests.Get(imh.Digest)
	}

	if err != nil {
		imh.Errors.Push(v2.ErrorCodeManifestUnknown, err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the digest, if we don't already have it.
	if imh.Digest == "" {
		dgst, err := digestManifest(imh, sm)
		if err != nil {
			imh.Errors.Push(v2.ErrorCodeDigestInvalid, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		imh.Digest = dgst
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprint(len(sm.Raw)))
	w.Header().Set("Docker-Content-Digest", imh.Digest.String())
	w.Write(sm.Raw)
}

// PutImageManifest validates and stores and image in the registry.
func (imh *imageManifestHandler) PutImageManifest(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(imh).Debug("PutImageManifest")
	manifests := imh.Repository.Manifests()
	dec := json.NewDecoder(r.Body)

	var manifest manifest.SignedManifest
	if err := dec.Decode(&manifest); err != nil {
		imh.Errors.Push(v2.ErrorCodeManifestInvalid, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	dgst, err := digestManifest(imh, &manifest)
	if err != nil {
		imh.Errors.Push(v2.ErrorCodeDigestInvalid, err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Validate manifest tag or digest matches payload
	if imh.Tag != "" {
		if manifest.Tag != imh.Tag {
			ctxu.GetLogger(imh).Errorf("invalid tag on manifest payload: %q != %q", manifest.Tag, imh.Tag)
			imh.Errors.Push(v2.ErrorCodeTagInvalid)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		imh.Digest = dgst
	} else if imh.Digest != "" {
		if dgst != imh.Digest {
			ctxu.GetLogger(imh).Errorf("payload digest does match: %q != %q", dgst, imh.Digest)
			imh.Errors.Push(v2.ErrorCodeDigestInvalid)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	} else {
		imh.Errors.Push(v2.ErrorCodeTagInvalid, "no tag or digest specified")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if err := manifests.Put(&manifest); err != nil {
		// TODO(stevvooe): These error handling switches really need to be
		// handled by an app global mapper.
		switch err := err.(type) {
		case distribution.ErrManifestVerification:
			for _, verificationError := range err {
				switch verificationError := verificationError.(type) {
				case distribution.ErrUnknownLayer:
					imh.Errors.Push(v2.ErrorCodeBlobUnknown, verificationError.FSLayer)
				case distribution.ErrManifestUnverified:
					imh.Errors.Push(v2.ErrorCodeManifestUnverified)
				default:
					if verificationError == digest.ErrDigestInvalidFormat {
						// TODO(stevvooe): We need to really need to move all
						// errors to types. Its much more straightforward.
						imh.Errors.Push(v2.ErrorCodeDigestInvalid)
					} else {
						imh.Errors.PushErr(verificationError)
					}
				}
			}
		default:
			imh.Errors.PushErr(err)
		}

		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Construct a canonical url for the uploaded manifest.
	location, err := imh.urlBuilder.BuildManifestURL(imh.Repository.Name(), imh.Digest.String())
	if err != nil {
		// NOTE(stevvooe): Given the behavior above, this absurdly unlikely to
		// happen. We'll log the error here but proceed as if it worked. Worst
		// case, we set an empty location header.
		ctxu.GetLogger(imh).Errorf("error building manifest url from digest: %v", err)
	}

	w.Header().Set("Location", location)
	w.Header().Set("Docker-Content-Digest", imh.Digest.String())
	w.WriteHeader(http.StatusAccepted)
}

// DeleteImageManifest removes the image with the given tag from the registry.
func (imh *imageManifestHandler) DeleteImageManifest(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(imh).Debug("DeleteImageManifest")

	// TODO(stevvooe): Unfortunately, at this point, manifest deletes are
	// unsupported. There are issues with schema version 1 that make removing
	// tag index entries a serious problem in eventually consistent storage.
	// Once we work out schema version 2, the full deletion system will be
	// worked out and we can add support back.
	imh.Errors.Push(v2.ErrorCodeUnsupported)
	w.WriteHeader(http.StatusBadRequest)
}

// digestManifest takes a digest of the given manifest. This belongs somewhere
// better but we'll wait for a refactoring cycle to find that real somewhere.
func digestManifest(ctx context.Context, sm *manifest.SignedManifest) (digest.Digest, error) {
	p, err := sm.Payload()
	if err != nil {
		if !strings.Contains(err.Error(), "missing signature key") {
			ctxu.GetLogger(ctx).Errorf("error getting manifest payload: %v", err)
			return "", err
		}

		// NOTE(stevvooe): There are no signatures but we still have a
		// payload. The request will fail later but this is not the
		// responsibility of this part of the code.
		p = sm.Raw
	}

	dgst, err := digest.FromBytes(p)
	if err != nil {
		ctxu.GetLogger(ctx).Errorf("error digesting manifest: %v", err)
		return "", err
	}

	return dgst, err
}

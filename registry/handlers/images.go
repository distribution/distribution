package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/registry/api/errcode"
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

	mhandler := handlers.MethodHandler{
		"GET": http.HandlerFunc(imageManifestHandler.GetImageManifest),
	}

	if !ctx.readOnly {
		mhandler["PUT"] = http.HandlerFunc(imageManifestHandler.PutImageManifest)
		mhandler["DELETE"] = http.HandlerFunc(imageManifestHandler.DeleteImageManifest)
	}

	return mhandler
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
	manifests, err := imh.Repository.Manifests(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	var sm *schema1.SignedManifest
	if imh.Tag != "" {
		sm, err = manifests.GetByTag(imh.Tag)
	} else {
		if etagMatch(r, imh.Digest.String()) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		sm, err = manifests.Get(imh.Digest)
	}

	if err != nil {
		imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnknown.WithDetail(err))
		return
	}

	// Get the digest, if we don't already have it.
	if imh.Digest == "" {
		dgst, err := digestManifest(imh, sm)
		if err != nil {
			imh.Errors = append(imh.Errors, v2.ErrorCodeDigestInvalid.WithDetail(err))
			return
		}
		if etagMatch(r, dgst.String()) {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		imh.Digest = dgst
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprint(len(sm.Raw)))
	w.Header().Set("Docker-Content-Digest", imh.Digest.String())
	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, imh.Digest))
	w.Write(sm.Raw)
}

func etagMatch(r *http.Request, etag string) bool {
	for _, headerVal := range r.Header["If-None-Match"] {
		if headerVal == etag || headerVal == fmt.Sprintf(`"%s"`, etag) { // allow quoted or unquoted
			return true
		}
	}
	return false
}

// PutImageManifest validates and stores and image in the registry.
func (imh *imageManifestHandler) PutImageManifest(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(imh).Debug("PutImageManifest")
	manifests, err := imh.Repository.Manifests(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	var jsonBuf bytes.Buffer
	if err := copyFullPayload(w, r, &jsonBuf, imh, "image manifest PUT", &imh.Errors); err != nil {
		// copyFullPayload reports the error if necessary
		return
	}

	var manifest schema1.SignedManifest
	if err := json.Unmarshal(jsonBuf.Bytes(), &manifest); err != nil {
		imh.Errors = append(imh.Errors, v2.ErrorCodeManifestInvalid.WithDetail(err))
		return
	}

	dgst, err := digestManifest(imh, &manifest)
	if err != nil {
		imh.Errors = append(imh.Errors, v2.ErrorCodeDigestInvalid.WithDetail(err))
		return
	}

	// Validate manifest tag or digest matches payload
	if imh.Tag != "" {
		if manifest.Tag != imh.Tag {
			ctxu.GetLogger(imh).Errorf("invalid tag on manifest payload: %q != %q", manifest.Tag, imh.Tag)
			imh.Errors = append(imh.Errors, v2.ErrorCodeTagInvalid)
			return
		}

		imh.Digest = dgst
	} else if imh.Digest != "" {
		if dgst != imh.Digest {
			ctxu.GetLogger(imh).Errorf("payload digest does match: %q != %q", dgst, imh.Digest)
			imh.Errors = append(imh.Errors, v2.ErrorCodeDigestInvalid)
			return
		}
	} else {
		imh.Errors = append(imh.Errors, v2.ErrorCodeTagInvalid.WithDetail("no tag or digest specified"))
		return
	}

	if err := manifests.Put(&manifest); err != nil {
		// TODO(stevvooe): These error handling switches really need to be
		// handled by an app global mapper.
		if err == distribution.ErrUnsupported {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnsupported)
			return
		}
		switch err := err.(type) {
		case distribution.ErrManifestVerification:
			for _, verificationError := range err {
				switch verificationError := verificationError.(type) {
				case distribution.ErrManifestBlobUnknown:
					imh.Errors = append(imh.Errors, v2.ErrorCodeManifestBlobUnknown.WithDetail(verificationError.Digest))
				case distribution.ErrManifestUnverified:
					imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnverified)
				default:
					if verificationError == digest.ErrDigestInvalidFormat {
						imh.Errors = append(imh.Errors, v2.ErrorCodeDigestInvalid)
					} else {
						imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown, verificationError)
					}
				}
			}
		default:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}

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
	w.WriteHeader(http.StatusCreated)
}

// DeleteImageManifest removes the manifest with the given digest from the registry.
func (imh *imageManifestHandler) DeleteImageManifest(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(imh).Debug("DeleteImageManifest")

	manifests, err := imh.Repository.Manifests(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	err = manifests.Delete(imh.Digest)
	if err != nil {
		switch err {
		case digest.ErrDigestUnsupported:
		case digest.ErrDigestInvalidFormat:
			imh.Errors = append(imh.Errors, v2.ErrorCodeDigestInvalid)
			return
		case distribution.ErrBlobUnknown:
			imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnknown)
			return
		case distribution.ErrUnsupported:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnsupported)
			return
		default:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

// digestManifest takes a digest of the given manifest. This belongs somewhere
// better but we'll wait for a refactoring cycle to find that real somewhere.
func digestManifest(ctx context.Context, sm *schema1.SignedManifest) (digest.Digest, error) {
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

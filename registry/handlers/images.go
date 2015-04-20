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
	"golang.org/x/net/context"
)

// imageManifestDispatcher takes the request context and builds the
// appropriate handler for handling image manifest requests.
func imageManifestHandler(ctx *Context, w http.ResponseWriter, r *http.Request) (httpErr error) {
	switch r.Method {
	case "GET":
		httpErr = GetImageManifest(ctx, w, r)
	case "PUT":
		httpErr = PutImageManifest(ctx, w, r)
	case "DELETE":
		httpErr = DeleteImageManifest(ctx, w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
	return httpErr
}

func parseTagDigest(ctx *Context, r *http.Request) (string, digest.Digest) {
	tag := getReference(ctx)
	dgst, err := digest.ParseDigest(tag)
	if err != nil {
		return tag, ""
	}
	return "", dgst
}

// GetImageManifest fetches the image manifest from the storage backend, if it exists.
func GetImageManifest(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	ctxu.GetLogger(ctx).Debug("GetImageManifest")
	tag, dgst := parseTagDigest(ctx, r)
	manifests := ctx.Repository.Manifests()

	var (
		sm  *manifest.SignedManifest
		err error
	)

	if tag != "" {
		sm, err = manifests.GetByTag(tag)
	} else {
		sm, err = manifests.Get(dgst)
	}

	if err != nil {
		return NewHTTPError(v2.ErrorCodeManifestUnknown, err, http.StatusNotFound)
	}

	// Get the digest, if we don't already have it.
	if dgst == "" {
		dgst, err = digestManifest(ctx, sm)
		if err != nil {
			return NewHTTPError(v2.ErrorCodeDigestInvalid, err, http.StatusBadRequest)
		}

	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprint(len(sm.Raw)))
	w.Header().Set("Docker-Content-Digest", dgst.String())
	w.Write(sm.Raw)
	return nil
}

// PutImageManifest validates and stores and image in the registry.
func PutImageManifest(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	ctxu.GetLogger(ctx).Debug("PutImageManifest")
	tag, sentDgst := parseTagDigest(ctx, r)
	manifests := ctx.Repository.Manifests()
	dec := json.NewDecoder(r.Body)

	var manifest manifest.SignedManifest
	if err := dec.Decode(&manifest); err != nil {
		return NewHTTPError(v2.ErrorCodeManifestInvalid, err, http.StatusBadRequest)
	}

	dgst, err := digestManifest(ctx, &manifest)
	if err != nil {
		return NewHTTPError(v2.ErrorCodeDigestInvalid, err, http.StatusBadRequest)
	}

	// Validate manifest tag or digest matches payload
	if tag != "" {
		if manifest.Tag != tag {
			ctxu.GetLogger(ctx).Errorf("invalid tag on manifest payload: %q != %q", manifest.Tag, tag)
			return NewHTTPError(v2.ErrorCodeTagInvalid, nil, http.StatusBadRequest)
		}

	} else if sentDgst != "" {
		if dgst.String() != sentDgst.String() {
			ctxu.GetLogger(ctx).Errorf("payload digest does match: %q != %q", dgst, dgst)
			return NewHTTPError(v2.ErrorCodeDigestInvalid, nil, http.StatusBadRequest)
		}
	} else {
		return NewHTTPError(v2.ErrorCodeTagInvalid, "no tag or digest specified", http.StatusBadRequest)
	}

	if err := manifests.Put(&manifest); err != nil {
		// TODO(stevvooe): These error handling switches really need to be
		// handled by an app global mapper.
		newErr := NewHTTPError(0, nil, http.StatusBadRequest)
		httpErr := newErr.(httpError)
		switch err := err.(type) {
		case distribution.ErrManifestVerification:
			for _, verificationError := range err {
				switch verificationError := verificationError.(type) {
				case distribution.ErrUnknownLayer:
					httpErr.Push(v2.ErrorCodeBlobUnknown, verificationError.FSLayer)
				case distribution.ErrManifestUnverified:
					httpErr.Push(v2.ErrorCodeManifestUnverified)
				default:
					if verificationError == digest.ErrDigestInvalidFormat {
						// TODO(stevvooe): We need to really need to move all
						// errors to types. Its much more straightforward.
						httpErr.Push(v2.ErrorCodeDigestInvalid)
					} else {
						httpErr.PushErr(verificationError)
					}
				}
			}
		default:
			httpErr.PushErr(err)
		}

		return httpErr
	}

	// Construct a canonical url for the uploaded manifest.
	location, err := ctx.urlBuilder.BuildManifestURL(ctx.Repository.Name(), dgst.String())
	if err != nil {
		// NOTE(stevvooe): Given the behavior above, this absurdly unlikely to
		// happen. We'll log the error here but proceed as if it worked. Worst
		// case, we set an empty location header.
		ctxu.GetLogger(ctx).Errorf("error building manifest url from digest: %v", err)
	}

	w.Header().Set("Location", location)
	w.Header().Set("Docker-Content-Digest", dgst.String())
	w.WriteHeader(http.StatusAccepted)
	return nil
}

// DeleteImageManifest removes the image with the given tag from the registry.
func DeleteImageManifest(ctx *Context, w http.ResponseWriter, r *http.Request) error {
	ctxu.GetLogger(ctx).Debug("DeleteImageManifest")

	// TODO(stevvooe): Unfortunately, at this point, manifest deletes are
	// unsupported. There are issues with schema version 1 that make removing
	// tag index entries a serious problem in eventually consistent storage.
	// Once we work out schema version 2, the full deletion system will be
	// worked out and we can add support back.
	return NewHTTPError(v2.ErrorCodeUnsupported, nil, http.StatusBadRequest)
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

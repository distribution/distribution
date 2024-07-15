package handlers

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/registry/api/errcode"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/reference"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
)

const (
	defaultArch         = "amd64"
	defaultOS           = "linux"
	maxManifestBodySize = 4 * 1024 * 1024
	imageClass          = "image"
)

type storageType int

const (
	manifestSchema2     storageType = iota // 0
	manifestlistSchema                     // 1
	ociSchema                              // 2
	ociImageIndexSchema                    // 3
	numStorageTypes                        // 4
)

// manifestDispatcher takes the request context and builds the
// appropriate handler for handling manifest requests.
func manifestDispatcher(ctx *Context, app *App, r *http.Request) http.Handler {
	manifestHandler := &manifestHandler{
		Context: ctx,
		isCache: app.isCache,
		Config:  app.Config,
	}
	ref := getReference(ctx)
	dgst, err := digest.Parse(ref)
	if err != nil {
		// We just have a tag
		manifestHandler.Tag = ref
	} else {
		manifestHandler.Digest = dgst
	}

	mhandler := handlers.MethodHandler{
		http.MethodGet:  http.HandlerFunc(manifestHandler.GetManifest),
		http.MethodHead: http.HandlerFunc(manifestHandler.GetManifest),
	}

	if !app.readOnly {
		mhandler[http.MethodPut] = http.HandlerFunc(manifestHandler.PutManifest)
		mhandler[http.MethodDelete] = http.HandlerFunc(manifestHandler.DeleteManifest)
	}

	return mhandler
}

// manifestHandler handles http operations on image manifests.
type manifestHandler struct {
	*Context

	Config *configuration.Configuration

	// isCache is true if this registry is configured as a pull through cache
	isCache bool

	// One of tag or digest gets set, depending on what is present in context.
	Tag    string
	Digest digest.Digest
}

// GetManifest fetches the image manifest from the storage backend, if it exists.
func (imh *manifestHandler) GetManifest(w http.ResponseWriter, r *http.Request) {
	dcontext.GetLogger(imh).Debug("GetImageManifest")
	manifests, err := imh.Repository.Manifests(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}
	var supports [numStorageTypes]bool

	// this parsing of Accept headers is not quite as full-featured as godoc.org's parser, but we don't care about "q=" values
	// https://github.com/golang/gddo/blob/e91d4165076d7474d20abda83f92d15c7ebc3e81/httputil/header/header.go#L165-L202
	for _, acceptHeader := range r.Header["Accept"] {
		// r.Header[...] is a slice in case the request contains the same header more than once
		// if the header isn't set, we'll get the zero value, which "range" will handle gracefully

		// we need to split each header value on "," to get the full list of "Accept" values (per RFC 2616)
		// https://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.1
		for _, mediaType := range strings.Split(acceptHeader, ",") {
			if mediaType, _, err = mime.ParseMediaType(mediaType); err != nil {
				continue
			}

			if mediaType == schema2.MediaTypeManifest {
				supports[manifestSchema2] = true
			}
			if mediaType == manifestlist.MediaTypeManifestList {
				supports[manifestlistSchema] = true
			}
			if mediaType == v1.MediaTypeImageManifest {
				supports[ociSchema] = true
			}
			if mediaType == v1.MediaTypeImageIndex {
				supports[ociImageIndexSchema] = true
			}
		}
	}

	if imh.Tag != "" {
		tags := imh.Repository.Tags(imh)
		desc, err := tags.Get(imh, imh.Tag)
		if err != nil {
			if _, ok := err.(distribution.ErrTagUnknown); ok {
				imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown.WithDetail(err))
			} else {
				imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			}
			return
		}
		imh.Digest = desc.Digest
	}

	if etagMatch(r, imh.Digest.String()) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	var options []distribution.ManifestServiceOption
	if imh.Tag != "" {
		options = append(options, distribution.WithTag(imh.Tag))
	}
	manifest, err := manifests.Get(imh, imh.Digest, options...)
	if err != nil {
		if _, ok := err.(distribution.ErrManifestUnknownRevision); ok {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown.WithDetail(err))
		} else {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}
	// determine the type of the returned manifest
	manifestType := manifestSchema2
	manifestList, isManifestList := manifest.(*manifestlist.DeserializedManifestList)
	if _, isOCImanifest := manifest.(*ocischema.DeserializedManifest); isOCImanifest {
		manifestType = ociSchema
	} else if isManifestList {
		if manifestList.MediaType == manifestlist.MediaTypeManifestList {
			manifestType = manifestlistSchema
		} else if manifestList.MediaType == v1.MediaTypeImageIndex {
			manifestType = ociImageIndexSchema
		}
	}

	if manifestType == ociSchema && !supports[ociSchema] {
		imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown.WithMessage("OCI manifest found, but accept header does not support OCI manifests"))
		return
	}
	if manifestType == ociImageIndexSchema && !supports[ociImageIndexSchema] {
		imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown.WithMessage("OCI index found, but accept header does not support OCI indexes"))
		return
	}

	if imh.Tag != "" && manifestType == manifestlistSchema && !supports[manifestlistSchema] {
		// Rewrite manifest in schema1 format
		dcontext.GetLogger(imh).Infof("rewriting manifest list %s in schema1 format to support old client", imh.Digest.String())

		// Find the image manifest corresponding to the default
		// platform
		var manifestDigest digest.Digest
		for _, manifestDescriptor := range manifestList.Manifests {
			if manifestDescriptor.Platform.Architecture == defaultArch && manifestDescriptor.Platform.OS == defaultOS {
				manifestDigest = manifestDescriptor.Digest
				break
			}
		}

		if manifestDigest == "" {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown)
			return
		}

		manifest, err = manifests.Get(imh, manifestDigest)
		if err != nil {
			if _, ok := err.(distribution.ErrManifestUnknownRevision); ok {
				imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown.WithDetail(err))
			} else {
				imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			}
			return
		}

		if _, isSchema2 := manifest.(*schema2.DeserializedManifest); isSchema2 && !supports[manifestSchema2] {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestInvalid.WithMessage("Schema 2 manifest not supported by client"))
			return
		} else {
			imh.Digest = manifestDigest
		}
	}

	ct, p, err := manifest.Payload()
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", fmt.Sprint(len(p)))
	w.Header().Set("Docker-Content-Digest", imh.Digest.String())
	w.Header().Set("Etag", fmt.Sprintf(`"%s"`, imh.Digest))

	if r.Method == http.MethodHead {
		return
	}

	if _, err := w.Write(p); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func etagMatch(r *http.Request, etag string) bool {
	for _, headerVal := range r.Header["If-None-Match"] {
		if headerVal == etag || headerVal == fmt.Sprintf(`"%s"`, etag) { // allow quoted or unquoted
			return true
		}
	}
	return false
}

// PutManifest validates and stores a manifest in the registry.
func (imh *manifestHandler) PutManifest(w http.ResponseWriter, r *http.Request) {
	dcontext.GetLogger(imh).Debug("PutImageManifest")
	manifests, err := imh.Repository.Manifests(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	var jsonBuf bytes.Buffer
	if err := copyFullPayload(imh, w, r, &jsonBuf, maxManifestBodySize, "image manifest PUT"); err != nil {
		// copyFullPayload reports the error if necessary
		imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestInvalid.WithDetail(err.Error()))
		return
	}

	mediaType := r.Header.Get("Content-Type")
	manifest, desc, err := distribution.UnmarshalManifest(mediaType, jsonBuf.Bytes())
	if err != nil {
		imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestInvalid.WithDetail(err))
		return
	}

	if imh.Digest != "" {
		if desc.Digest != imh.Digest {
			dcontext.GetLogger(imh).Errorf("payload digest does not match: %q != %q", desc.Digest, imh.Digest)
			imh.Errors = append(imh.Errors, errcode.ErrorCodeDigestInvalid)
			return
		}
	} else if imh.Tag != "" {
		imh.Digest = desc.Digest
	} else {
		imh.Errors = append(imh.Errors, errcode.ErrorCodeTagInvalid.WithDetail("no tag or digest specified"))
		return
	}

	isAnOCIManifest := mediaType == v1.MediaTypeImageManifest || mediaType == v1.MediaTypeImageIndex

	if isAnOCIManifest {
		dcontext.GetLogger(imh).Debug("Putting an OCI Manifest!")
	} else {
		dcontext.GetLogger(imh).Debug("Putting a Docker Manifest!")
	}

	var options []distribution.ManifestServiceOption
	if imh.Tag != "" {
		options = append(options, distribution.WithTag(imh.Tag))
	}

	if err := imh.applyResourcePolicy(manifest); err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	_, err = manifests.Put(imh, manifest, options...)
	if err != nil {
		// TODO(stevvooe): These error handling switches really need to be
		// handled by an app global mapper.
		if err == distribution.ErrUnsupported {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnsupported)
			return
		}
		if err == distribution.ErrAccessDenied {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeDenied)
			return
		}
		switch err := err.(type) {
		case distribution.ErrManifestVerification:
			for _, verificationError := range err {
				switch verificationError := verificationError.(type) {
				case distribution.ErrManifestBlobUnknown:
					imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestBlobUnknown.WithDetail(verificationError.Digest))
				case distribution.ErrManifestNameInvalid:
					imh.Errors = append(imh.Errors, errcode.ErrorCodeNameInvalid.WithDetail(err))
				case distribution.ErrManifestUnverified:
					imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnverified)
				default:
					if verificationError == digest.ErrDigestInvalidFormat {
						imh.Errors = append(imh.Errors, errcode.ErrorCodeDigestInvalid)
					} else {
						imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown, verificationError)
					}
				}
			}
		case errcode.Error:
			imh.Errors = append(imh.Errors, err)
		default:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}
		return
	}

	// Tag this manifest
	if imh.Tag != "" {
		tags := imh.Repository.Tags(imh)
		err = tags.Tag(imh, imh.Tag, desc)
		if err != nil {
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			return
		}

	}

	// Construct a canonical url for the uploaded manifest.
	ref, err := reference.WithDigest(imh.Repository.Named(), imh.Digest)
	if err != nil {
		imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	location, err := imh.urlBuilder.BuildManifestURL(ref)
	if err != nil {
		// NOTE(stevvooe): Given the behavior above, this absurdly unlikely to
		// happen. We'll log the error here but proceed as if it worked. Worst
		// case, we set an empty location header.
		dcontext.GetLogger(imh).Errorf("error building manifest url from digest: %v", err)
	}

	w.Header().Set("Location", location)
	w.Header().Set("Docker-Content-Digest", imh.Digest.String())
	w.WriteHeader(http.StatusCreated)

	dcontext.GetLogger(imh).Debug("Succeeded in putting manifest!")
}

// applyResourcePolicy checks whether the resource class matches what has
// been authorized and allowed by the policy configuration.
func (imh *manifestHandler) applyResourcePolicy(manifest distribution.Manifest) error {
	allowedClasses := imh.Config.Policy.Repository.Classes
	if len(allowedClasses) == 0 {
		return nil
	}

	var class string
	switch m := manifest.(type) {
	case *schema2.DeserializedManifest:
		switch m.Config.MediaType {
		case schema2.MediaTypeImageConfig:
			class = imageClass
		case schema2.MediaTypePluginConfig:
			class = "plugin"
		default:
			return errcode.ErrorCodeDenied.WithMessage("unknown manifest class for " + m.Config.MediaType)
		}
	case *ocischema.DeserializedManifest:
		switch m.Config.MediaType {
		case v1.MediaTypeImageConfig:
			class = imageClass
		default:
			return errcode.ErrorCodeDenied.WithMessage("unknown manifest class for " + m.Config.MediaType)
		}
	}

	if class == "" {
		return nil
	}

	// Check to see if class is allowed in registry
	var allowedClass bool
	for _, c := range allowedClasses {
		if class == c {
			allowedClass = true
			break
		}
	}
	if !allowedClass {
		return errcode.ErrorCodeDenied.WithMessage(fmt.Sprintf("registry does not allow %s manifest", class))
	}

	resources := authorizedResources(imh)
	n := imh.Repository.Named().Name()

	var foundResource bool
	for _, r := range resources {
		if r.Name == n {
			if r.Class == "" {
				r.Class = imageClass
			}
			if r.Class == class {
				return nil
			}
			foundResource = true
		}
	}

	// resource was found but no matching class was found
	if foundResource {
		return errcode.ErrorCodeDenied.WithMessage(fmt.Sprintf("repository not authorized for %s manifest", class))
	}

	return nil
}

// DeleteManifest removes the manifest with the given digest or the tag with the given name from the registry.
func (imh *manifestHandler) DeleteManifest(w http.ResponseWriter, r *http.Request) {
	dcontext.GetLogger(imh).Debug("DeleteImageManifest")

	if imh.isCache {
		imh.Errors = append(imh.Errors, errcode.ErrorCodeUnsupported)
		return
	}

	if imh.Tag != "" {
		dcontext.GetLogger(imh).Debug("DeleteImageTag")
		tagService := imh.Repository.Tags(imh.Context)
		if err := tagService.Untag(imh.Context, imh.Tag); err != nil {
			switch err.(type) {
			case distribution.ErrTagUnknown, driver.PathNotFoundError:
				imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown.WithDetail(err))
			default:
				imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			}
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	manifests, err := imh.Repository.Manifests(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	err = manifests.Delete(imh, imh.Digest)
	if err != nil {
		switch err {
		case digest.ErrDigestUnsupported:
		case digest.ErrDigestInvalidFormat:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeDigestInvalid)
			return
		case distribution.ErrBlobUnknown:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeManifestUnknown)
			return
		case distribution.ErrUnsupported:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnsupported)
			return
		default:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown)
			return
		}
	}

	tagService := imh.Repository.Tags(imh)
	referencedTags, err := tagService.Lookup(imh, distribution.Descriptor{Digest: imh.Digest})
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	var (
		errs []error
		mu   sync.Mutex
	)
	g := errgroup.Group{}
	g.SetLimit(storage.DefaultConcurrencyLimit)
	for _, tag := range referencedTags {
		tag := tag

		g.Go(func() error {
			if err := tagService.Untag(imh, tag); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait() // imh will record all errors, so ignore the error of Wait()
	imh.Errors = errs

	w.WriteHeader(http.StatusAccepted)
}

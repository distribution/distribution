package handlers

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/docker/distribution"
	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/docker/distribution/reference"
	"github.com/docker/distribution/registry/api/errcode"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/registry/auth"
	"github.com/gorilla/handlers"
)

// These constants determine which architecture and OS to choose from a
// manifest list when downconverting it to a schema1 manifest.
const (
	defaultArch = "amd64"
	defaultOS   = "linux"
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
		"GET":  http.HandlerFunc(imageManifestHandler.GetImageManifest),
		"HEAD": http.HandlerFunc(imageManifestHandler.GetImageManifest),
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

// annotatedTag will annotate OCI tags by prepending a string, and leave docker
// tags unmodified.
func (imh *imageManifestHandler) annotatedTag(oci bool) string {
	if oci {
		return "oci:" + imh.Tag
	}
	return imh.Tag
}

// GetImageManifest fetches the image manifest from the storage backend, if it exists.
func (imh *imageManifestHandler) GetImageManifest(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("\n\nGetting a manifest!\n\n\n")
	supportsSchema2 := false
	supportsManifestList := false
	supportsOCISchema := false
	supportsOCIManifestList := false
	if acceptHeaders, ok := r.Header["Accept"]; ok {
		for _, mediaType := range acceptHeaders {
			if mediaType == schema2.MediaTypeManifest {
				supportsSchema2 = true
			}
			if mediaType == manifestlist.MediaTypeManifestList {
				supportsManifestList = true
			}
			if mediaType == schema2.MediaTypeOCIManifest {
				supportsOCISchema = true
			}
			if mediaType == manifestlist.MediaTypeOCIManifestList {
				supportsOCIManifestList = true
			}
		}
	}

	supportsOCI := supportsOCISchema || supportsOCIManifestList

	ctxu.GetLogger(imh).Debug("GetImageManifest")
	manifests, err := imh.Repository.Manifests(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	var manifest distribution.Manifest
	if imh.Tag != "" {
		tags := imh.Repository.Tags(imh)
		desc, err := tags.Get(imh, imh.annotatedTag(supportsOCI))
		if err != nil {
			if _, ok := err.(distribution.ErrTagUnknown); !ok {
				fmt.Printf("\n\nXXX: %T: %v\n\n\n", err, err)
				imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnknown.WithDetail(err))
				return
			}
			if supportsOCI {
				desc, err = tags.Get(imh, imh.annotatedTag(false))
				if err != nil {
					fmt.Printf("\n\nXXX: %T: %v\n\n\n", err, err)
					imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnknown.WithDetail(err))
					return
				}
			}
		}
		imh.Digest = desc.Digest
	}

	if etagMatch(r, imh.Digest.String()) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	var options []distribution.ManifestServiceOption
	if imh.Tag != "" {
		options = append(options, distribution.WithTag(imh.annotatedTag(supportsOCI)))
	}
	manifest, err = manifests.Get(imh, imh.Digest, options...)
	if err != nil {
		imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnknown.WithDetail(err))
		return
	}

	schema2Manifest, isSchema2 := manifest.(*schema2.DeserializedManifest)
	manifestList, isManifestList := manifest.(*manifestlist.DeserializedManifestList)

	isAnOCIManifest := isSchema2 && (schema2Manifest.MediaType == schema2.MediaTypeOCIManifest)
	isAnOCIManifestList := isManifestList && (manifestList.MediaType == manifestlist.MediaTypeOCIManifestList)

	badCombinations := [][]bool{
		{isSchema2 && !isAnOCIManifest, supportsOCISchema && !supportsSchema2},
		{isManifestList && !isAnOCIManifestList, supportsOCIManifestList && !supportsManifestList},
		{isAnOCIManifest, !supportsOCISchema && supportsSchema2},
		{isAnOCIManifestList, !supportsOCIManifestList && supportsManifestList},
	}
	for i, combo := range badCombinations {
		if combo[0] && combo[1] {
			fmt.Printf("\n\nbad combo! %d\n\n\n", i)
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	if isAnOCIManifest {
		fmt.Print("\n\nreturning OCI manifest\n\n")
	} else if isSchema2 {
		fmt.Print("\n\nreturning schema 2\n\n")
	} else {
		fmt.Print("\n\nwell I guess this is a schema1 manifest\n\n")
	}

	// Only rewrite schema2 manifests when they are being fetched by tag.
	// If they are being fetched by digest, we can't return something not
	// matching the digest.
	if imh.Tag != "" && isSchema2 && !(supportsSchema2 || supportsOCISchema) {
		// Rewrite manifest in schema1 format
		ctxu.GetLogger(imh).Infof("rewriting manifest %s in schema1 format to support old client", imh.Digest.String())

		manifest, err = imh.convertSchema2Manifest(schema2Manifest)
		if err != nil {
			return
		}
	} else if imh.Tag != "" && isManifestList && !(supportsManifestList || supportsOCIManifestList) {
		// Rewrite manifest in schema1 format
		ctxu.GetLogger(imh).Infof("rewriting manifest list %s in schema1 format to support old client", imh.Digest.String())

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
			imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnknown)
			return
		}

		manifest, err = manifests.Get(imh, manifestDigest)
		if err != nil {
			imh.Errors = append(imh.Errors, v2.ErrorCodeManifestUnknown.WithDetail(err))
			return
		}

		// If necessary, convert the image manifest
		if schema2Manifest, isSchema2 := manifest.(*schema2.DeserializedManifest); isSchema2 && !supportsSchema2 {
			manifest, err = imh.convertSchema2Manifest(schema2Manifest)
			if err != nil {
				return
			}
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
	w.Write(p)

	fmt.Printf("\n\nThey succeeded!\n\n\n")
}

func (imh *imageManifestHandler) convertSchema2Manifest(schema2Manifest *schema2.DeserializedManifest) (distribution.Manifest, error) {
	targetDescriptor := schema2Manifest.Target()
	blobs := imh.Repository.Blobs(imh)
	configJSON, err := blobs.Get(imh, targetDescriptor.Digest)
	if err != nil {
		imh.Errors = append(imh.Errors, v2.ErrorCodeManifestInvalid.WithDetail(err))
		return nil, err
	}

	ref := imh.Repository.Named()

	if imh.Tag != "" {
		ref, err = reference.WithTag(ref, imh.Tag)
		if err != nil {
			imh.Errors = append(imh.Errors, v2.ErrorCodeTagInvalid.WithDetail(err))
			return nil, err
		}
	}

	builder := schema1.NewConfigManifestBuilder(imh.Repository.Blobs(imh), imh.Context.App.trustKey, ref, configJSON)
	for _, d := range schema2Manifest.Layers {
		if err := builder.AppendReference(d); err != nil {
			imh.Errors = append(imh.Errors, v2.ErrorCodeManifestInvalid.WithDetail(err))
			return nil, err
		}
	}
	manifest, err := builder.Build(imh)
	if err != nil {
		imh.Errors = append(imh.Errors, v2.ErrorCodeManifestInvalid.WithDetail(err))
		return nil, err
	}
	imh.Digest = digest.FromBytes(manifest.(*schema1.SignedManifest).Canonical)

	return manifest, nil
}

func etagMatch(r *http.Request, etag string) bool {
	for _, headerVal := range r.Header["If-None-Match"] {
		if headerVal == etag || headerVal == fmt.Sprintf(`"%s"`, etag) { // allow quoted or unquoted
			return true
		}
	}
	return false
}

// PutImageManifest validates and stores an image in the registry.
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

	mediaType := r.Header.Get("Content-Type")
	manifest, desc, err := distribution.UnmarshalManifest(mediaType, jsonBuf.Bytes())
	if err != nil {
		imh.Errors = append(imh.Errors, v2.ErrorCodeManifestInvalid.WithDetail(err))
		return
	}

	if imh.Digest != "" {
		if desc.Digest != imh.Digest {
			ctxu.GetLogger(imh).Errorf("payload digest does match: %q != %q", desc.Digest, imh.Digest)
			imh.Errors = append(imh.Errors, v2.ErrorCodeDigestInvalid)
			return
		}
	} else if imh.Tag != "" {
		imh.Digest = desc.Digest
	} else {
		imh.Errors = append(imh.Errors, v2.ErrorCodeTagInvalid.WithDetail("no tag or digest specified"))
		return
	}

	isAnOCIManifest := mediaType == schema2.MediaTypeOCIManifest || mediaType == manifestlist.MediaTypeOCIManifestList

	if isAnOCIManifest {
		fmt.Printf("\n\nThey're putting an OCI Manifest!\n\n\n")
	} else {
		fmt.Printf("\n\nThey're putting a Docker Manifest!\n\n\n")
	}

	var options []distribution.ManifestServiceOption
	if imh.Tag != "" {
		options = append(options, distribution.WithTag(imh.annotatedTag(isAnOCIManifest)))
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
			fmt.Printf("\n\nXXX 1\n\n\n")
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnsupported)
			return
		}
		if err == distribution.ErrAccessDenied {
			fmt.Printf("\n\nXXX 2\n\n\n")
			imh.Errors = append(imh.Errors, errcode.ErrorCodeDenied)
			return
		}
		switch err := err.(type) {
		case distribution.ErrManifestVerification:
			for _, verificationError := range err {
				switch verificationError := verificationError.(type) {
				case distribution.ErrManifestBlobUnknown:
					imh.Errors = append(imh.Errors, v2.ErrorCodeManifestBlobUnknown.WithDetail(verificationError.Digest))
				case distribution.ErrManifestNameInvalid:
					imh.Errors = append(imh.Errors, v2.ErrorCodeNameInvalid.WithDetail(err))
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
		case errcode.Error:
			imh.Errors = append(imh.Errors, err)
		default:
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		}

		fmt.Printf("\n\nXXX 3\n\n\n")
		return
	}

	// Tag this manifest
	if imh.Tag != "" {
		tags := imh.Repository.Tags(imh)
		err = tags.Tag(imh, imh.annotatedTag(isAnOCIManifest), desc)
		if err != nil {
			fmt.Printf("\n\nXXX 4: %T: %v\n\n\n", err, err)
			imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
			return
		}

	}

	// Construct a canonical url for the uploaded manifest.
	ref, err := reference.WithDigest(imh.Repository.Named(), imh.Digest)
	if err != nil {
		fmt.Printf("\n\nXXX 5\n\n\n")
		imh.Errors = append(imh.Errors, errcode.ErrorCodeUnknown.WithDetail(err))
		return
	}

	location, err := imh.urlBuilder.BuildManifestURL(ref)
	if err != nil {
		// NOTE(stevvooe): Given the behavior above, this absurdly unlikely to
		// happen. We'll log the error here but proceed as if it worked. Worst
		// case, we set an empty location header.
		ctxu.GetLogger(imh).Errorf("error building manifest url from digest: %v", err)
	}

	w.Header().Set("Location", location)
	w.Header().Set("Docker-Content-Digest", imh.Digest.String())
	w.WriteHeader(http.StatusCreated)

	fmt.Printf("\n\nThey succeeded!\n\n\n")
}

// applyResourcePolicy checks whether the resource class matches what has
// been authorized and allowed by the policy configuration.
func (imh *imageManifestHandler) applyResourcePolicy(manifest distribution.Manifest) error {
	allowedClasses := imh.App.Config.Policy.Repository.Classes
	if len(allowedClasses) == 0 {
		return nil
	}

	var class string
	switch m := manifest.(type) {
	case *schema1.SignedManifest:
		class = "image"
	case *schema2.DeserializedManifest:
		switch m.Config.MediaType {
		case schema2.MediaTypeConfig:
			class = "image"
		case schema2.MediaTypePluginConfig:
			class = "plugin"
		default:
			message := fmt.Sprintf("unknown manifest class for %s", m.Config.MediaType)
			return errcode.ErrorCodeDenied.WithMessage(message)
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
		message := fmt.Sprintf("registry does not allow %s manifest", class)
		return errcode.ErrorCodeDenied.WithMessage(message)
	}

	resources := auth.AuthorizedResources(imh)
	n := imh.Repository.Named().Name()

	var foundResource bool
	for _, r := range resources {
		if r.Name == n {
			if r.Class == "" {
				r.Class = "image"
			}
			if r.Class == class {
				return nil
			}
			foundResource = true
		}
	}

	// resource was found but no matching class was found
	if foundResource {
		message := fmt.Sprintf("repository not authorized for %s manifest", class)
		return errcode.ErrorCodeDenied.WithMessage(message)
	}

	return nil

}

// DeleteImageManifest removes the manifest with the given digest from the registry.
func (imh *imageManifestHandler) DeleteImageManifest(w http.ResponseWriter, r *http.Request) {
	ctxu.GetLogger(imh).Debug("DeleteImageManifest")

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

	tagService := imh.Repository.Tags(imh)
	referencedTags, err := tagService.Lookup(imh, distribution.Descriptor{Digest: imh.Digest})
	if err != nil {
		imh.Errors = append(imh.Errors, err)
		return
	}

	for _, tag := range referencedTags {
		if err := tagService.Untag(imh, tag); err != nil {
			imh.Errors = append(imh.Errors, err)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

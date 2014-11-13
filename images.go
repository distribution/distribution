package registry

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/handlers"
)

// ImageManifest defines the structure of an image manifest
type ImageManifest struct {
	// Name is the name of the image's repository
	Name string `json:"name"`

	// Tag is the tag of the image specified by this manifest
	Tag string `json:"tag"`

	// Architecture is the host architecture on which this image is intended to
	// run
	Architecture string `json:"architecture"`

	// FSLayers is a list of filesystem layer blobSums contained in this image
	FSLayers []FSLayer `json:"fsLayers"`

	// History is a list of unstructured historical data for v1 compatibility
	History []ManifestHistory `json:"history"`

	// SchemaVersion is the image manifest schema that this image follows
	SchemaVersion int `json:"schemaVersion"`

	// Raw is the byte representation of the ImageManifest, used for signature
	// verification
	Raw []byte `json:"-"`
}

// imageManifest is used to avoid recursion in unmarshaling
type imageManifest ImageManifest

func (m *ImageManifest) UnmarshalJSON(b []byte) error {
	var manifest imageManifest
	err := json.Unmarshal(b, &manifest)
	if err != nil {
		return err
	}

	*m = ImageManifest(manifest)
	m.Raw = b
	return nil
}

// FSLayer is a container struct for BlobSums defined in an image manifest
type FSLayer struct {
	// BlobSum is the tarsum of the referenced filesystem image layer
	BlobSum string `json:"blobSum"`
}

// ManifestHistory stores unstructured v1 compatibility information
type ManifestHistory struct {
	// V1Compatibility is the raw v1 compatibility information
	V1Compatibility string `json:"v1Compatibility"`
}

// Checksum is a container struct for an image checksum
type Checksum struct {
	// HashAlgorithm is the algorithm used to compute the checksum
	// Supported values: md5, sha1, sha256, sha512
	HashAlgorithm string

	// Sum is the actual checksum value for the given HashAlgorithm
	Sum string
}

// imageManifestDispatcher takes the request context and builds the
// appropriate handler for handling image manifest requests.
func imageManifestDispatcher(ctx *Context, r *http.Request) http.Handler {
	imageManifestHandler := &imageManifestHandler{
		Context: ctx,
		Tag:     ctx.vars["tag"],
	}

	imageManifestHandler.log = imageManifestHandler.log.WithField("tag", imageManifestHandler.Tag)

	return handlers.MethodHandler{
		"GET":    http.HandlerFunc(imageManifestHandler.GetImageManifest),
		"PUT":    http.HandlerFunc(imageManifestHandler.PutImageManifest),
		"DELETE": http.HandlerFunc(imageManifestHandler.DeleteImageManifest),
	}
}

// imageManifestHandler handles http operations on image manifests.
type imageManifestHandler struct {
	*Context

	Tag string
}

// GetImageManifest fetches the image manifest from the storage backend, if it exists.
func (imh *imageManifestHandler) GetImageManifest(w http.ResponseWriter, r *http.Request) {

}

// PutImageManifest validates and stores and image in the registry.
func (imh *imageManifestHandler) PutImageManifest(w http.ResponseWriter, r *http.Request) {

}

// DeleteImageManifest removes the image with the given tag from the registry.
func (imh *imageManifestHandler) DeleteImageManifest(w http.ResponseWriter, r *http.Request) {

}

package registry

import (
	"github.com/docker/docker-registry/common"
	"github.com/gorilla/mux"
)

const (
	routeNameImageManifest     = "image-manifest"
	routeNameTags              = "tags"
	routeNameLayer             = "layer"
	routeNameLayerUpload       = "layer-upload"
	routeNameLayerUploadResume = "layer-upload-resume"
)

var allEndpoints = []string{
	routeNameImageManifest,
	routeNameTags,
	routeNameLayer,
	routeNameLayerUpload,
	routeNameLayerUploadResume,
}

// v2APIRouter builds a gorilla router with named routes for the various API
// methods. We may export this for use by the client.
func v2APIRouter() *mux.Router {
	router := mux.NewRouter().
		StrictSlash(true)

	// GET      /v2/<name>/image/<tag>	Image Manifest	Fetch the image manifest identified by name and tag.
	// PUT      /v2/<name>/image/<tag>	Image Manifest	Upload the image manifest identified by name and tag.
	// DELETE   /v2/<name>/image/<tag>	Image Manifest	Delete the image identified by name and tag.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/image/{tag:" + common.TagNameRegexp.String() + "}").
		Name(routeNameImageManifest)

	// GET	/v2/<name>/tags/list	Tags	Fetch the tags under the repository identified by name.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/tags/list").
		Name(routeNameTags)

	// GET	/v2/<name>/layer/<tarsum>	Layer	Fetch the layer identified by tarsum.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/layer/{tarsum:" + common.TarsumRegexp.String() + "}").
		Name(routeNameLayer)

	// POST	/v2/<name>/layer/<tarsum>/upload/	Layer Upload	Initiate an upload of the layer identified by tarsum. Requires length and a checksum parameter.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/layer/{tarsum:" + common.TarsumRegexp.String() + "}/upload/").
		Name(routeNameLayerUpload)

	// GET	/v2/<name>/layer/<tarsum>/upload/<uuid>	Layer Upload	Get the status of the upload identified by tarsum and uuid.
	// PUT	/v2/<name>/layer/<tarsum>/upload/<uuid>	Layer Upload	Upload all or a chunk of the upload identified by tarsum and uuid.
	// DELETE	/v2/<name>/layer/<tarsum>/upload/<uuid>	Layer Upload	Cancel the upload identified by layer and uuid
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/layer/{tarsum:" + common.TarsumRegexp.String() + "}/upload/{uuid}").
		Name(routeNameLayerUploadResume)

	return router
}

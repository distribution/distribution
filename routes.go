package registry

import (
	"github.com/docker/docker-registry/common"
	"github.com/gorilla/mux"
)

const (
	routeNameImageManifest    = "image-manifest"
	routeNameTags             = "tags"
	routeNameBlob             = "blob"
	routeNameBlobUpload       = "blob-upload"
	routeNameBlobUploadResume = "blob-upload-resume"
)

var allEndpoints = []string{
	routeNameImageManifest,
	routeNameTags,
	routeNameBlob,
	routeNameBlobUpload,
	routeNameBlobUploadResume,
}

// v2APIRouter builds a gorilla router with named routes for the various API
// methods. We may export this for use by the client.
func v2APIRouter() *mux.Router {
	router := mux.NewRouter().
		StrictSlash(true)

	// GET      /v2/<name>/manifest/<tag>	Image Manifest	Fetch the image manifest identified by name and tag.
	// PUT      /v2/<name>/manifest/<tag>	Image Manifest	Upload the image manifest identified by name and tag.
	// DELETE   /v2/<name>/manifest/<tag>	Image Manifest	Delete the image identified by name and tag.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/manifest/{tag:" + common.TagNameRegexp.String() + "}").
		Name(routeNameImageManifest)

	// GET	/v2/<name>/tags/list	Tags	Fetch the tags under the repository identified by name.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/tags/list").
		Name(routeNameTags)

	// GET	/v2/<name>/blob/<digest>	Layer	Fetch the blob identified by digest.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/blob/{digest:[a-zA-Z0-9-_+.]+:[a-zA-Z0-9-_+.=]+}").
		Name(routeNameBlob)

	// POST	/v2/<name>/blob/upload/	Layer Upload	Initiate an upload of the layer identified by tarsum.
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/blob/upload/").
		Name(routeNameBlobUpload)

	// GET	/v2/<name>/blob/upload/<uuid>	Layer Upload	Get the status of the upload identified by tarsum and uuid.
	// PUT	/v2/<name>/blob/upload/<uuid>	Layer Upload	Upload all or a chunk of the upload identified by tarsum and uuid.
	// DELETE	/v2/<name>/blob/upload/<uuid>	Layer Upload	Cancel the upload identified by layer and uuid
	router.
		Path("/v2/{name:" + common.RepositoryNameRegexp.String() + "}/blob/upload/{uuid}").
		Name(routeNameBlobUploadResume)

	return router
}

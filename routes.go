package registry

import (
	"github.com/gorilla/mux"
)

const (
	routeNameRoot             = "root"
	routeNameName             = "name"
	routeNameImageManifest    = "image-manifest"
	routeNameTags             = "tags"
	routeNameLayer            = "layer"
	routeNameStartLayerUpload = "start-layer-upload"
	routeNameLayerUpload      = "layer-upload"
)

var allEndpoints = []string{
	routeNameImageManifest,
	routeNameTags,
	routeNameLayer,
	routeNameStartLayerUpload,
	routeNameLayerUpload,
}

// v2APIRouter builds a gorilla router with named routes for the various API
// methods. We may export this for use by the client.
func v2APIRouter() *mux.Router {
	router := mux.NewRouter()

	rootRouter := router.
		PathPrefix("/v2").
		Name(routeNameRoot).
		Subrouter()

	// All routes are subordinate to named routes
	namedRouter := rootRouter.
		PathPrefix("/{name:[A-Za-z0-9-_]+/[A-Za-z0-9-_]+}"). // TODO(stevvooe): Verify this format with core
		Name(routeNameName).
		Subrouter().
		StrictSlash(true)

	// GET      /v2/<name>/image/<tag>	Image Manifest	Fetch the image manifest identified by name and tag.
	// PUT      /v2/<name>/image/<tag>	Image Manifest	Upload the image manifest identified by name and tag.
	// DELETE   /v2/<name>/image/<tag>	Image Manifest	Delete the image identified by name and tag.
	namedRouter.
		Path("/image/{tag:[A-Za-z0-9-_]+}").
		Name(routeNameImageManifest)

	// GET	/v2/<name>/tags	Tags	Fetch the tags under the repository identified by name.
	namedRouter.
		Path("/tags").
		Name(routeNameTags)

	// GET	/v2/<name>/layer/<tarsum>	Layer	Fetch the layer identified by tarsum.
	namedRouter.
		Path("/layer/{tarsum}").
		Name(routeNameLayer)

	// POST	/v2/<name>/layer/<tarsum>/upload/	Layer Upload	Initiate an upload of the layer identified by tarsum. Requires length and a checksum parameter.
	namedRouter.
		Path("/layer/{tarsum}/upload/").
		Name(routeNameStartLayerUpload)

	// GET	/v2/<name>/layer/<tarsum>/upload/<uuid>	Layer Upload	Get the status of the upload identified by tarsum and uuid.
	// PUT	/v2/<name>/layer/<tarsum>/upload/<uuid>	Layer Upload	Upload all or a chunk of the upload identified by tarsum and uuid.
	// DELETE	/v2/<name>/layer/<tarsum>/upload/<uuid>	Layer Upload	Cancel the upload identified by layer and uuid
	namedRouter.
		Path("/layer/{tarsum}/upload/{uuid}").
		Name(routeNameLayerUpload)

	return router
}

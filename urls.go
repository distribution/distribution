package registry

import (
	"net/http"
	"net/url"

	"github.com/docker/docker-registry/digest"
	"github.com/docker/docker-registry/storage"
	"github.com/gorilla/mux"
)

type urlBuilder struct {
	url    *url.URL // url root (ie http://localhost/)
	router *mux.Router
}

func newURLBuilder(root *url.URL) *urlBuilder {
	return &urlBuilder{
		url:    root,
		router: v2APIRouter(),
	}
}

func newURLBuilderFromRequest(r *http.Request) *urlBuilder {
	u := &url.URL{
		Scheme: r.URL.Scheme,
		Host:   r.Host,
	}

	return newURLBuilder(u)
}

func newURLBuilderFromString(root string) (*urlBuilder, error) {
	u, err := url.Parse(root)
	if err != nil {
		return nil, err
	}

	return newURLBuilder(u), nil
}

func (ub *urlBuilder) forManifest(m *storage.Manifest) (string, error) {
	return ub.buildManifestURL(m.Name, m.Tag)
}

func (ub *urlBuilder) buildManifestURL(name, tag string) (string, error) {
	route := clonedRoute(ub.router, routeNameImageManifest)

	manifestURL, err := route.
		Schemes(ub.url.Scheme).
		Host(ub.url.Host).
		URL("name", name, "tag", tag)
	if err != nil {
		return "", err
	}

	return manifestURL.String(), nil
}

func (ub *urlBuilder) forLayer(l storage.Layer) (string, error) {
	return ub.buildLayerURL(l.Name(), l.Digest())
}

func (ub *urlBuilder) buildLayerURL(name string, dgst digest.Digest) (string, error) {
	route := clonedRoute(ub.router, routeNameBlob)

	layerURL, err := route.
		Schemes(ub.url.Scheme).
		Host(ub.url.Host).
		URL("name", name, "digest", dgst.String())
	if err != nil {
		return "", err
	}

	return layerURL.String(), nil
}

func (ub *urlBuilder) buildLayerUploadURL(name string) (string, error) {
	route := clonedRoute(ub.router, routeNameBlobUpload)

	uploadURL, err := route.
		Schemes(ub.url.Scheme).
		Host(ub.url.Host).
		URL("name", name)
	if err != nil {
		return "", err
	}

	return uploadURL.String(), nil
}

func (ub *urlBuilder) forLayerUpload(layerUpload storage.LayerUpload) (string, error) {
	return ub.buildLayerUploadResumeURL(layerUpload.Name(), layerUpload.UUID())
}

func (ub *urlBuilder) buildLayerUploadResumeURL(name, uuid string, values ...url.Values) (string, error) {
	route := clonedRoute(ub.router, routeNameBlobUploadResume)

	uploadURL, err := route.
		Schemes(ub.url.Scheme).
		Host(ub.url.Host).
		URL("name", name, "uuid", uuid)
	if err != nil {
		return "", err
	}

	return appendValuesURL(uploadURL, values...).String(), nil
}

// appendValuesURL appends the parameters to the url.
func appendValuesURL(u *url.URL, values ...url.Values) *url.URL {
	merged := u.Query()

	for _, v := range values {
		for k, vv := range v {
			merged[k] = append(merged[k], vv...)
		}
	}

	u.RawQuery = merged.Encode()
	return u
}

// appendValues appends the parameters to the url. Panics if the string is not
// a url.
func appendValues(u string, values ...url.Values) string {
	up, err := url.Parse(u)

	if err != nil {
		panic(err) // should never happen
	}

	return appendValuesURL(up, values...).String()
}

// clondedRoute returns a clone of the named route from the router.
func clonedRoute(router *mux.Router, name string) *mux.Route {
	route := new(mux.Route)
	*route = *router.GetRoute(name) // clone the route
	return route
}

package oci

import (
	"context"
	"fmt"
	"net/http"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/configuration"
	dcontext "github.com/distribution/distribution/v3/context"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/extension"
	"github.com/distribution/distribution/v3/registry/storage"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/gorilla/handlers"
	"github.com/opencontainers/go-digest"
	"gopkg.in/yaml.v2"
)

const (
	namespaceName         = "oci"
	extensionName         = "ext"
	discoverComponentName = "discover"
	namespaceUrl          = "https://github.com/opencontainers/distribution-spec/blob/main/extensions/_oci.md"
	namespaceDescription  = "oci extension enables listing of supported registry and repository extensions"

	artifactsExtensiontName = "artifacts"
	referrersComponentName  = "referrers"
)

type ociNamespace struct {
	storageDriver    driver.StorageDriver
	discoverEnabled  bool
	referrersEnabled bool
}

type ociOptions struct {
	RegExtensionComponents      []string `yaml:"ext,omitempty"`
	ArtifactExtensionComponents []string `yaml:"artifacts,omitempty"`
}

// newOciNamespace creates a new extension namespace with the name "oci"
func newOciNamespace(ctx context.Context, storageDriver driver.StorageDriver, options configuration.ExtensionConfig) (extension.Namespace, error) {
	optionsYaml, err := yaml.Marshal(options)
	if err != nil {
		return nil, err
	}

	var ociOption ociOptions
	err = yaml.Unmarshal(optionsYaml, &ociOption)
	if err != nil {
		return nil, err
	}

	discoverEnabled := false
	for _, component := range ociOption.RegExtensionComponents {
		switch component {
		case "discover":
			discoverEnabled = true
		}
		fmt.Println(component)
	}

	referrersEnabled := false
	for _, component := range ociOption.ArtifactExtensionComponents {
		switch component {
		case "referrers":
			referrersEnabled = true
		}
		fmt.Println(component)
	}

	return &ociNamespace{
		storageDriver:    storageDriver,
		discoverEnabled:  discoverEnabled,
		referrersEnabled: referrersEnabled,
	}, nil
}

func init() {
	// register the extension namespace.
	extension.Register(namespaceName, newOciNamespace)
}

// GetManifestHandlers returns a list of manifest handlers that will be registered in the manifest store.
func (o *ociNamespace) GetManifestHandlers(repo distribution.Repository, blobStore distribution.BlobStore) []storage.ManifestHandler {
	if o.referrersEnabled {
		return []storage.ManifestHandler{
			&artifactManifestHandler{
				repository:    repo,
				blobStore:     blobStore,
				storageDriver: o.storageDriver,
			}}
	}

	return []storage.ManifestHandler{}
}

// GetRepositoryRoutes returns a list of extension routes scoped at a repository level
func (o *ociNamespace) GetRepositoryRoutes() []extension.Route {
	var routes []extension.Route

	if o.discoverEnabled {
		routes = append(routes, extension.Route{
			Namespace: namespaceName,
			Extension: extensionName,
			Component: discoverComponentName,
			Descriptor: v2.RouteDescriptor{
				Entity: "Extension",
				Methods: []v2.MethodDescriptor{
					{
						Method:      "GET",
						Description: "Get all extensions enabled for a repository.",
					},
				},
			},
			Dispatcher: o.discoverDispatcher,
		})
	}

	if o.referrersEnabled {
		routes = append(routes, extension.Route{
			Namespace: namespaceName,
			Extension: artifactsExtensiontName,
			Component: referrersComponentName,
			Descriptor: v2.RouteDescriptor{
				Entity: "Referrers",
				Methods: []v2.MethodDescriptor{
					{
						Method:      "GET",
						Description: "Get all referrers for the given digest. Currently the API doesn't support pagination.",
					},
				},
			},
			Dispatcher: o.referrersDispatcher,
		})
	}

	return routes
}

// GetRegistryRoutes returns a list of extension routes scoped at a registry level
// There are no registry scoped routes exposed by this namespace
func (o *ociNamespace) GetRegistryRoutes() []extension.Route {
	var routes []extension.Route

	if o.discoverEnabled {
		routes = append(routes, extension.Route{
			Namespace: namespaceName,
			Extension: extensionName,
			Component: discoverComponentName,
			Descriptor: v2.RouteDescriptor{
				Entity: "Extension",
				Methods: []v2.MethodDescriptor{
					{
						Method:      "GET",
						Description: "Get all extensions enabled for a registry.",
					},
				},
			},
			Dispatcher: o.discoverDispatcher,
		})
	}

	return routes
}

// GetNamespaceName returns the name associated with the namespace
func (o *ociNamespace) GetNamespaceName() string {
	return namespaceName
}

// GetNamespaceUrl returns the url link to the documentation where the namespace's extension and endpoints are defined
func (o *ociNamespace) GetNamespaceUrl() string {
	return namespaceUrl
}

// GetNamespaceDescription returns the description associated with the namespace
func (o *ociNamespace) GetNamespaceDescription() string {
	return namespaceDescription
}

func (o *ociNamespace) discoverDispatcher(ctx *extension.Context, r *http.Request) http.Handler {
	extensionHandler := &extensionHandler{
		Context:       ctx,
		storageDriver: o.storageDriver,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(extensionHandler.getExtensions),
	}
}

func (o *ociNamespace) referrersDispatcher(extCtx *extension.Context, r *http.Request) http.Handler {

	handler := &referrersHandler{
		storageDriver: o.storageDriver,
		extContext:    extCtx,
	}
	q := r.URL.Query()
	if dgstStr := q.Get("digest"); dgstStr == "" {
		dcontext.GetLogger(extCtx).Errorf("digest not available")
	} else if d, err := digest.Parse(dgstStr); err != nil {
		dcontext.GetLogger(extCtx).Errorf("error parsing digest=%q: %v", dgstStr, err)
	} else {
		handler.Digest = d
	}

	mhandler := handlers.MethodHandler{
		"GET": http.HandlerFunc(handler.getReferrers),
	}

	return mhandler
}

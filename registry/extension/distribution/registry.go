package distribution

import (
	"context"
	"net/http"

	"github.com/distribution/distribution/v3/configuration"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/distribution/v3/registry/extension"
	"github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/gorilla/handlers"
	"gopkg.in/yaml.v2"
)

const (
	namespaceName           = "distribution"
	extensionName           = "registry"
	manifestsComponentName  = "manifests"
	tagHistoryComponentName = "taghistory"
)

type distributionNamespace struct {
	storageDriver     driver.StorageDriver
	manifestsEnabled  bool
	tagHistoryEnabled bool
}

type distributionOptions struct {
	RegExtensionComponents []string `yaml:"registry,omitempty"`
}

// newDistNamespace creates a new extension namespace with the name "distribution"
func newDistNamespace(ctx context.Context, storageDriver driver.StorageDriver, options configuration.ExtensionConfig) (extension.ExtensionNamespace, error) {

	optionsYaml, err := yaml.Marshal(options)
	if err != nil {
		return nil, err
	}

	var distOptions distributionOptions
	err = yaml.Unmarshal(optionsYaml, &distOptions)
	if err != nil {
		return nil, err
	}

	manifestsEnabled := false
	tagHistoryEnabled := false
	for _, component := range distOptions.RegExtensionComponents {
		switch component {
		case "manifests":
			manifestsEnabled = true
		case "taghistory":
			tagHistoryEnabled = true
		}
	}

	return &distributionNamespace{
		storageDriver:     storageDriver,
		manifestsEnabled:  manifestsEnabled,
		tagHistoryEnabled: tagHistoryEnabled,
	}, nil
}

func init() {
	// register the extension namespace.
	extension.Register(namespaceName, newDistNamespace)
}

// GetRepositoryRoutes returns a list of extension routes scoped at a repository level
func (d *distributionNamespace) GetRepositoryRoutes() []extension.ExtensionRoute {
	var routes []extension.ExtensionRoute

	if d.manifestsEnabled {
		routes = append(routes, extension.ExtensionRoute{
			Namespace: namespaceName,
			Extension: extensionName,
			Component: manifestsComponentName,
			Descriptor: v2.RouteDescriptor{
				Entity: "Manifest",
				Methods: []v2.MethodDescriptor{
					{
						Method:      "GET",
						Description: "Get all manifest digests for a given repository. Currently the API doesn't support pagination.",
					},
				},
			},
			Dispatcher: d.manifestsDispatcher,
		})
	}

	if d.tagHistoryEnabled {
		routes = append(routes, extension.ExtensionRoute{
			Namespace: namespaceName,
			Extension: extensionName,
			Component: tagHistoryComponentName,
			Descriptor: v2.RouteDescriptor{
				Entity: "TagHistory",
				Methods: []v2.MethodDescriptor{
					{
						Method:      "GET",
						Description: "Get a set of digests that the specified tag historically pointed to",
						Requests: []v2.RequestDescriptor{
							{
								QueryParameters: []v2.ParameterDescriptor{
									{
										Name:     "tag",
										Type:     "string",
										Required: true,
									},
								},
							},
						},
					},
				},
			},
			Dispatcher: d.tagHistoryDispatcher,
		})
	}

	return routes
}

// GetRegistryRoutes returns a list of extension routes scoped at a registry level
// There are no registry scoped routes exposed by this namespace
func (d *distributionNamespace) GetRegistryRoutes() []extension.ExtensionRoute {
	return nil
}

func (d *distributionNamespace) tagHistoryDispatcher(ctx *extension.Context, r *http.Request) http.Handler {
	tagHistoryHandler := &tagHistoryHandler{
		Context:       ctx,
		storageDriver: d.storageDriver,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(tagHistoryHandler.getTagManifestDigests),
	}
}

func (d *distributionNamespace) manifestsDispatcher(ctx *extension.Context, r *http.Request) http.Handler {
	manifestsHandler := &manifestHandler{
		Context:       ctx,
		storageDriver: d.storageDriver,
	}

	return handlers.MethodHandler{
		"GET": http.HandlerFunc(manifestsHandler.getManifests),
	}
}

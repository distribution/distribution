package distribution

import (
	"context"

	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	extension "github.com/distribution/distribution/v3/registry/extension"
	repositoryextension "github.com/distribution/distribution/v3/registry/extension/repository"
)

func newRegistryExtension(ctx context.Context, options map[string]interface{}) ([]extension.Route, error) {
	ns := "distribution"
	ext := "registry"
	return []extension.Route{
		{
			Namespace: ns,
			Extension: ext,
			Component: "repository",
			Descriptor: v2.RouteDescriptor{
				Entity: "Repository",
				Methods: []v2.MethodDescriptor{
					{
						Method:      "DELETE",
						Description: "Remove repository",
					},
				},
			},
			Dispatcher: repositoryDispatcher,
		},
		{
			Namespace: ns,
			Extension: ext,
			Component: "manifests",
			Descriptor: v2.RouteDescriptor{
				Entity: "Manifest",
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
			Dispatcher: manifestDispatcher,
		},
	}, nil
}

func init() {
	repositoryextension.Register("distribution", newRegistryExtension)
}

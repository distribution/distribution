package s3

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/mitchellh/mapstructure"
)

// ServiceEndpoint configures a service's endpoint URL and signing region.
// An empty SigningRegion uses the region requested by the AWS client.
type ServiceEndpoint struct {
	URL           string `mapstructure:"url"`
	SigningRegion string `mapstructure:"signingregion"`
}

func parseServiceEndpoints(value any) (map[string]ServiceEndpoint, error) {
	var overrides map[string]ServiceEndpoint
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:      &overrides,
		ErrorUnused: true,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(value); err != nil {
		return nil, fmt.Errorf("invalid serviceendpoints parameter: %w", err)
	}
	if err := validateServiceEndpoints(overrides); err != nil {
		return nil, err
	}
	return overrides, nil
}

func validateServiceEndpoints(overrides map[string]ServiceEndpoint) error {
	for service, endpoint := range overrides {
		if service == "" || strings.TrimSpace(service) != service {
			return fmt.Errorf("serviceendpoints keys must be non-empty AWS service identifiers")
		}
		u, err := url.Parse(endpoint.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("serviceendpoints.%s.url must be an absolute HTTP or HTTPS URL without credentials, query, or fragment", service)
		}
	}
	return nil
}

func serviceEndpointResolver(overrides map[string]ServiceEndpoint, fallback endpoints.Resolver) endpoints.Resolver {
	return endpoints.ResolverFunc(func(service, region string, options ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
		endpoint, ok := overrides[service]
		if !ok {
			return fallback.EndpointFor(service, region, options...)
		}
		signingRegion := endpoint.SigningRegion
		if signingRegion == "" {
			signingRegion = region
		}
		resolved := endpoints.ResolvedEndpoint{URL: endpoint.URL, SigningRegion: signingRegion}
		if partition, ok := endpoints.PartitionForRegion(endpoints.DefaultPartitions(), region); ok {
			resolved.PartitionID = partition.ID()
		}
		return resolved, nil
	})
}

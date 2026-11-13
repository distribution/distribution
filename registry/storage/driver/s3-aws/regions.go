package s3

import (
	"context"
	"fmt"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	stsv2 "github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/distribution/distribution/v3/internal/dcontext"
)

// resolveRegionEndpoint prefers SDK v1 for backwards compatibility, since SDK v2
// defaults to regional S3/STS endpoints where SDK v1 uses global endpoints.
func resolveRegionEndpoint(ctx context.Context, service, region string, options ...func(*endpoints.Options)) (endpoints.ResolvedEndpoint, error) {
	// SDK v1 can infer URLs for unknown regions, so success alone is insufficient.
	// Regionless lookups include credential endpoints such as EC2 metadata.
	if _, listed := sdkV1Regions[region]; listed || region == "" {
		resolved, err := endpoints.DefaultResolver().EndpointFor(service, region, options...)
		if err == nil && resolved.URL != "" {
			return resolved, nil
		}
	}

	// Forward endpoint settings to the SDK v2 service resolver.
	opts := endpoints.Options{}
	opts.Set(options...)

	fips := awsv2.FIPSEndpointStateUnset
	if opts.UseFIPSEndpoint == endpoints.FIPSEndpointStateEnabled {
		fips = awsv2.FIPSEndpointStateEnabled
	}

	dualStack := awsv2.DualStackEndpointStateUnset
	if opts.UseDualStack || opts.UseDualStackEndpoint == endpoints.DualStackEndpointStateEnabled {
		dualStack = awsv2.DualStackEndpointStateEnabled
	}

	var resolved awsv2.Endpoint
	var err error
	if service == "s3" {
		resolved, err = s3v2.NewDefaultEndpointResolver().ResolveEndpoint(region, s3v2.EndpointResolverOptions{
			DisableHTTPS:         opts.DisableSSL,
			UseFIPSEndpoint:      fips,
			UseDualStackEndpoint: dualStack,
		})
	} else if service == "sts" {
		resolved, err = stsv2.NewDefaultEndpointResolver().ResolveEndpoint(region, stsv2.EndpointResolverOptions{
			DisableHTTPS:         opts.DisableSSL,
			UseFIPSEndpoint:      fips,
			UseDualStackEndpoint: dualStack,
		})
	} else {
		return endpoints.ResolvedEndpoint{}, fmt.Errorf("unsupported service %q for AWS SDK v2 endpoint resolution", service)
	}
	if err != nil {
		return endpoints.ResolvedEndpoint{}, err
	}

	dcontext.GetLoggerWithFields(ctx, map[any]any{
		"region":   region,
		"service":  service,
		"endpoint": resolved.URL,
	}).Warn("Using AWS SDK v2 endpoint resolution")

	return endpoints.ResolvedEndpoint{
		URL:           resolved.URL,
		PartitionID:   resolved.PartitionID,
		SigningRegion: resolved.SigningRegion,
		SigningName:   resolved.SigningName,
		SigningMethod: "v4",
	}, nil
}

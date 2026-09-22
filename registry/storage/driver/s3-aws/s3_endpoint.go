package s3

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// configureEndpoint applies an explicit registry endpoint, otherwise retaining
// the SDK endpoint configuration. Secure supplies a scheme only when none is explicit.
func configureEndpoint(o *s3.Options, endpoint string, secure bool) {
	if endpoint != "" {
		if !strings.Contains(endpoint, "://") {
			if secure {
				endpoint = "https://" + endpoint
			} else {
				endpoint = "http://" + endpoint
			}
		}
		o.BaseEndpoint = aws.String(endpoint)
	}
	// DisableHTTPS rewrites finalized requests. Use it only for SDK-generated
	// endpoints so it cannot overwrite an explicit custom endpoint's scheme.
	o.EndpointOptions.DisableHTTPS = !secure && o.BaseEndpoint == nil
}

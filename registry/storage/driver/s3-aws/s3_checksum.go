package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// addDeleteObjectsContentMD5 preserves the checksum sent by SDK v1 for bulk
// deletion. SDK v2 supplies a newer checksum, but older S3-compatible services
// still require Content-MD5. Smithy hashes and rewinds the serialized XML body
// before signing and sending it.
func addDeleteObjectsContentMD5(stack *middleware.Stack) error {
	if stack.ID() != "DeleteObjects" {
		return nil
	}
	return smithyhttp.AddContentChecksumMiddleware(stack)
}

// addUploadContentChecksums retains v1's upload MD5 and signed payloads for
// when_required, including over HTTPS. Explicit when_supported configuration
// instead uses the SDK's automatic checksums and streaming behavior.
func addUploadContentChecksums(stack *middleware.Stack) error {
	if stack.ID() != "PutObject" && stack.ID() != "UploadPart" {
		return nil
	}
	if err := smithyhttp.AddContentChecksumMiddleware(stack); err != nil {
		return err
	}
	hash := &v4.ComputePayloadSHA256{}
	_, err := stack.Finalize.Swap(hash.ID(), hash)
	return err
}

// The SDK resolves an absent checksum policy to WhenSupported. Inspect its
// already-loaded sources to distinguish that default from an explicit AWS
// setting, without loading configuration again or changing AWS precedence.
func resolveRequestChecksumCalculation(cfg aws.Config, configured aws.RequestChecksumCalculation) aws.RequestChecksumCalculation {
	if configured != 0 {
		return configured
	}
	for _, source := range cfg.ConfigSources {
		switch c := source.(type) {
		case config.EnvConfig:
			if c.RequestChecksumCalculation != 0 {
				return cfg.RequestChecksumCalculation
			}
		case config.SharedConfig:
			if c.RequestChecksumCalculation != 0 {
				return cfg.RequestChecksumCalculation
			}
		}
	}
	return aws.RequestChecksumCalculationWhenRequired
}

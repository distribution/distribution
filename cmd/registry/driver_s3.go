//go:build !exclude_s3 && !only_local

package main

// Register the S3 (AWS) storage driver and its CloudFront middleware.
//
// Build without it using one of:
//
//	go build -tags exclude_s3 ./cmd/registry
//	go build -tags only_local ./cmd/registry
import (
	_ "github.com/distribution/distribution/v3/registry/storage/driver/middleware/cloudfront"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/s3-aws"
)

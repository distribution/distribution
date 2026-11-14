//go:build !exclude_gcs && !only_local

package main

// Register the Google Cloud Storage driver.
//
// Build without it using one of:
//
//	go build -tags exclude_gcs ./cmd/registry
//	go build -tags only_local ./cmd/registry
import _ "github.com/distribution/distribution/v3/registry/storage/driver/gcs"

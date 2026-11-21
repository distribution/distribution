//go:build !exclude_azure && !only_local

package main

// Register the Azure Blob Storage driver.
//
// Build without it using one of:
//
//	go build -tags exclude_azure ./cmd/registry
//	go build -tags only_local ./cmd/registry
import _ "github.com/distribution/distribution/v3/registry/storage/driver/azure"

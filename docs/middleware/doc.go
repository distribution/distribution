// Package middleware provides a Repository middleware for Docker's
// distribution project which allows custom ManifestService and TagService
// implementations to be returned from distribution.Repository.
//
// This is useful for having registry store layer blobs while delegating
// responsibility for metadata to a separate system (ie. a database)
package middleware

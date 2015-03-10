package distribution

import "golang.org/x/net/context"

// Registry represents a collection of repositories, addressable by name.
type Registry interface {
	// Repository should return a reference to the named repository. The
	// registry may or may not have the repository but should always return a
	// reference.
	Repository(ctx context.Context, name string) (Repository, error)
}

// Repository is a named collection of manifests and layers.
type Repository interface {
	// Name returns the name of the repository.
	Name() string

	// Tags returns a reference to this repository's tag service.
	Tags(ctx context.Context) TagService

	// Manifests returns a reference to this repository's manifest service.
	Manifests(ctx context.Context) ManifestService

	// Blobs returns a this repository's blob service.
	Blobs(ctx context.Context) BlobService

	// Signatures returns the signature service for this repository.
	Signatures(ctx context.Context) SignatureService
}

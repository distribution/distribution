package distribution

import "golang.org/x/net/context"

// Scope defines the set of items that match a namespace.
type Scope interface {
	// Contains returns true if the name belongs to the namespace.
	Contains(name string) (bool, error)
}

// Namespace represents a collection of repositories, addressable by name.
// Generally, a namespace is backed by a set of one or more services,
// providing facilities such as registry access, trust, and indexing.
type Namespace interface {
	// Scope describes the names that can be used with this Namespace. The
	// global namespace will have a scope that matches all names. The scope
	// effectively provides an identity for the namespace.
	Scope() Scope

	// Repository should return a reference to the named repository. The
	// registry may or may not have the repository but should always return a
	// reference.
	Repository(ctx context.Context, name string) (Repository, error)

	// TODO(stevvooe): Namespaces should provide other services that allow one
	// to explore the contents of the namespace. One such example is an
	// "index" or search service. A commented example is provided here to
	// illustrate the concept.
	//
	// Index returns an IndexService that provides search across the
	// namespace, supporting content discovery and collective information
	// about available images.
	// Index(ctx context.Context) (IndexService, error)
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

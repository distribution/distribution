package middleware

import (
	"fmt"

	"github.com/docker/dhe-deploy/manager/schema"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
)

// RegisterStore should be called before instantiating the metadata middleware
// to register your storage implementation with this package.
//
// This uses some minor global state to save the registered store.
func RegisterStore(store Store) error {
	if registeredStore != nil {
		return fmt.Errorf("a store has already been registered for the metadata middleware")
	}
	registeredStore = store
	return nil
}

// Store represents an abstract datastore for use with the metadata middleware.
//
// Each function is also passed the registry context, which contains useful
// information such as the currently authed user.
type Store interface {
	ManifestStore
	TagStore
	schema.EventManager
}

type ManifestStore interface {
	// Get returns a manifest given its digest as a raw byte slice.
	//
	// If the key is not found this must return ErrNotFound from this
	// package.
	GetManifest(ctx context.Context, key string) ([]byte, error)

	// Put stores a manifest in the datastore given the manifest hash.
	PutManifest(ctx context.Context, repo, digest string, val distribution.Manifest) error

	// Delete removes a manifest by the hash.
	//
	// If the key is not found this must return ErrNotFound from this
	// package.
	DeleteManifest(ctx context.Context, key string) error
}

type TagStore interface {
	// Get returns a tag's Descriptor given its name.
	//
	// If the key is not found this must return ErrNotFound from this
	// package.
	GetTag(ctx context.Context, repo distribution.Repository, key string) (distribution.Descriptor, error)

	// Put stores a tag's Descriptor in the datastore given the tag name.
	PutTag(ctx context.Context, repo distribution.Repository, key string, val distribution.Descriptor) error

	// Delete removes a tag by the name.
	//
	// If the key is not found this must return ErrNotFound from this
	// package.
	DeleteTag(ctx context.Context, repo distribution.Repository, key string) error

	// AllTags returns all tag names as a slice of strings for the repository
	// in which a TagStore was created
	AllTags(ctx context.Context, repo distribution.Repository) ([]string, error)

	// Lookup returns all tags which point to a given digest as a slice of
	// tag names
	LookupTags(ctx context.Context, repo distribution.Repository, digest distribution.Descriptor) ([]string, error)
}

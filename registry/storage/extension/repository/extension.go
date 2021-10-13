package extension

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/extension"
	"github.com/opencontainers/go-digest"
)

// ManifestHandler gets and puts manifests of a particular type.
// ManifestHandler is a superset of storage.ManifestHandler to avoid import cycle.
type ManifestHandler interface {
	CanUnmarshal(content []byte) bool

	// Unmarshal unmarshals the manifest from a byte slice.
	Unmarshal(ctx context.Context, dgst digest.Digest, content []byte) (distribution.Manifest, error)

	// CanPut
	CanPut(manifest distribution.Manifest) bool

	// Put creates or updates the given manifest returning the manifest digest.
	Put(ctx context.Context, manifest distribution.Manifest, skipDependencyVerification bool) (digest.Digest, error)
}

type LinkedBlobStore interface {
	distribution.BlobStore
	distribution.BlobEnumerator
	LinkBlob(ctx context.Context, desc distribution.Descriptor) error
}

type LinkPathFunc func(name string, dgst digest.Digest) (string, error)

type LinkedBlobStoreOptions struct {
	// RootPath is used for enumerating blobs.
	RootPath string

	// ResolvePath resolves the link path of a certain blob in the CAS.
	ResolvePath LinkPathFunc

	// UseCache prefers cache and falls back to the backend blob store.
	UseCache bool

	// UseMiddleware allows the middleware configured for this blob store.
	UseMiddleware bool
}

type RepositoryStore interface {
	extension.Store
	LinkedBlobStore(ctx context.Context, opts LinkedBlobStoreOptions) (LinkedBlobStore, error)
}

type RepositoryExtension interface {
	extension.Extension
	ManifestHandler(ctx context.Context, repo distribution.Repository, store RepositoryStore) (ManifestHandler, error)
	RepositoryExtension(ctx context.Context, repo distribution.Repository, store RepositoryStore) (interface{}, error)
}

type InitFunc func(ctx context.Context, options map[string]interface{}) (RepositoryExtension, error)

var extensions map[string]InitFunc

// Register is used to register an InitFunc for
// a storage extension backend with the given name.
func Register(name string, initFunc InitFunc) error {
	if extensions == nil {
		extensions = make(map[string]InitFunc)
	}

	if _, exists := extensions[name]; exists {
		return fmt.Errorf("name already registered: %s", name)
	}

	extensions[name] = initFunc

	return nil
}

// Get constructs a storage extension with the given options using the named backend.
func Get(ctx context.Context, name string, options map[string]interface{}) (RepositoryExtension, error) {
	if extensions != nil {
		if initFunc, exists := extensions[name]; exists {
			return initFunc(ctx, options)
		}
	}

	return nil, fmt.Errorf("no storage repository extension registered with name: %s", name)
}

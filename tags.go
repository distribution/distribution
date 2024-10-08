package distribution

import (
	"context"

	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// TagService provides access to information about tagged objects.
type TagService interface {
	// Get retrieves the descriptor identified by the tag. Some
	// implementations may differentiate between "trusted" tags and
	// "untrusted" tags. If a tag is "untrusted", the mapping will be returned
	// as an ErrTagUntrusted error, with the target descriptor.
	Get(ctx context.Context, tag string) (v1.Descriptor, error)

	// Tag associates the tag with the provided descriptor, updating the
	// current association, if needed.
	Tag(ctx context.Context, tag string, desc v1.Descriptor) error

	// Untag removes the given tag association
	Untag(ctx context.Context, tag string) error

	// All returns the set of tags managed by this tag service
	All(ctx context.Context) ([]string, error)

	// Lookup returns the set of tags referencing the given digest.
	Lookup(ctx context.Context, digest v1.Descriptor) ([]string, error)
}

// TagManifestsProvider provides method to retrieve the digests of manifests that a tag historically
// pointed to
type TagManifestsProvider interface {
	// ManifestDigests returns set of digests that this tag historically pointed to. This also
	// includes currently linked digest. There is no ordering guaranteed
	ManifestDigests(ctx context.Context, tag string) ([]digest.Digest, error)
}

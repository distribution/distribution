package distribution

import "golang.org/x/net/context"

// TagService provides access to information about tagged objects.
type TagService interface {
	// All returns all of the tags known by this tag service.
	All(ctx context.Context) ([]string, error)

	// Get retrieves the descriptor identifed by the tag. Some implementations
	// may differentiate between "trusted" tags and "untrusted" tags. If a tag
	// is "untrusted", the mapping will be returned as an ErrTagUntrusted
	// error, with the target descriptor.
	Get(ctx context.Context, name string) (Descriptor, error)

	// Set associates the tag with the provided descriptor, updating the
	// current association, if needed.
	Set(ctx context.Context, name string, desc Descriptor) error

	// Remove the specified tag association with name.
	Remove(ctx context.Context, name string) error

	// TODO(stevvooe): Replace All() ([]string, error) with Tags() TagReader,
	// which returns a seekable reader that allows one to read result sets of
	// tags.
}

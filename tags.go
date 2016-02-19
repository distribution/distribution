package distribution

// TagService provides access to information about tagged objects.
type TagService interface {
	// Get retrieves the descriptor identified by the tag. Some
	// implementations may differentiate between "trusted" tags and
	// "untrusted" tags. If a tag is "untrusted", the mapping will be returned
	// as an ErrTagUntrusted error, with the target descriptor.
	Get(tag string) (Descriptor, error)

	// Tag associates the tag with the provided descriptor, updating the
	// current association, if needed.
	Tag(tag string, desc Descriptor) error

	// Untag removes the given tag association
	Untag(tag string) error

	// All returns the set of tags managed by this tag service
	All() ([]string, error)

	// Lookup returns the set of tags referencing the given digest.
	Lookup(digest Descriptor) ([]string, error)
}

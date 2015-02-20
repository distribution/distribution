package distribution

import "github.com/docker/distribution/digest"

// Descriptor describes targeted content. Used in conjunction with a blob
// store, a descriptor can be used to fetch, store and target any kind of
// blob. The struct also describes the wire protocol format. Fields should
// only be added but never changed.
type Descriptor struct {
	// MediaType describe the type of the content. All text based formats are
	// encoded as utf-8.
	MediaType string `json:"mediaType,omitempty"`

	// Length in bytes of content.
	Length int64 `json:"length,omitempty"`

	// Digest uniquely identifies the content. A byte stream can be verified
	// against against this digest.
	Digest digest.Digest `json:"digest,omitempty"`

	// NOTE(stevvooe): Before adding a field here, please ensure that all
	// other options have been exhausted. Much of the type relationships
	// depend on the simplicity of this type.
}

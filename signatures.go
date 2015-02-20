package distribution

import (
	"github.com/docker/distribution/digest"
	"golang.org/x/net/context"
)

// WIP(stevvooe): While the other interfaces are based on active
// specifications or proposals, the signatures interface still lacks a
// complete proposal. We may need to add or remove functionality, likely on
// the signature type itself.

// Signature is simply a blob that may contain a cryptographic blob used for
// verification. The contents are dependent on the trust system. The main
// functionality of a signature comes from the relationships expressed in the
// SignatureService interface.
type Signature interface {
	Blob // io access to the signature data itself.
}

// SignatureService provides the ability to get and set signatures for a
// particular object. One or more signatures can be associated with a blob.
type SignatureService interface {
	// Get returns the signatures associated with the provided digest.
	Get(ctx context.Context, dgst digest.Digest) ([]Signature, error)

	// Assign one or more signatures to the specified digest.
	Assign(ctx context.Context, dgst digest.Digest, signatures ...Signature) error

	// Remove one or more signatures from the specified digest.
	Remove(ctx context.Context, dgst digest.Digest, signatures ...Signature) error
}

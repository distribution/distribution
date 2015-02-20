package distribution

import (
	"github.com/docker/distribution/digest"
	"golang.org/x/net/context"
)

// WIP(stevvooe): While the other interfaces are based on active
// specifications or proposals, the signatures interface still lacks a
// complete proposal. We may need to add or remove functionality, or propose
// an actual signature type.

// SignatureService provides the ability to get and set signatures for a
// particular object. The main functionality of a signature comes from the
// relationships expressed in the SignatureService interface. A signature is
// simply a byte slice that may contain cryptographic information used for
// verification. One or more signatures can be associated with a blob. The
// byte slices may themselves be blobs but this relationship is not enforced.
// The contents are dependent on the trust system.
type SignatureService interface {
	// Get returns the signatures associated with the provided digest.
	Get(ctx context.Context, dgst digest.Digest) ([][]byte, error)

	// Assign one or more signatures to the specified digest.
	Assign(ctx context.Context, dgst digest.Digest, signatures ...[]byte) error

	// Remove one or more signatures from the specified digest.
	Remove(ctx context.Context, dgst digest.Digest, signatures ...[]byte) error
}

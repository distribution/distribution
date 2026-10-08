package storage

import (
	"context"

	"github.com/opencontainers/go-digest"
)

// putDigestAlgorithmKey is the context key under which the digest algorithm
// requested for a blobStore/linkedBlobStore Put is stored.
type putDigestAlgorithmKey struct{}

// withPutDigestAlgorithm returns a context instructing blobStore.Put (and
// linkedBlobStore.Put) to address the written content using alg, instead of
// the default canonical (sha256) algorithm. Used so that a manifest PUT by
// digest is stored under the digest algorithm it was referenced by.
func withPutDigestAlgorithm(ctx context.Context, alg digest.Algorithm) context.Context {
	if alg == "" || alg == digest.Canonical {
		return ctx
	}
	return context.WithValue(ctx, putDigestAlgorithmKey{}, alg)
}

// putDigestAlgorithm returns the digest algorithm content should be
// addressed with, defaulting to canonical (sha256) if none was requested.
func putDigestAlgorithm(ctx context.Context) digest.Algorithm {
	if alg, ok := ctx.Value(putDigestAlgorithmKey{}).(digest.Algorithm); ok && alg.Available() {
		return alg
	}
	return digest.Canonical
}

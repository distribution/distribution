package storage

import (
	"context"

	"github.com/distribution/distribution/v3"
	"github.com/opencontainers/go-digest"
)

// ReferenceService is a service to manage internal links from subjects back to
// their referrers.
type ReferenceService interface {
	// Link creates a link from a subject back to a referrer
	Link(ctx context.Context, mediaType string, referrer, subject digest.Digest) error
}

type referenceHandler struct {
	*blobStore
	repository distribution.Repository
	pathFn     func(name, mediaType string, reference, artifact_subject_must_be_manifest digest.Digest) (string, error)
}

func (r *referenceHandler) Link(ctx context.Context, artifactType string, referrer, subject digest.Digest) error {
	path, err := r.pathFn(r.repository.Named().Name(), artifactType, referrer, subject)
	if err != nil {
		return err
	}

	return r.blobStore.link(ctx, path, referrer)
}

// subjectReferrerLinkPath provides the path to the subject's referrer link
func subjectReferrerLinkPath(name, mediaType string, referrer, subject digest.Digest) (string, error) {
	return pathFor(subjectReferrerLinkPathSpec{name: name, mediaType: mediaType, referrer: referrer, subject: subject})
}

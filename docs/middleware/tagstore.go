package middleware

import (
	"github.com/docker/dhe-deploy/events"
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"

	log "github.com/Sirupsen/logrus"
	"github.com/palantir/stacktrace"
)

type tagStore struct {
	ctx   context.Context
	repo  distribution.Repository
	store Store

	blobService distribution.TagService
	// When deleting tags we need the ManifestService backed by the blobstore
	blobMfstService distribution.ManifestService
}

// Get returns a tag from the blobstore.
// Note that we don't use the metadata store for this - if we did pulls would
// fail as the the metadata exists only on the filesystem.
func (t *tagStore) Get(ctx context.Context, tag string) (distribution.Descriptor, error) {
	return t.blobService.Get(ctx, tag)
}

// Tag associates the tag with the provided descriptor, updating the
// current association, if needed.
func (t *tagStore) Tag(ctx context.Context, tag string, desc distribution.Descriptor) error {
	if err := t.blobService.Tag(ctx, tag, desc); err != nil {
		return err
	}
	err := t.store.PutTag(ctx, t.repo, tag, desc)
	if err != nil {
		return err
	}
	author, _ := ctx.Value(auth.UserNameKey).(string)
	// need to create event manager where the middleware gets initted
	err = events.TagImageEvent(t.store, author, t.repo.Named().Name(), tag)
	if err != nil {
		log.Errorf("TagImageEvent creation failed: %+v", err)
	}
	return nil
}

// Untag removes the given tag association from both the blobstore and our
// metadata store directly.
func (t *tagStore) Untag(ctx context.Context, tag string) error {
	// If the metadata store deletes a manifest we should also remove the
	// manifest from the filesystem
	if err := t.store.DeleteTag(ctx, t.repo, tag); err != nil {
		return stacktrace.Propagate(err, "error deleting tag from metadata store")
	}
	if err := t.blobService.Untag(ctx, tag); err != nil {
		return stacktrace.Propagate(err, "error untagging from blobstore")
	}
	return nil
}

// All returns the set of tags for the parent repository, as
// defined in tagStore.repo
func (t *tagStore) All(ctx context.Context) ([]string, error) {
	return t.blobService.All(ctx)
}

// Lookup returns the set of tags referencing the given digest.
func (t *tagStore) Lookup(ctx context.Context, digest distribution.Descriptor) ([]string, error) {
	return t.blobService.Lookup(ctx, digest)
}

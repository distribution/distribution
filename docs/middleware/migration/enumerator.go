package migration

import (
	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/reference"
	"github.com/palantir/stacktrace"

	log "github.com/Sirupsen/logrus"
)

type Enumerator interface {
	EnumerateRepo(ctx context.Context, reg distribution.Namespace, repoName string) error
}

// NewEnumerator returns an enumerator which provides functions to iterate over
// a repository's tags, calling the given tagEnumerator function for each tag.
func NewEnumerator(onGetTag tagEnumerator) Enumerator {
	return &enumerator{onGetTag}
}

// tagEnumerator is a function signature for handling a specific repository's tag
// on each tieration
type tagEnumerator func(ctx context.Context, repo distribution.Repository, tagName string, tag distribution.Descriptor) error

// enumerator handles iterating over a repository's tags, calling `onGetTag` on
// each tag
type enumerator struct {
	onGetTag tagEnumerator
}

// EnumerateRepo iterates over a given repository's tags, calling `EnumerateTag`
// on each tag. The repository is specified as a string via the `repoName`
// argument.
// A context and registry (distribution.Namespace) must be supplied with valid,
// instantiated drivers.
func (e *enumerator) EnumerateRepo(ctx context.Context, reg distribution.Namespace, repoName string) error {
	named, err := reference.ParseNamed(repoName)
	if err != nil {
		log.WithField("error", err).Errorf("failed to parse repo name %s", repoName)
		return nil
	}

	repo, err := reg.Repository(ctx, named)
	if err != nil {
		log.WithField("error", err).Errorf("failed to construct repository %s", repoName)
		return nil
	}

	// enumerate all repository tags
	tags, err := repo.Tags(ctx).All(ctx)
	if err != nil {
		log.WithField("error", err).Errorf("failed to return all tags for repository %s", repoName)
		return nil
	}

	for _, t := range tags {
		if err = e.EnumerateTags(ctx, repo, t); err != nil {
			log.WithField("error", err).Errorf("error processing tag during enumeration %s", t)
		}
	}

	return nil
}

// EnumerateTags is called with a tag name as a string, loads the tag's
// descriptor and delegates to `enumerator.onGetTag` with the tag name
// and descriptor for further processing.
//
// This allows us to pass custom functions for migration and consistency
// checking whilst leveraging the same enumeration code.
func (e *enumerator) EnumerateTags(ctx context.Context, repo distribution.Repository, tagName string) error {
	// TagService.All returns a slice of strings instead of a concrete
	// distribution.Descriptor. Here we transform the tag name into a
	// descriptor and call the supplied onGetTag function.
	desc, err := repo.Tags(ctx).Get(ctx, tagName)
	if err != nil {
		return stacktrace.NewError("failed retrieving tag descriptor for tag %s: %s", tagName, err)
	}

	return e.onGetTag(ctx, repo, tagName, desc)
}

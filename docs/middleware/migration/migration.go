package migration

import (
	"github.com/docker/dhe-deploy/manager/schema"
	"github.com/docker/dhe-deploy/registry/middleware"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/palantir/stacktrace"
)

func NewMigration(reg distribution.Namespace, store middleware.Store) *migration {
	m := &migration{
		isFromResume: false,
		reg:          reg,
		store:        store,
	}
	m.enumerator = NewEnumerator(m.AddTagAndManifest)
	return m
}

func NewMigrationWithEnumerator(reg distribution.Namespace, e Enumerator) *migration {
	return &migration{
		isFromResume: false,
		enumerator:   e,
		reg:          reg,
	}
}

// migration handles the migration process for moving tag and manifest
// information for repositories (stored as files in distribution) into our
// tagstore.
type migration struct {
	// reg is a distribution.Namespace instance instantiated with storage
	// drivers
	reg distribution.Namespace
	// isFromResume indicates whether this migration has been started because
	// of a previously failed attempt
	isFromResume bool
	// currentRepo stores the repository we're currently migrating (or have
	// just resumed from)
	currentRepo string
	// enumerator handles iterating through each repository's tags
	enumerator Enumerator
	// store
	store middleware.Store
}

func (m *migration) Resume(from string) {
	m.isFromResume = true
	m.currentRepo = from
}

// Migrate begins migration from either the start of all repositories or
// `currentRepo` if `isFromResume` is true.
//
// If the migration fails the name of the current repository and the error is
// returned.
func (m *migration) Migrate(ctx context.Context) (repo string, err error) {
	repositoryEnumerator, ok := m.reg.(distribution.RepositoryEnumerator)
	if !ok {
		return "", stacktrace.NewError("unable to convert Namespace to RepositoryEnumerator")
	}

	hasResumed := false
	err = repositoryEnumerator.Enumerate(ctx, func(repoName string) error {
		repo = repoName

		if m.isFromResume && !hasResumed {
			// if the repository we're iterating through is before `currentRepo`,
			// therefore we can skip this as we've already migrated this repo
			// in a previous migration attempt
			if repoName != m.currentRepo {
				return nil
			}
			// this is the same repo as the last attempt, so we can continue
			// the migration.
			hasResumed = true
		}

		context.GetLoggerWithFields(ctx, map[interface{}]interface{}{
			"repo": repoName,
		}).Infof("enumerating repository")

		err := m.enumerator.EnumerateRepo(ctx, m.reg, repoName)
		if err != nil {
			context.GetLoggerWithFields(ctx, map[interface{}]interface{}{
				"repo":  repoName,
				"error": err,
			}).Errorf("error enumerating repository")
		}
		return err
	})

	return repo, err
}

// tag represents a singla tag which is being migrated into the tagstore.
type tag struct {
	dbTag      *schema.Tag
	dbManifest *schema.Manifest

	// store is an implementation of the middleware store interface which
	// saves tags and manifests to the DB
	store middleware.Store
}

// resolveTagAndManifest constructs a concrete schema.Tag and schema.Manifest
// from the blobs stored within the registry.
func (m *migration) AddTagAndManifest(ctx context.Context, repo distribution.Repository, tagName string, tag distribution.Descriptor) error {
	repoName := repo.Named().Name()

	// Load the manifest as referred to by the tag
	mfstService, err := repo.Manifests(ctx)
	if err != nil {
		return stacktrace.NewError("unable to construct manifest service for '%s:%s': %v", repoName, tagName, err)
	}
	manifest, err := mfstService.Get(ctx, tag.Digest)
	if err != nil {
		return stacktrace.NewError("unable to retrieve manifest service for '%s:%s': %v", repoName, tagName, err)
	}

	// Note that the store expects the context to have a key named "target"
	// with the config blob; this is due to how registry works when statting
	// and verifying uploads.
	//
	// In order to re-use code for loading manifest information from a blob
	// into the DB we should load the config blob if necessary and store it
	// in the context.

	// Tackle manifest metadata such as layers, arch and OS
	if v2m, ok := manifest.(*schema2.DeserializedManifest); ok {
		// The target refers to the manifest config. We need this in order to store
		// metadata such as the OS and architecture of this manifest, so instead of
		// calling Stat we'll retrieve this blob and store it in the context for the
		// Store to process
		target := v2m.Target()
		content, err := repo.Blobs(ctx).Get(ctx, target.Digest)
		if err != nil {
			return stacktrace.NewError("unable to retrieve manifest config for '%s:%s' (digest %s): %v", repoName, tagName, target.Digest, err)
		}
		ctx = context.WithValue(ctx, "target", content)
	}

	// Manifest's PKs are formatted as `namespace/repo@sha256:...`
	named := repo.Named().String()
	if err = m.store.PutManifest(ctx, named, tag.Digest.String(), manifest); err != nil {
		return stacktrace.NewError("unable to save manifest in store for '%s:%s': %v", repoName, tagName, err)
	}
	if err = m.store.PutTag(ctx, repo, tagName, tag); err != nil {
		return stacktrace.NewError("unable to save tag in store for '%s:%s': %v", repoName, tagName, err)
	}

	return nil
}

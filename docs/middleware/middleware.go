package middleware

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/handlers"
	"github.com/docker/libtrust"
)

// registeredStore is the storage implementation used for saving manifests
// and tags. This is set by calling RegisterStore() before constructing
// the middleware.
var registeredStore Store

func InitMiddleware(ctx context.Context, repository distribution.Repository, options map[string]interface{}) (distribution.Repository, error) {
	if registeredStore == nil {
		return nil, fmt.Errorf("no store has been registered for metadata middleware")
	}

	trustKey, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		return nil, fmt.Errorf("unable to generate ephemeral signing key: %s", err)
	}

	// Return a new struct which embeds the repository anonymously. This allows
	// us to overwrite specific repository functions for loading manifest and
	// tag services.
	return &WrappedRepository{
		Repository: repository,

		app:        ctx.(*handlers.App),
		store:      registeredStore,
		signingKey: trustKey,
	}, nil

}

// WrappedRepository implements distribution.Repository, providing new calls
// when creating the TagService and MetadataService
type WrappedRepository struct {
	distribution.Repository

	app        *handlers.App
	store      Store
	signingKey libtrust.PrivateKey
}

func (repo *WrappedRepository) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	// Get the default manifest service which uses blobStore to save manifests.
	blobService, err := repo.Repository.Manifests(ctx, options...)

	return &manifestStore{
		app:        repo.app,
		ctx:        ctx,
		store:      repo.store,
		signingKey: repo.signingKey,

		repo:        repo,
		blobService: blobService,
	}, err
}

func (repo *WrappedRepository) Tags(ctx context.Context) distribution.TagService {
	blobMfstService, err := repo.Repository.Manifests(ctx)
	if err != nil {
		context.GetLoggerWithField(ctx, "err", err).Error("error creating ManifestService within metadata TagService")
	}
	return &tagStore{
		ctx:   ctx,
		repo:  repo,
		store: repo.store,

		blobService:     repo.Repository.Tags(ctx),
		blobMfstService: blobMfstService,
	}
}

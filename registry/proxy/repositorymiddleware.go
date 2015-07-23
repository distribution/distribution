package proxy

import (
	"net/http"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/client"
	"github.com/docker/distribution/registry/client/auth"
	"github.com/docker/distribution/registry/client/transport"
	"github.com/docker/distribution/registry/middleware/repository"
	"github.com/docker/distribution/registry/storage"
)

type proxyMiddleware struct {
	repository distribution.Repository
	blobStore  distribution.BlobStore
	manifests  distribution.ManifestService
}

var _ distribution.Repository = &proxyMiddleware{}

func proxyRepositoryMiddleware(repository distribution.Repository, options map[string]interface{}) (distribution.Repository, error) {
	ctx := context.Background()
	tr := transport.NewTransport(http.DefaultTransport,
		auth.NewAuthorizer(scm, auth.NewTokenHandler(http.DefaultTransport, cs, repository.Name(), "pull")))

	remoteRepo, err := client.NewRepository(ctx, repository.Name(), remoteURL, tr)
	if err != nil {
		return nil, err
	}
	localManifests, err := repository.Manifests(ctx, storage.SkipLayerVerification)
	if err != nil {
		return nil, err
	}
	remoteManifests, err := remoteRepo.Manifests(ctx)
	if err != nil {
		return nil, err
	}
	return proxyMiddleware{
		repository: repository,
		manifests: proxyManifestStore{
			repositoryName:  repository.Name(),
			localManifests:  localManifests,
			remoteManifests: remoteManifests,
			ctx:             ctx,
		},
		blobStore: proxyBlobStore{
			localStore:  repository.Blobs(ctx),
			remoteStore: remoteRepo.Blobs(ctx),
		},
	}, nil
}

// proxyMiddleware implements the Repository interface

func (prm proxyMiddleware) Name() string {
	return prm.repository.Name()
}

func (prm proxyMiddleware) Blobs(ctx context.Context) distribution.BlobStore {
	return prm.blobStore
}

func (prm proxyMiddleware) Manifests(ctx context.Context, options ...distribution.ManifestServiceOption) (distribution.ManifestService, error) {
	return prm.manifests, nil
}

func (prm proxyMiddleware) Signatures() distribution.SignatureService {
	return prm.repository.Signatures()
}

// init registers the proxy repository middleware
func init() {
	middleware.Register("proxy", middleware.InitFunc(proxyRepositoryMiddleware))
}

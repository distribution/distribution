// Package middleware - proxy wrapper for the registry

package proxy

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/client"
	"github.com/docker/distribution/registry/middleware/repository"
	"net/http"
)

type proxyMiddleware struct {
	repository distribution.Repository
	blobStore  distribution.BlobStore
	manifests  distribution.ManifestService
}

var _ distribution.Repository = &proxyMiddleware{}

func newProxyMiddleware(repository distribution.Repository, options map[string]interface{}) (distribution.Repository, error) {
	remoteURL, ok := options["remoteurl"].(string)
	if !ok || remoteURL == "" {
		return nil, fmt.Errorf("No remote URL")
	}

	ctx := context.Background()
	remoteRepo, err := client.NewRepository(ctx, repository.Name(), remoteURL, http.DefaultTransport)
	if err != nil {
		return nil, err
	}

	return proxyMiddleware{
		repository: repository,
		manifests: proxyManifestStore{
			localManifests:  repository.Manifests(),
			remoteManifests: remoteRepo.Manifests(),
		},
		blobStore: proxyBlobStore{
			localStore:  repository.Blobs(ctx),
			remoteStore: remoteRepo.Blobs(ctx),
		},
	}, nil
}

// Implement the Repository interface:

func (prm proxyMiddleware) Name() string {
	return prm.repository.Name()
}

func (prm proxyMiddleware) Blobs(ctx context.Context) distribution.BlobStore {
	return prm.blobStore
}

func (prm proxyMiddleware) Manifests() distribution.ManifestService {
	return prm.manifests
}

func (prm proxyMiddleware) Signatures() distribution.SignatureService {
	return prm.repository.Signatures()
}

// init registers the proxy registry
func init() {
	middleware.Register("proxy", middleware.InitFunc(newProxyMiddleware))
}

package proxy

import (
	"context"
	"time"

	"github.com/opencontainers/go-digest"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/registry/proxy/scheduler"
	"github.com/distribution/reference"
)

type proxyManifestStore struct {
	ctx             context.Context
	localManifests  distribution.ManifestService
	remoteManifests distribution.ManifestService
	repositoryName  reference.Named
	scheduler       *scheduler.TTLExpirationScheduler
	ttl             *time.Duration
	authChallenger  authChallenger
}

var _ distribution.ManifestService = &proxyManifestStore{}

func (pms proxyManifestStore) Exists(ctx context.Context, dgst digest.Digest) (bool, error) {
	exists, err := pms.localManifests.Exists(ctx, dgst)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}
	if err := pms.authChallenger.tryEstablishChallenges(ctx); err != nil {
		return false, err
	}
	return pms.remoteManifests.Exists(ctx, dgst)
}

func (pms proxyManifestStore) Get(ctx context.Context, dgst digest.Digest, options ...distribution.ManifestServiceOption) (distribution.Manifest, error) {
	// At this point `dgst` was either specified explicitly, or returned by the
	// tagstore with the most recent association.
	var fromRemote bool
	manifest, err := pms.localManifests.Get(ctx, dgst, options...)
	if err != nil {
		if err := pms.authChallenger.tryEstablishChallenges(ctx); err != nil {
			return nil, err
		}

		manifest, err = pms.remoteManifests.Get(ctx, dgst, options...)
		if err != nil {
			return nil, err
		}
		fromRemote = true
	}

	_, payload, err := manifest.Payload()
	if err != nil {
		return nil, err
	}

	proxyMetrics.ManifestPush(uint64(len(payload)), !fromRemote)
	if fromRemote {
		proxyMetrics.ManifestPull(uint64(len(payload)))

		_, err = pms.localManifests.Put(ctx, manifest)
		if err != nil {
			return nil, err
		}

		// Schedule the manifest blob for removal
		repoBlob, err := reference.WithDigest(pms.repositoryName, dgst)
		if err != nil {
			dcontext.GetLogger(ctx).Errorf("Error creating reference: %s", err)
			return nil, err
		}

		if pms.scheduler != nil && pms.ttl != nil {
			if err := pms.scheduler.AddManifest(repoBlob, *pms.ttl); err != nil {
				dcontext.GetLogger(ctx).Errorf("Error adding manifest: %s", err)
				return nil, err
			}
		}

		// Ensure the manifest blob is cleaned up
		// pms.scheduler.AddBlob(blobRef, repositoryTTL)

	}

	return manifest, err
}

func (pms proxyManifestStore) Put(ctx context.Context, manifest distribution.Manifest, options ...distribution.ManifestServiceOption) (digest.Digest, error) {
	var d digest.Digest
	return d, distribution.ErrUnsupported
}

func (pms proxyManifestStore) Delete(ctx context.Context, dgst digest.Digest) error {
	return distribution.ErrUnsupported
}

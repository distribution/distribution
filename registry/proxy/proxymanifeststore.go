package proxy

import (
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/client"
	"github.com/docker/distribution/registry/proxy/scheduler"
)

// todo(richardscothern): from cache control header or config
const repositoryTTL = time.Duration(24 * 7 * time.Hour)

type proxyManifestStore struct {
	ctx             context.Context
	localManifests  distribution.ManifestService
	remoteManifests distribution.ManifestService
	repositoryName  string
	scheduler       *scheduler.TTLExpirationScheduler
}

var _ distribution.ManifestService = &proxyManifestStore{}

func (pms proxyManifestStore) Exists(dgst digest.Digest) (bool, error) {
	exists, err := pms.localManifests.Exists(dgst)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	return pms.remoteManifests.Exists(dgst)
}

func (pms proxyManifestStore) Get(dgst digest.Digest) (*manifest.SignedManifest, error) {
	sm, err := pms.localManifests.Get(dgst)
	if err == nil {
		proxyMetrics.ManifestPush(uint64(len(sm.Raw)))
		return sm, err
	}

	sm, err = pms.remoteManifests.Get(dgst)
	if err != nil {
		return nil, err
	}

	proxyMetrics.ManifestPull(uint64(len(sm.Raw)))
	err = pms.localManifests.Put(sm)
	if err != nil {
		return nil, err
	}

	// Schedule the repo for removal
	pms.scheduler.AddManifest(pms.repositoryName, repositoryTTL)

	// Ensure the manifest blob is cleaned up
	pms.scheduler.AddBlob(dgst.String(), repositoryTTL)

	proxyMetrics.ManifestPush(uint64(len(sm.Raw)))

	return sm, err
}

func (pms proxyManifestStore) Tags() ([]string, error) {
	return pms.localManifests.Tags()
}

func (pms proxyManifestStore) ExistsByTag(tag string) (bool, error) {
	exists, err := pms.localManifests.ExistsByTag(tag)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	return pms.remoteManifests.ExistsByTag(tag)
}

func (pms proxyManifestStore) GetByTag(tag string, options ...distribution.ManifestServiceOption) (*manifest.SignedManifest, error) {
	var localDigest digest.Digest

	localManifest, err := pms.localManifests.GetByTag(tag, options...)
	switch err.(type) {
	case distribution.ErrManifestUnknown, distribution.ErrManifestUnknownRevision:
		goto fromremote
	case nil:
		break
	default:
		return nil, err
	}

	localDigest, err = manifestDigest(localManifest)
	if err != nil {
		return nil, err
	}

fromremote:
	var sm *manifest.SignedManifest
	sm, err = pms.remoteManifests.GetByTag(tag, client.AddEtagToTag(tag, localDigest.String()))
	if err != nil {
		return nil, err
	}

	if sm == nil {
		context.GetLogger(pms.ctx).Debugf("Local manifest for %q is latest, dgst=%s", tag, localDigest.String())
		return localManifest, nil
	}
	context.GetLogger(pms.ctx).Debugf("Updated manifest for %q, dgst=%s", tag, localDigest.String())

	err = pms.localManifests.Put(sm)
	if err != nil {
		return nil, err
	}

	dgst, err := manifestDigest(sm)
	if err != nil {
		return nil, err
	}
	pms.scheduler.AddBlob(dgst.String(), repositoryTTL)
	pms.scheduler.AddManifest(pms.repositoryName, repositoryTTL)

	proxyMetrics.ManifestPull(uint64(len(sm.Raw)))
	proxyMetrics.ManifestPush(uint64(len(sm.Raw)))

	return sm, err
}

func manifestDigest(sm *manifest.SignedManifest) (digest.Digest, error) {
	payload, err := sm.Payload()
	if err != nil {
		return "", err

	}

	dgst, err := digest.FromBytes(payload)
	if err != nil {
		return "", err
	}

	return dgst, nil
}

func (pms proxyManifestStore) Put(manifest *manifest.SignedManifest) error {
	return distribution.ErrUnsupported
}

func (pms proxyManifestStore) Delete(dgst digest.Digest) error {
	return distribution.ErrUnsupported
}

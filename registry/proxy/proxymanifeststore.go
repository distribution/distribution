package proxy

import (
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/registry/proxy/scheduler"
)

// todo(richardscothern): from cache control header
const repositoryTTL = time.Duration(10 * time.Minute)

type proxyManifestStore struct {
	ctx             context.Context
	localManifests  distribution.ManifestService
	remoteManifests distribution.ManifestService
	repositoryName  string
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

	scheduler.AddManifest(pms.repositoryName, repositoryTTL)
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
	// todo(richardscothern): use Etag
	sm, err := pms.remoteManifests.GetByTag(tag)
	if err != nil {
		return nil, err
	}

	payload, err := sm.Payload()
	if err != nil {
		return nil, err
	}

	digestFromRemote, err := digest.FromBytes(payload)
	if err != nil {
		return nil, err
	}

	remoteManifestExistsLocally, err := pms.localManifests.Exists(digestFromRemote)
	if err != nil {
		return nil, err
	}
	if remoteManifestExistsLocally {
		proxyMetrics.ManifestPush(uint64(len(payload)))
		return sm, err
	}

	context.GetLogger(pms.ctx).Infof("Newer manifest fetched for %q = %s", tag, digestFromRemote)
	err = pms.localManifests.Put(sm)
	if err != nil {
		return nil, err
	}
	proxyMetrics.ManifestPull(uint64(len(sm.Raw)))
	proxyMetrics.ManifestPush(uint64(len(sm.Raw)))

	scheduler.AddManifest(pms.repositoryName, repositoryTTL)

	return sm, err
}

func (pms proxyManifestStore) Put(manifest *manifest.SignedManifest) error {
	return v2.ErrorCodeUnsupported
}

func (pms proxyManifestStore) Delete(dgst digest.Digest) error {
	return v2.ErrorCodeUnsupported
}

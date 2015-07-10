package proxy

import (
	"fmt"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/registry/proxy/scheduler"
	"github.com/docker/libtrust"
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
		return sm, err
	}

	sm, err = pms.remoteManifests.Get(dgst)
	if err != nil {
		return nil, err
	}

	err = pms.localManifests.Put(sm, VerifyRemoteManifest)
	if err != nil {
		return nil, err
	}

	scheduler.AddManifest(pms.repositoryName, repositoryTTL)

	return sm, err
}

func (pms proxyManifestStore) Put(manifest *manifest.SignedManifest, verifyFunc distribution.ManifestVerifyFunc) error {
	return fmt.Errorf("Not supported")
}

func (pms proxyManifestStore) Delete(dgst digest.Digest) error {
	return fmt.Errorf("Not supported")
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

func (pms proxyManifestStore) GetByTag(tag string) (*manifest.SignedManifest, error) {
	// todo(richardscothern): this would be much more efficient with etag
	// support in the client.

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
		return sm, err
	}

	context.GetLogger(pms.ctx).Infof("Newer manifest fetched for %q = %s", tag, digestFromRemote)
	err = pms.localManifests.Put(sm, VerifyRemoteManifest)
	if err != nil {
		return nil, err
	}

	scheduler.AddManifest(pms.repositoryName, repositoryTTL)

	return sm, err
}

// VerifyRemoteManifest ensures that the manifest content is valid from the
// perspective of the registry proxy.  It does not ensure referenced
// blobs exists locally
func VerifyRemoteManifest(ctx context.Context, mnfst *manifest.SignedManifest, name string, bs distribution.BlobService) error {
	var errs distribution.ErrManifestVerification
	if mnfst.Name != name {
		errs = append(errs, fmt.Errorf("repository name does not match manifest name"))
	}

	if _, err := manifest.Verify(mnfst); err != nil {
		switch err {
		case libtrust.ErrMissingSignatureKey, libtrust.ErrInvalidJSONContent, libtrust.ErrMissingSignatureKey:
			errs = append(errs, distribution.ErrManifestUnverified{})
		default:
			if err.Error() == "invalid signature" { // TODO(stevvooe): This should be exported by libtrust
				errs = append(errs, distribution.ErrManifestUnverified{})
			} else {
				errs = append(errs, err)
			}
		}
	}
	return nil
}

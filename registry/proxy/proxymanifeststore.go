package proxy

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/libtrust"
)

// proxyManifest
type proxyManifestStore struct {
	ctx             context.Context
	localManifests  distribution.ManifestService
	remoteManifests distribution.ManifestService
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
		// todo(richardscothern): A temporary failure to write doesn't
		// have to be an error.  This could also be async
		return nil, err
	}
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
	// If a manifest is fetched by tag, always check the remote
	// for fresh content.

	exists, err := pms.remoteManifests.ExistsByTag(tag)
	if err != nil {
		return nil, err
	}

	if exists {
		// todo(richardscothern): get digest from local manifest
		// and use as etag with If-None-Match in the GetByTag
		// request.  If we get 307, return here
	}

	sm, err := pms.remoteManifests.GetByTag(tag)
	if err != nil {
		return nil, err
	}

	err = pms.localManifests.Put(sm, VerifyRemoteManifest)
	if err != nil {
		// todo(richardscothern): A temporary failure to write doesn't
		// have to be an error.  This could also be async
		return nil, err
	}
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

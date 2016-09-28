package middleware

import (
	"encoding/json"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest/manifestlist"
)

func (ms *manifestStore) VerifyList(ctx context.Context, mnfst *manifestlist.DeserializedManifestList) error {
	var errs distribution.ErrManifestVerification

	for _, manifestDescriptor := range mnfst.References() {
		exists, err := ms.Exists(ctx, manifestDescriptor.Digest)
		if err != nil && err != distribution.ErrBlobUnknown {
			errs = append(errs, err)
		}
		if err != nil || !exists {
			// On error here, we always append unknown blob errors.
			errs = append(errs, distribution.ErrManifestBlobUnknown{Digest: manifestDescriptor.Digest})
		}
	}

	if len(errs) != 0 {
		return errs
	}

	return nil
}

func (ms *manifestStore) UnmarshalList(ctx context.Context, dgst digest.Digest, content []byte) (distribution.Manifest, error) {
	context.GetLogger(ms.ctx).Debug("(*manifestListHandler).Unmarshal")

	var m manifestlist.DeserializedManifestList
	if err := json.Unmarshal(content, &m); err != nil {
		return nil, err
	}

	return &m, nil
}

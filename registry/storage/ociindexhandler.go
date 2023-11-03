package storage

import (
	"context"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/opencontainers/go-digest"
)

// ocischemaIndexHandler is a ManifestHandler that covers the OCI Image Index.
type ocischemaIndexHandler struct {
	*manifestListHandler
}

var _ ManifestHandler = &manifestListHandler{}

func (ms *ocischemaIndexHandler) Unmarshal(ctx context.Context, dgst digest.Digest, content []byte) (distribution.Manifest, error) {
	dcontext.GetLogger(ms.ctx).Debug("(*ociIndexHandler).Unmarshal")

	m := &ocischema.DeserializedImageIndex{}
	if err := m.UnmarshalJSON(content); err != nil {
		return nil, err
	}

	return m, nil
}

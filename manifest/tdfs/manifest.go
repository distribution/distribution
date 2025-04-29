package tdfs

import (
	"fmt"

	tdfs "github.com/2DFS/2dfs-builder/filesystem"
	"github.com/2DFS/2dfs-registry/v3"
	"github.com/opencontainers/go-digest"
)

const (
	// MediaTypeForeignLayer is the mediaType used for layers that must be
	// downloaded from foreign URLs.
	MediaTypeTdfsLayer = "application/vnd.oci.image.layer.v1.2dfs.field"
)

const (
	defaultSchemaVersion = 1
	defaultMediaType     = MediaTypeTdfsLayer
)

func init() {
	if err := distribution.RegisterManifestSchema(defaultMediaType, unmarshalTdfs); err != nil {
		panic(fmt.Sprintf("Unable to register manifest: %s", err))
	}
}

func unmarshalTdfs(b []byte) (distribution.Manifest, distribution.Descriptor, error) {
	m := &DeserializedTdfsManifest{}
	field, err := tdfs.GetField().Unmarshal(string(b))
	if err != nil {
		return nil, distribution.Descriptor{}, err
	}

	m.canonical = b
	m.MediaType = MediaTypeTdfsLayer
	m.Field = field

	return m, distribution.Descriptor{
		Digest:    digest.FromBytes(b),
		Size:      int64(len(b)),
		MediaType: defaultMediaType,
	}, nil
}

// Manifest defines a schema2 manifest.
type TdfsManifest struct {
	MediaType string
	Field     tdfs.Field
}

// References returns the descriptors of this manifests references.
func (m TdfsManifest) References() []distribution.Descriptor {
	return []distribution.Descriptor{}
}

// Target returns the target of this manifest.
func (m TdfsManifest) Target() distribution.Descriptor {
	return distribution.Descriptor{}
}

// DeserializedManifest wraps Manifest with a copy of the original JSON.
// It satisfies the distribution.Manifest interface.
type DeserializedTdfsManifest struct {
	TdfsManifest

	// canonical is the canonical byte representation of the Manifest.
	canonical []byte
}

// Payload returns the raw content of the manifest. The contents can be used to
// calculate the content identifier.
func (m DeserializedTdfsManifest) Payload() (string, []byte, error) {
	return m.MediaType, m.canonical, nil
}

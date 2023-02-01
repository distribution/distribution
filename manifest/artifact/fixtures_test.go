package artifact

import (
	_ "embed"

	"github.com/distribution/distribution/v3"
	_ "github.com/distribution/distribution/v3/manifest/ocischema"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var (
	// ManifestBytes is an example of a valid artifact manifest
	//go:embed fixtures/manifest.json
	ManifestBytes      []byte
	ManifestDescriptor = distribution.Descriptor{
		MediaType: v1.MediaTypeArtifactManifest,
		Size:      647,
		Digest:    "sha256:dadc6d0e6ccdcb629c78d1d13ee4e857b0fad468371e67839e777f2e1b9e33c4",
	}
	ManifestDeserialized = &DeserializedManifest{
		Manifest: Manifest{
			MediaType:    v1.MediaTypeArtifactManifest,
			ArtifactType: "application/vnd.example.sbom.v1",
			Blobs: []distribution.Descriptor{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:b093528a5eabd2ce6c954c0ecad0509f95536744c176f181c2640a8b126cdbcf",
					Size:      29876998,
				},
			},
			Subject: distribution.Descriptor{
				MediaType: v1.MediaTypeImageManifest,
				Digest:    "sha256:f756842dc7541130d3a327a870a38aa9521233fc076d0ee2cea895c8c0a1e388",
				Size:      549,
			},
			Annotations: map[string]string{
				"org.opencontainers.artifact.created": "2023-01-16T14:40:01Z",
				"org.example.sbom.format":             "json",
			},
		},
		canonical: ManifestBytes,
	}
)

var (
	// ManifestNoMediaType is an invalid artifact manifest because there is no
	// mediaType field, unlike image manifests the fils is mandatory.
	//go:embed fixtures/no-media-type.json
	ManifestNoMediaType []byte
)

var (
	// ManifestNoSubjectBytes is an example of a valid artifact manifest that has no
	// subject.
	//go:embed fixtures/no-subject.json
	ManifestNoSubjectBytes      []byte
	ManifestNoSubjectDescriptor = distribution.Descriptor{
		MediaType: v1.MediaTypeArtifactManifest,
		Size:      459,
		Digest:    "sha256:89d65c08998b0b94515414eb060b6c28e15f76b2c3a51efe0b2b541a53babe55",
	}
	ManifestNoSubjectDeserialized = &DeserializedManifest{
		Manifest: Manifest{
			MediaType:    v1.MediaTypeArtifactManifest,
			ArtifactType: "application/vnd.example.sbom.v1",
			Blobs: []distribution.Descriptor{
				{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    "sha256:b093528a5eabd2ce6c954c0ecad0509f95536744c176f181c2640a8b126cdbcf",
					Size:      29876998,
				},
			},
			Annotations: map[string]string{
				"org.opencontainers.artifact.created": "2023-01-16T14:40:01Z",
				"org.example.sbom.format":             "json",
			},
		},
		canonical: ManifestNoSubjectBytes,
	}
)

var (
	// ManifestBlobSubjectBytes is an example of an invalid artifact manifest
	// because the subject must be another manifest.
	//go:embed fixtures/blob-subject.json
	ManifestBlobSubjectBytes []byte
)

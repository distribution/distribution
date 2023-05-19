package testutil

import (
	"fmt"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema1" //nolint:staticcheck // Ignore SA1019: "github.com/distribution/distribution/v3/manifest/schema1" is deprecated, as it's used for backward compatibility.
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/docker/libtrust"
	"github.com/opencontainers/go-digest"
)

// MakeManifestList constructs a manifest list out of a list of manifest digests
func MakeManifestList(blobstatter distribution.BlobStatter, manifestDigests []digest.Digest) (*manifestlist.DeserializedManifestList, error) {
	ctx := context.Background()

	var manifestDescriptors []manifestlist.ManifestDescriptor
	for _, manifestDigest := range manifestDigests {
		descriptor, err := blobstatter.Stat(ctx, manifestDigest)
		if err != nil {
			return nil, err
		}
		platformSpec := manifestlist.PlatformSpec{
			Architecture: "atari2600",
			OS:           "CP/M",
			Variant:      "ternary",
			Features:     []string{"VLIW", "superscalaroutoforderdevnull"},
		}
		manifestDescriptor := manifestlist.ManifestDescriptor{
			Descriptor: descriptor,
			Platform:   platformSpec,
		}
		manifestDescriptors = append(manifestDescriptors, manifestDescriptor)
	}

	return manifestlist.FromDescriptors(manifestDescriptors)
}

// MakeSchema1Manifest constructs a schema 1 manifest from a given list of digests and returns
// the digest of the manifest.
//
// Deprecated: Docker Image Manifest v2, Schema 1 is deprecated since 2015.
// Use Docker Image Manifest v2, Schema 2, or the OCI Image Specification.
func MakeSchema1Manifest(digests []digest.Digest) (*schema1.SignedManifest, error) {
	mfst := schema1.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: "who",
		Tag:  "cares",
	}

	for _, d := range digests {
		mfst.FSLayers = append(mfst.FSLayers, schema1.FSLayer{BlobSum: d})
		mfst.History = append(mfst.History, schema1.History{V1Compatibility: ""})
	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		return nil, fmt.Errorf("unexpected error generating private key: %v", err)
	}

	signedManifest, err := schema1.Sign(&mfst, pk)
	if err != nil {
		return nil, fmt.Errorf("error signing manifest: %v", err)
	}

	return signedManifest, nil
}

// MakeSchema2Manifest constructs a schema 2 manifest from a given list of digests and returns
// the digest of the manifest
func MakeSchema2Manifest(repository distribution.Repository, digests []digest.Digest) (distribution.Manifest, error) {
	ctx := context.Background()
	blobStore := repository.Blobs(ctx)

	var configJSON []byte

	d, err := blobStore.Put(ctx, schema2.MediaTypeImageConfig, configJSON)
	if err != nil {
		return nil, fmt.Errorf("unexpected error storing content in blobstore: %v", err)
	}
	builder := schema2.NewManifestBuilder(d, configJSON)
	for _, digest := range digests {
		builder.AppendReference(distribution.Descriptor{Digest: digest})
	}

	mfst, err := builder.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("unexpected error generating manifest: %v", err)
	}

	return mfst, nil
}

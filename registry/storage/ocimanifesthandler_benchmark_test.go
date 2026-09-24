package storage

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/internal/dcontext"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/distribution/reference"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func BenchmarkOCIManifestVerify(b *testing.B) {
	ctx := dcontext.Background()
	inmem := inmemory.New()
	latDriver := &latencyDriver{
		StorageDriver: inmem,
		delay:         10 * time.Millisecond,
		enabled:       false,
	}

	registry, err := NewRegistry(ctx, latDriver,
		ManifestURLsAllowRegexp(regexp.MustCompile("^https?://foo")),
		ManifestURLsDenyRegexp(regexp.MustCompile("^https?://foo/nope")),
		EnableValidateImageIndexImagesExist,
		EnableDelete,
	)
	if err != nil {
		b.Fatal("Failed to construct namespace:", err)
	}

	repoName, err := reference.WithName("benchmark-repo")
	if err != nil {
		b.Fatal(err)
	}
	repo, err := registry.Repository(ctx, repoName)
	if err != nil {
		b.Fatal(err)
	}

	manifestService, err := repo.Manifests(ctx)
	if err != nil {
		b.Fatal(err)
	}

	config, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageConfig, nil)
	if err != nil {
		b.Fatal(err)
	}

	// Create 10 layers and put them into the blob store so existence checks succeed
	var layers []v1.Descriptor
	for i := 0; i < 10; i++ {
		content := []byte(fmt.Sprintf("layer-content-%d", i))
		desc, err := repo.Blobs(ctx).Put(ctx, v1.MediaTypeImageLayer, content)
		if err != nil {
			b.Fatal(err)
		}
		layers = append(layers, desc)
	}

	m := ocischema.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: v1.MediaTypeImageManifest,
		Config:    config,
		Layers:    layers,
	}

	dm, err := ocischema.FromStruct(m)
	if err != nil {
		b.Fatal(err)
	}

	// Enable latency injection
	latDriver.enabled = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = manifestService.Put(ctx, dm)
		if err != nil {
			b.Fatal(err)
		}
	}
}

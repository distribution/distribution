package ocischema

import (
	"bytes"
	"encoding/json"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const expectedManifestSerialization = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.oci.image.manifest.v1+json",
   "config": {
      "mediaType": "application/vnd.oci.image.config.v1+json",
      "digest": "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
      "size": 985,
      "annotations": {
         "apple": "orange"
      }
   },
   "layers": [
      {
         "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
         "digest": "sha256:62d8908bee94c202b2d35224a221aaa2058318bfa9879fa541efaecba272331b",
         "size": 153263,
         "annotations": {
            "lettuce": "wrap"
         }
      }
   ],
   "annotations": {
      "hot": "potato"
   }
}`

var (
	emptyJsonDescriptor = distribution.Descriptor{
		MediaType: v1.DescriptorEmptyJSON.MediaType,
		Size:      v1.DescriptorEmptyJSON.Size,
		Digest:    v1.DescriptorEmptyJSON.Digest,
	}
)

func makeTestManifest(mediaType string) Manifest {
	return Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: mediaType,
		Config: v1.Descriptor{
			MediaType:   v1.MediaTypeImageConfig,
			Digest:      "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:        985,
			Annotations: map[string]string{"apple": "orange"},
		},
		Layers: []v1.Descriptor{
			{
				MediaType:   v1.MediaTypeImageLayerGzip,
				Digest:      "sha256:62d8908bee94c202b2d35224a221aaa2058318bfa9879fa541efaecba272331b",
				Size:        153263,
				Annotations: map[string]string{"lettuce": "wrap"},
			},
		},
		Annotations: map[string]string{"hot": "potato"},
	}
}

func TestManifest(t *testing.T) {
	mfst := makeTestManifest(v1.MediaTypeImageManifest)

	deserialized, err := FromStruct(mfst)
	if err != nil {
		t.Fatalf("error creating DeserializedManifest: %v", err)
	}

	mediaType, canonical, _ := deserialized.Payload()

	if mediaType != v1.MediaTypeImageManifest {
		t.Fatalf("unexpected media type: %s", mediaType)
	}

	// Check that the canonical field is the same as json.MarshalIndent
	// with these parameters.
	expected, err := json.MarshalIndent(&mfst, "", "   ")
	if err != nil {
		t.Fatalf("error marshaling manifest: %v", err)
	}
	if !bytes.Equal(expected, canonical) {
		t.Fatalf("manifest bytes not equal:\nexpected:\n%s\nactual:\n%s\n", string(expected), string(canonical))
	}

	// Check that canonical field matches expected value.
	if !bytes.Equal([]byte(expectedManifestSerialization), canonical) {
		t.Fatalf("manifest bytes not equal:\nexpected:\n%s\nactual:\n%s\n", expectedManifestSerialization, string(canonical))
	}

	var unmarshalled DeserializedManifest
	if err := json.Unmarshal(deserialized.canonical, &unmarshalled); err != nil {
		t.Fatalf("error unmarshaling manifest: %v", err)
	}

	if !reflect.DeepEqual(&unmarshalled, deserialized) {
		t.Fatalf("manifests are different after unmarshaling: %v != %v", unmarshalled, *deserialized)
	}
	if deserialized.Annotations["hot"] != "potato" {
		t.Fatalf("unexpected annotation in manifest: %s", deserialized.Annotations["hot"])
	}

	target := deserialized.Target()
	if target.Digest != "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b" {
		t.Fatalf("unexpected digest in target: %s", target.Digest.String())
	}
	if target.MediaType != v1.MediaTypeImageConfig {
		t.Fatalf("unexpected media type in target: %s", target.MediaType)
	}
	if target.Size != 985 {
		t.Fatalf("unexpected size in target: %d", target.Size)
	}
	if target.Annotations["apple"] != "orange" {
		t.Fatalf("unexpected annotation in target: %s", target.Annotations["apple"])
	}

	references := deserialized.References()
	if len(references) != 2 {
		t.Fatalf("unexpected number of references: %d", len(references))
	}

	if !reflect.DeepEqual(references[0], target) {
		t.Fatalf("first reference should be target: %v != %v", references[0], target)
	}

	// Test the second reference
	if references[1].Digest != "sha256:62d8908bee94c202b2d35224a221aaa2058318bfa9879fa541efaecba272331b" {
		t.Fatalf("unexpected digest in reference: %s", references[0].Digest.String())
	}
	if references[1].MediaType != v1.MediaTypeImageLayerGzip {
		t.Fatalf("unexpected media type in reference: %s", references[0].MediaType)
	}
	if references[1].Size != 153263 {
		t.Fatalf("unexpected size in reference: %d", references[0].Size)
	}
	if references[1].Annotations["lettuce"] != "wrap" {
		t.Fatalf("unexpected annotation in reference: %s", references[1].Annotations["lettuce"])
	}
}

func TestManifestUnmarshal(t *testing.T) {
	_, descriptor, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(expectedManifestSerialization))
	if err != nil {
		t.Fatalf("unmarshal manifest failed: %v", err)
	}
	mfst := makeTestManifest(v1.MediaTypeImageManifest)

	deserialized, err := FromStruct(mfst)
	if err != nil {
		t.Fatalf("error creating DeserializedManifest: %v", err)
	}

	if !reflect.DeepEqual(descriptor.Annotations, deserialized.Annotations) {
		t.Fatalf("manifest annotation not equal:\nexpected:\n%v\nactual:\n%v\n", deserialized.Annotations, descriptor.Annotations)
	}
	if len(descriptor.Annotations) != 1 {
		t.Fatal("manifest index annotation length should be 1")
	}
	if descriptor.Size != int64(len([]byte(expectedManifestSerialization))) {
		t.Fatalf("manifest size is not correct:\nexpected:\n%d\nactual:\n%v\n", int64(len([]byte(expectedManifestSerialization))), descriptor.Size)
	}
	if descriptor.Digest.String() != digest.FromBytes([]byte(expectedManifestSerialization)).String() {
		t.Fatalf("manifest digest is not correct:\nexpected:\n%s\nactual:\n%s\n", digest.FromBytes([]byte(expectedManifestSerialization)), descriptor.Digest)
	}
	if descriptor.MediaType != v1.MediaTypeImageManifest {
		t.Fatalf("manifest media type is not correct:\nexpected:\n%s\nactual:\n%s\n", v1.MediaTypeImageManifest, descriptor.MediaType)
	}
}

func manifestMediaTypeTest(mediaType string, shouldError bool) func(*testing.T) {
	return func(t *testing.T) {
		mfst := makeTestManifest(mediaType)

		deserialized, err := FromStruct(mfst)
		if err != nil {
			t.Fatalf("error creating DeserializedManifest: %v", err)
		}

		unmarshalled, descriptor, err := distribution.UnmarshalManifest(
			v1.MediaTypeImageManifest,
			deserialized.canonical)

		if shouldError {
			if err == nil {
				t.Fatal("bad content type should have produced error")
			}
		} else {
			if err != nil {
				t.Fatalf("error unmarshaling manifest, %v", err)
			}

			asManifest := unmarshalled.(*DeserializedManifest)
			if asManifest.MediaType != mediaType {
				t.Fatalf("Bad media type '%v' as unmarshalled", asManifest.MediaType)
			}

			if descriptor.MediaType != v1.MediaTypeImageManifest {
				t.Fatalf("Bad media type '%v' for descriptor", descriptor.MediaType)
			}

			unmarshalledMediaType, _, _ := unmarshalled.Payload()
			if unmarshalledMediaType != v1.MediaTypeImageManifest {
				t.Fatalf("Bad media type '%v' for payload", unmarshalledMediaType)
			}
		}
	}
}

func TestManifestMediaTypes(t *testing.T) {
	t.Run("No_MediaType", manifestMediaTypeTest("", false))
	t.Run("ImageManifest", manifestMediaTypeTest(v1.MediaTypeImageManifest, false))
	t.Run("Bad_MediaType", manifestMediaTypeTest(v1.MediaTypeImageManifest+"XXX", true))
}

func TestValidateManifest(t *testing.T) {
	mfst := Manifest{
		Config: v1.Descriptor{Size: 1},
		Layers: []v1.Descriptor{{Size: 2}},
	}
	index := manifestlist.ManifestList{
		Manifests: []manifestlist.ManifestDescriptor{
			{Descriptor: v1.Descriptor{Size: 3}},
		},
	}
	t.Run("valid", func(t *testing.T) {
		b, err := json.Marshal(mfst)
		if err != nil {
			t.Fatal("unexpected error marshaling manifest", err)
		}
		if err := validateManifest(b); err != nil {
			t.Error("manifest should be valid", err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		b, err := json.Marshal(index)
		if err != nil {
			t.Fatal("unexpected error marshaling index", err)
		}
		if err := validateManifest(b); err == nil {
			t.Error("index should not be valid")
		}
	})
}

func TestArtifactManifest(t *testing.T) {
	for name, test := range map[string]struct {
		manifest             Manifest
		expectValid          bool
		expectedArtifactType string
	}{
		"not_artifact": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				MediaType: v1.MediaTypeImageManifest,
				Config: distribution.Descriptor{
					MediaType: v1.MediaTypeImageConfig,
					Size:      200,
					Digest:    "sha256:4de6702c739d8c9ed907f4c031fd0abc54ee1bf372603a585e139730772cc0b8",
				},
				Layers: []distribution.Descriptor{
					{
						MediaType: v1.MediaTypeImageLayerGzip,
						Size:      23423,
						Digest:    "sha256:ff1b4a27562d8ffc821b4d7368818ad7c759cfc2068b7adf0d2712315d67359a",
					},
				},
			},
			expectValid:          true,
			expectedArtifactType: v1.MediaTypeImageConfig,
		},
		"typical_artifact": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				Config: distribution.Descriptor{
					MediaType: "application/vnd.example.thing",
					Size:      200,
					Digest:    "sha256:4de6702c739d8c9ed907f4c031fd0abc54ee1bf372603a585e139730772cc0b8",
				},
				Layers: []distribution.Descriptor{
					{
						MediaType: v1.MediaTypeImageLayerGzip,
						Size:      23423,
						Digest:    "sha256:ff1b4a27562d8ffc821b4d7368818ad7c759cfc2068b7adf0d2712315d67359a",
					},
				},
			},
			expectValid:          true,
			expectedArtifactType: "application/vnd.example.thing",
		},
		"also_typical_artifact": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				ArtifactType: "application/vnd.example.sbom",
				Config: distribution.Descriptor{
					MediaType: v1.MediaTypeImageConfig,
					Size:      200,
					Digest:    "sha256:4de6702c739d8c9ed907f4c031fd0abc54ee1bf372603a585e139730772cc0b8",
				},
				Layers: []distribution.Descriptor{
					{
						MediaType: v1.MediaTypeImageLayerGzip,
						Size:      23423,
						Digest:    "sha256:ff1b4a27562d8ffc821b4d7368818ad7c759cfc2068b7adf0d2712315d67359a",
					},
				},
			},
			expectValid:          true,
			expectedArtifactType: "application/vnd.example.sbom",
		},
		"configless_artifact": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				ArtifactType: "application/vnd.example.catgif",
				Config:       emptyJsonDescriptor,
				Layers: []distribution.Descriptor{
					{
						MediaType: "image/gif",
						Size:      23423,
						Digest:    "sha256:ff1b4a27562d8ffc821b4d7368818ad7c759cfc2068b7adf0d2712315d67359a",
					},
				},
			},
			expectValid:          true,
			expectedArtifactType: "application/vnd.example.catgif",
		},
		"invalid_artifact": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				Config: emptyJsonDescriptor, // This MUST have an artifactType
				Layers: []distribution.Descriptor{
					{
						MediaType: "image/gif",
						Size:      23423,
						Digest:    "sha256:ff1b4a27562d8ffc821b4d7368818ad7c759cfc2068b7adf0d2712315d67359a",
					},
				},
			},
			expectValid: false,
		},
		"annotation_artifact": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2.,
				},
				ArtifactType: "application/vnd.example.comment",
				Config:       emptyJsonDescriptor,
				Layers: []distribution.Descriptor{
					emptyJsonDescriptor,
				},
				Annotations: map[string]string{
					"com.example.data": "payload",
				},
			},
			expectValid:          true,
			expectedArtifactType: "application/vnd.example.comment",
		},
		"valid_subject": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				ArtifactType: "application/vnd.example.comment",
				Config:       emptyJsonDescriptor,
				Layers: []distribution.Descriptor{
					emptyJsonDescriptor,
				},
				Subject: &distribution.Descriptor{
					MediaType: v1.MediaTypeImageManifest,
					Size:      365,
					Digest:    "sha256:05b3abf2579a5eb66403cd78be557fd860633a1fe2103c7642030defe32c657f",
				},
				Annotations: map[string]string{
					"com.example.data": "payload",
				},
			},
			expectValid:          true,
			expectedArtifactType: "application/vnd.example.comment",
		},
		"invalid_subject": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				ArtifactType: "application/vnd.example.comment",
				Config:       emptyJsonDescriptor,
				Layers: []distribution.Descriptor{
					emptyJsonDescriptor,
				},
				Subject: &distribution.Descriptor{
					MediaType: v1.MediaTypeImageLayerGzip, // The subject is a manifest
					Size:      365,
					Digest:    "sha256:05b3abf2579a5eb66403cd78be557fd860633a1fe2103c7642030defe32c657f",
				},
				Annotations: map[string]string{
					"com.example.data": "payload",
				},
			},
			expectValid: false,
		},
		"docker_manifest_valid_as_subject": {
			manifest: Manifest{
				Versioned: specs.Versioned{
					SchemaVersion: 2,
				},
				ArtifactType: "application/vnd.example.comment",
				Config:       emptyJsonDescriptor,
				Layers: []distribution.Descriptor{
					emptyJsonDescriptor,
				},
				Subject: &distribution.Descriptor{
					MediaType: schema2.MediaTypeManifest,
					Size:      365,
					Digest:    "sha256:05b3abf2579a5eb66403cd78be557fd860633a1fe2103c7642030defe32c657f",
				},
				Annotations: map[string]string{
					"com.example.data": "payload",
				},
			},
			expectValid:          true,
			expectedArtifactType: "application/vnd.example.comment",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dm, err := FromStruct(test.manifest)
			if err != nil {
				t.Fatalf("Error making DeserializedManifest from struct: %s", err)
			}
			m, _, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, dm.canonical)
			if test.expectValid != (nil == err) {
				t.Fatalf("expectValid=%t but got err=%v", test.expectValid, err)
			}
			if err != nil {
				return
			}
			if artifactType := m.(distribution.Referrer).Type(); artifactType != test.expectedArtifactType {
				t.Errorf("Expected artifactType to be %q but got %q", test.expectedArtifactType, artifactType)
			}
		})
	}
}

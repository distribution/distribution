package ocischema

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/distribution/distribution/v3/manifest/manifestlist"

	"github.com/opencontainers/go-digest"
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

func makeTestManifest(mediaType string) Manifest {
	return Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     mediaType,
		},
		Config: distribution.Descriptor{
			MediaType:   v1.MediaTypeImageConfig,
			Digest:      "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:        985,
			Annotations: map[string]string{"apple": "orange"},
		},
		Layers: []distribution.Descriptor{
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
		t.Fatalf("manifest index annotation length should be 1")
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
				t.Fatalf("bad content type should have produced error")
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
		Config: distribution.Descriptor{Size: 1},
		Layers: []distribution.Descriptor{{Size: 2}},
	}
	index := manifestlist.ManifestList{
		Manifests: []manifestlist.ManifestDescriptor{
			{Descriptor: distribution.Descriptor{Size: 3}},
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

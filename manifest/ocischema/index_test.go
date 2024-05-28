package ocischema

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const expectedOCIImageIndexSerialization = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.oci.image.index.v1+json",
   "manifests": [
      {
         "mediaType": "application/vnd.oci.image.manifest.v1+json",
         "digest": "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
         "size": 985,
         "platform": {
            "architecture": "amd64",
            "os": "linux"
         }
      },
      {
         "mediaType": "application/vnd.oci.image.manifest.v1+json",
         "digest": "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
         "size": 985,
         "annotations": {
            "platform": "none"
         }
      },
      {
         "mediaType": "application/vnd.oci.image.manifest.v1+json",
         "digest": "sha256:6346340964309634683409684360934680934608934608934608934068934608",
         "size": 2392,
         "annotations": {
            "what": "for"
         },
         "platform": {
            "architecture": "sun4m",
            "os": "sunos"
         }
      }
   ],
   "annotations": {
      "com.example.favourite-colour": "blue",
      "com.example.locale": "en_GB"
   }
}`

func makeTestOCIImageIndex(t *testing.T, mediaType string) ([]distribution.Descriptor, *DeserializedImageIndex) {
	manifestDescriptors := []distribution.Descriptor{
		{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:      985,
			Platform: &v1.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
		},
		{
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			Digest:      "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:        985,
			Annotations: map[string]string{"platform": "none"},
		},
		{
			MediaType:   "application/vnd.oci.image.manifest.v1+json",
			Digest:      "sha256:6346340964309634683409684360934680934608934608934608934068934608",
			Size:        2392,
			Annotations: map[string]string{"what": "for"},
			Platform: &v1.Platform{
				Architecture: "sun4m",
				OS:           "sunos",
			},
		},
	}
	annotations := map[string]string{
		"com.example.favourite-colour": "blue",
		"com.example.locale":           "en_GB",
	}

	deserialized, err := fromDescriptorsWithMediaType(manifestDescriptors, annotations, mediaType)
	if err != nil {
		t.Fatalf("error creating DeserializedManifestList: %v", err)
	}

	return manifestDescriptors, deserialized
}

func TestOCIImageIndex(t *testing.T) {
	manifestDescriptors, deserialized := makeTestOCIImageIndex(t, v1.MediaTypeImageIndex)

	mediaType, canonical, _ := deserialized.Payload()

	if mediaType != v1.MediaTypeImageIndex {
		t.Fatalf("unexpected media type: %s", mediaType)
	}

	// Check that the canonical field is the same as json.MarshalIndent
	// with these parameters.
	expected, err := json.MarshalIndent(&deserialized.ImageIndex, "", "   ")
	if err != nil {
		t.Fatalf("error marshaling manifest list: %v", err)
	}
	if !bytes.Equal(expected, canonical) {
		t.Fatalf("manifest bytes not equal:\nexpected:\n%s\nactual:\n%s\n", string(expected), string(canonical))
	}

	// Check that the canonical field has the expected value.
	if !bytes.Equal([]byte(expectedOCIImageIndexSerialization), canonical) {
		t.Fatalf("manifest bytes not equal:\nexpected:\n%s\nactual:\n%s\n", expectedOCIImageIndexSerialization, string(canonical))
	}

	var unmarshalled DeserializedImageIndex
	if err := json.Unmarshal(deserialized.canonical, &unmarshalled); err != nil {
		t.Fatalf("error unmarshaling manifest: %v", err)
	}

	if !reflect.DeepEqual(&unmarshalled, deserialized) {
		t.Fatalf("manifests are different after unmarshaling: %v != %v", unmarshalled, *deserialized)
	}

	references := deserialized.References()
	if len(references) != 3 {
		t.Fatalf("unexpected number of references: %d", len(references))
	}
	if !reflect.DeepEqual(references, manifestDescriptors) {
		t.Errorf("expected references:\n%v\nbut got:\n%v", references, manifestDescriptors)
	}
}

func TestOCIManifestIndexUnmarshal(t *testing.T) {
	_, descriptor, err := distribution.UnmarshalManifest(v1.MediaTypeImageIndex, []byte(expectedOCIImageIndexSerialization))
	if err != nil {
		t.Fatalf("unmarshal manifest index failed: %v", err)
	}
	_, deserialized := makeTestOCIImageIndex(t, v1.MediaTypeImageIndex)

	if !reflect.DeepEqual(descriptor.Annotations, deserialized.Annotations) {
		t.Fatalf("manifest index annotation not equal:\nexpected:\n%v\nactual:\n%v\n", deserialized.Annotations, descriptor.Annotations)
	}
	if len(descriptor.Annotations) != 2 {
		t.Fatalf("manifest index annotation length should be 2")
	}
	if descriptor.Size != int64(len([]byte(expectedOCIImageIndexSerialization))) {
		t.Fatalf("manifest index size is not correct:\nexpected:\n%d\nactual:\n%v\n", int64(len([]byte(expectedOCIImageIndexSerialization))), descriptor.Size)
	}
	if descriptor.Digest.String() != digest.FromBytes([]byte(expectedOCIImageIndexSerialization)).String() {
		t.Fatalf("manifest index digest is not correct:\nexpected:\n%s\nactual:\n%s\n", digest.FromBytes([]byte(expectedOCIImageIndexSerialization)), descriptor.Digest)
	}
	if descriptor.MediaType != v1.MediaTypeImageIndex {
		t.Fatalf("manifest index media type is not correct:\nexpected:\n%s\nactual:\n%s\n", v1.MediaTypeImageManifest, descriptor.MediaType)
	}
}

func indexMediaTypeTest(contentType string, mediaType string, shouldError bool) func(*testing.T) {
	return func(t *testing.T) {
		var m *DeserializedImageIndex
		_, m = makeTestOCIImageIndex(t, mediaType)

		_, canonical, err := m.Payload()
		if err != nil {
			t.Fatalf("error getting payload, %v", err)
		}

		unmarshalled, descriptor, err := distribution.UnmarshalManifest(
			contentType,
			canonical)

		if shouldError {
			if err == nil {
				t.Fatalf("bad content type should have produced error")
			}
		} else {
			if err != nil {
				t.Fatalf("error unmarshaling manifest, %v", err)
			}

			asManifest := unmarshalled.(*DeserializedImageIndex)
			if asManifest.MediaType != mediaType {
				t.Fatalf("Bad media type '%v' as unmarshalled", asManifest.MediaType)
			}

			if descriptor.MediaType != contentType {
				t.Fatalf("Bad media type '%v' for descriptor", descriptor.MediaType)
			}

			unmarshalledMediaType, _, _ := unmarshalled.Payload()
			if unmarshalledMediaType != contentType {
				t.Fatalf("Bad media type '%v' for payload", unmarshalledMediaType)
			}
		}
	}
}

func TestIndexMediaTypes(t *testing.T) {
	t.Run("No_MediaType", indexMediaTypeTest(v1.MediaTypeImageIndex, "", false))
	t.Run("ImageIndex", indexMediaTypeTest(v1.MediaTypeImageIndex, v1.MediaTypeImageIndex, false))
	t.Run("Bad_MediaType", indexMediaTypeTest(v1.MediaTypeImageIndex, v1.MediaTypeImageIndex+"XXX", true))
}

func TestValidateIndex(t *testing.T) {
	manifest := schema2.Manifest{
		Config: distribution.Descriptor{Size: 1},
		Layers: []distribution.Descriptor{{Size: 2}},
	}
	index := ImageIndex{
		Manifests: []distribution.Descriptor{{Size: 3}},
	}
	t.Run("valid", func(t *testing.T) {
		b, err := json.Marshal(index)
		if err != nil {
			t.Fatal("unexpected error marshaling index", err)
		}
		if err := validateIndex(b); err != nil {
			t.Error("index should be valid", err)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		b, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal("unexpected error marshaling manifest", err)
		}
		if err := validateIndex(b); err == nil {
			t.Error("manifest should not be valid")
		}
	})
}

package schema2

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest"
)

const expectedManifestSerialization = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
   "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "digest": "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
      "size": 985
   },
   "layers": [
      {
         "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
         "digest": "sha256:62d8908bee94c202b2d35224a221aaa2058318bfa9879fa541efaecba272331b",
         "size": 153263
      }
   ]
}`

func makeTestManifest(mediaType string) Manifest {
	return Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     mediaType,
		},
		Config: distribution.Descriptor{
			MediaType: MediaTypeImageConfig,
			Digest:    "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b",
			Size:      985,
		},
		Layers: []distribution.Descriptor{
			{
				MediaType: MediaTypeLayer,
				Digest:    "sha256:62d8908bee94c202b2d35224a221aaa2058318bfa9879fa541efaecba272331b",
				Size:      153263,
			},
		},
	}
}

func TestManifest(t *testing.T) {
	mfst := makeTestManifest(MediaTypeManifest)

	deserialized, err := FromStruct(mfst)
	if err != nil {
		t.Fatalf("error creating DeserializedManifest: %v", err)
	}

	mediaType, canonical, _ := deserialized.Payload()

	if mediaType != MediaTypeManifest {
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

	target := deserialized.Target()
	if target.Digest != "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b" {
		t.Fatalf("unexpected digest in target: %s", target.Digest.String())
	}
	if target.MediaType != MediaTypeImageConfig {
		t.Fatalf("unexpected media type in target: %s", target.MediaType)
	}
	if target.Size != 985 {
		t.Fatalf("unexpected size in target: %d", target.Size)
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
	if references[1].MediaType != MediaTypeLayer {
		t.Fatalf("unexpected media type in reference: %s", references[0].MediaType)
	}
	if references[1].Size != 153263 {
		t.Fatalf("unexpected size in reference: %d", references[0].Size)
	}
}

func mediaTypeTest(t *testing.T, mediaType string, shouldError bool) {
	mfst := makeTestManifest(mediaType)

	deserialized, err := FromStruct(mfst)
	if err != nil {
		t.Fatalf("error creating DeserializedManifest: %v", err)
	}

	unmarshalled, descriptor, err := distribution.UnmarshalManifest(
		MediaTypeManifest,
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

		if descriptor.MediaType != MediaTypeManifest {
			t.Fatalf("Bad media type '%v' for descriptor", descriptor.MediaType)
		}

		unmarshalledMediaType, _, _ := unmarshalled.Payload()
		if unmarshalledMediaType != MediaTypeManifest {
			t.Fatalf("Bad media type '%v' for payload", unmarshalledMediaType)
		}
	}
}

func TestMediaTypes(t *testing.T) {
	mediaTypeTest(t, "", true)
	mediaTypeTest(t, MediaTypeManifest, false)
	mediaTypeTest(t, MediaTypeManifest+"XXX", true)
}

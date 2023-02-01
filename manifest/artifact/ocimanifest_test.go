package artifact_test

import (
	"reflect"
	"testing"

	"github.com/distribution/distribution/v3"
	artifact "github.com/distribution/distribution/v3/manifest/artifact"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestArtifactFunc(t *testing.T) {
	for name, test := range map[string]struct {
		rawManifest        []byte
		expectError        bool
		expectedDescriptor distribution.Descriptor
		expectedManifest   distribution.Manifest
	}{
		"valid_artifact": {
			rawManifest:        artifact.ManifestBytes,
			expectedDescriptor: artifact.ManifestDescriptor,
			expectedManifest:   artifact.ManifestDeserialized,
		},
		"artifact_must_have_mediaType": {
			rawManifest: artifact.ManifestNoMediaType,
			expectError: true,
		},
		"artifact_can_have_no_subject": {
			rawManifest:        artifact.ManifestNoSubjectBytes,
			expectedDescriptor: artifact.ManifestNoSubjectDescriptor,
			expectedManifest:   artifact.ManifestNoSubjectDeserialized,
		},
		"artifact_subject_must_be_manifest": {
			rawManifest: artifact.ManifestBlobSubjectBytes,
			expectError: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			test := test
			t.Parallel()

			manifest, descriptor, err := distribution.UnmarshalManifest(v1.MediaTypeArtifactManifest, test.rawManifest)

			if err != nil != test.expectError {
				t.Fatalf("Unexpected error value: %s, expected error=%t", err, test.expectError)
			}
			if !reflect.DeepEqual(descriptor, test.expectedDescriptor) {
				t.Errorf("Descriptor incorrect:\n%v\nexpected:\n%v", descriptor, test.expectedDescriptor)
			}
			if !reflect.DeepEqual(manifest, test.expectedManifest) {
				t.Errorf("Manifest incorrect:\n%v\nexpected:\n%v", manifest, test.expectedManifest)
			}
		})
	}
}

func TestPayload(t *testing.T) {
	t.Parallel()

	manifest, _, err := distribution.UnmarshalManifest(v1.MediaTypeArtifactManifest, artifact.ManifestBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal manifest: %s", err)
	}

	mediaType, payload, err := manifest.Payload()

	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if mediaType != v1.MediaTypeArtifactManifest {
		t.Errorf("Unexpected mediaType %q, should be %q", mediaType, v1.MediaTypeArtifactManifest)
	}
	if !reflect.DeepEqual(payload, artifact.ManifestBytes) {
		t.Errorf("Unexpected payload, should exactly match inputted manifest.")
	}
}

func TestReferences(t *testing.T) {
	// References only returns the blobs of this artifact and not also it's
	// subject because the subject does not have to exist when the artifact is
	// pushed. Unlike when an image index returns the references image manifests
	// because those do have to exist before the image index is pushed.
	t.Parallel()

	manifest, _, err := distribution.UnmarshalManifest(v1.MediaTypeArtifactManifest, artifact.ManifestBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal manifest: %s", err)
	}

	references := manifest.References()

	if !reflect.DeepEqual(references, artifact.ManifestDeserialized.Blobs) {
		t.Errorf("Unexpected references:\n%v\nexpected:\n%v", references, artifact.ManifestDeserialized.Blobs)
	}
}

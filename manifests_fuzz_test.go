package distribution_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/opencontainers/go-digest"

	"github.com/docker/distribution"

	// Registering the schemas is what makes UnmarshalManifest dispatch to them,
	// exactly as cmd/registry does.
	_ "github.com/docker/distribution/manifest/manifestlist"
	_ "github.com/docker/distribution/manifest/ocischema"
	_ "github.com/docker/distribution/manifest/schema1"
	_ "github.com/docker/distribution/manifest/schema2"
)

// FuzzUnmarshalManifest drives the entry point the registry uses for every
// manifest PUT: the Content-Type header and the raw request body.
//
// Threat model coverage (registry-threat-model.md), harness 3: structural
// fuzzing of manifest parsing. The registry is content-addressed, so the
// property that matters is not merely "does not crash" but that the descriptor
// the parser hands back addresses the exact bytes that arrived. A descriptor
// whose digest or size describes something other than the payload, or a
// Payload() that returns bytes other than the ones digested, means the registry
// stores content under a digest that does not describe it -- the client asks
// for one image and gets another.
func FuzzUnmarshalManifest(f *testing.F) {
	seeds := []struct {
		contentType string
		payload     string
	}{
		{"", ""},
		{"", "{}"},
		{"application/vnd.docker.distribution.manifest.v2+json", `{}`},
		{
			"application/vnd.docker.distribution.manifest.v2+json",
			`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json",` +
				`"config":{"mediaType":"application/vnd.docker.container.image.v1+json","size":1,` +
				`"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"},` +
				`"layers":[{"mediaType":"application/vnd.docker.image.rootfs.diff.tar.gzip","size":2,` +
				`"digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111"}]}`,
		},
		{
			"application/vnd.oci.image.manifest.v1+json",
			`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json",` +
				`"config":{"mediaType":"application/vnd.oci.image.config.v1+json","size":1,` +
				`"digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"},` +
				`"layers":[]}`,
		},
		{
			"application/vnd.docker.distribution.manifest.list.v2+json",
			`{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.list.v2+json","manifests":[]}`,
		},
		{
			"application/vnd.oci.image.index.v1+json",
			`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[]}`,
		},
		// Media type parsing: parameters, casing and padding are all handled by
		// mime.ParseMediaType before the dispatch table is consulted.
		{"application/vnd.docker.distribution.manifest.v2+json; charset=utf-8", `{"schemaVersion":2}`},
		{"APPLICATION/VND.DOCKER.DISTRIBUTION.MANIFEST.V2+JSON", `{"schemaVersion":2}`},
		{"application/vnd.docker.distribution.manifest.v2+json ;;", `{"schemaVersion":2}`},
		{"application/octet-stream", `{"schemaVersion":2}`},
		{"not a media type", `{}`},
		// Payloads that stress the JSON decoder rather than the schema.
		{"application/vnd.docker.distribution.manifest.v2+json", `{"schemaVersion":2,"mediaType":null}`},
		{"application/vnd.docker.distribution.manifest.v2+json", `[]`},
		{"application/vnd.docker.distribution.manifest.v2+json", `null`},
		{"application/vnd.docker.distribution.manifest.v2+json", "\x00\x01\x02"},
		{"application/vnd.docker.distribution.manifest.v2+json", `{"layers":[{"size":-1}]}`},
		{"application/vnd.docker.distribution.manifest.v2+json", `{"layers":[{"size":9223372036854775807}]}`},
		// Duplicate keys: encoding/json keeps the last, so the struct and the
		// canonical bytes can disagree about what the manifest says.
		{
			"application/vnd.docker.distribution.manifest.v2+json",
			`{"mediaType":"application/vnd.docker.distribution.manifest.v2+json",` +
				`"mediaType":"application/vnd.oci.image.manifest.v1+json"}`,
		},
	}
	for _, seed := range seeds {
		f.Add(seed.contentType, []byte(seed.payload))
	}

	f.Fuzz(func(t *testing.T, contentType string, payload []byte) {
		manifest, descriptor, err := distribution.UnmarshalManifest(contentType, payload)
		if err != nil {
			// Rejected: nothing is stored.
			return
		}
		if manifest == nil {
			t.Fatalf("UnmarshalManifest returned a nil manifest and a nil error for %q", contentType)
		}

		// The registry addresses the manifest by this digest, so it must describe
		// the bytes that arrived and nothing else.
		if want := digest.FromBytes(payload); descriptor.Digest != want {
			t.Fatalf("descriptor digest %q does not describe the payload (want %q)",
				descriptor.Digest, want)
		}
		if descriptor.Size != int64(len(payload)) {
			t.Fatalf("descriptor size %d does not match the payload length %d",
				descriptor.Size, len(payload))
		}

		// Payload() is what gets written to storage; it must be the same bytes the
		// digest was taken over.
		mediaType, canonical, err := manifest.Payload()
		if err != nil {
			t.Fatalf("Payload failed on an accepted manifest: %v", err)
		}
		if !bytes.Equal(canonical, payload) {
			t.Fatalf("Payload returned %d bytes, the accepted payload was %d bytes; "+
				"storage and digest would disagree", len(canonical), len(payload))
		}
		if mediaType != descriptor.MediaType {
			t.Fatalf("Payload media type %q disagrees with the descriptor media type %q",
				mediaType, descriptor.MediaType)
		}

		// References() must not panic, and it is enumerated here to keep that on the
		// fuzzed path. Their contents are deliberately not asserted: schema2's
		// References always includes m.Config, so a manifest without one yields a
		// zero descriptor, and the parser is not the layer that rejects it --
		// verifyManifest in registry/storage resolves every reference against the
		// blob store. That rejection is asserted by FuzzBlobUploadSession, which
		// drives the real PUT path.
		for range manifest.References() {
		}

		// Re-parsing the canonical bytes must yield the same descriptor: the
		// registry re-reads manifests from storage and must resolve them identically.
		again, againDescriptor, err := distribution.UnmarshalManifest(mediaType, canonical)
		if err != nil {
			t.Fatalf("re-parsing the canonical payload failed: %v", err)
		}
		if !reflect.DeepEqual(againDescriptor, descriptor) {
			t.Fatalf("re-parsing produced a different descriptor: %+v then %+v",
				descriptor, againDescriptor)
		}
		if _, againCanonical, err := again.Payload(); err != nil {
			t.Fatalf("Payload failed after re-parsing: %v", err)
		} else if !bytes.Equal(againCanonical, canonical) {
			t.Fatalf("re-parsing changed the canonical bytes")
		}
	})
}

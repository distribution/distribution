package notifications

import (
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest"
	"github.com/distribution/distribution/v3/manifest/schema2"
	v2 "github.com/distribution/distribution/v3/registry/api/v2"
	"github.com/distribution/reference"
	events "github.com/docker/go-events"
	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var (
	// common environment for expected manifest events.

	repo   = "test/repo"
	source = SourceRecord{
		Addr:       "remote.test",
		InstanceID: uuid.NewString(),
	}
	ub = mustUB(v2.NewURLBuilderFromString("http://test.example.com/", false))

	actor = ActorRecord{
		Name: "test",
	}
	request      = RequestRecord{}
	tag          = "latest"
	ociMediaType = v1.MediaTypeImageManifest
	artifactType = "application/vnd.example.sbom.v1"
	cfg          = distribution.Descriptor{
		MediaType: artifactType,
		Size:      100,
		Digest:    "cfgdgst",
	}

	sm      *schema2.DeserializedManifest
	payload []byte
	dgst    digest.Digest
)

func TestEventBridgeManifestPulled(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(event events.Event) error {
		checkCommonManifest(t, EventActionPull, event)

		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.ManifestPulled(repoRef, sm); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func TestEventBridgeManifestPushed(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(event events.Event) error {
		checkCommonManifest(t, EventActionPush, event)

		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.ManifestPushed(repoRef, sm); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func TestEventBridgeManifestPushedWithTag(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(event events.Event) error {
		checkCommonManifest(t, EventActionPush, event)
		if event.(Event).Target.Tag != "latest" {
			t.Fatalf("missing or unexpected tag: %#v", event.(Event).Target)
		}

		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.ManifestPushed(repoRef, sm, distribution.WithTag(tag)); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func TestEventBridgeManifestPulledWithTag(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(event events.Event) error {
		checkCommonManifest(t, EventActionPull, event)
		if event.(Event).Target.Tag != "latest" {
			t.Fatalf("missing or unexpected tag: %#v", event.(Event).Target)
		}

		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.ManifestPulled(repoRef, sm, distribution.WithTag(tag)); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func TestEventBridgeManifestDeleted(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(event events.Event) error {
		checkDeleted(t, EventActionDelete, event)
		if event.(Event).Target.Digest != dgst {
			t.Fatalf("unexpected digest on event target: %q != %q", event.(Event).Target.Digest, dgst)
		}
		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.ManifestDeleted(repoRef, dgst); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func TestEventBridgeTagDeleted(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(event events.Event) error {
		checkDeleted(t, EventActionDelete, event)
		if event.(Event).Target.Tag != tag {
			t.Fatalf("unexpected tag on event target: %q != %q", event.(Event).Target.Tag, tag)
		}
		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.TagDeleted(repoRef, tag); err != nil {
		t.Fatalf("unexpected error notifying tag deletion: %v", err)
	}
}

func TestEventBridgeRepoDeleted(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(event events.Event) error {
		checkDeleted(t, EventActionDelete, event)
		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.RepoDeleted(repoRef); err != nil {
		t.Fatalf("unexpected error notifying repo deletion: %v", err)
	}
}

func createTestEnv(t *testing.T, fn testSinkFn) Listener {
	manifest := schema2.Manifest{
		Versioned: manifest.Versioned{
			MediaType: ociMediaType,
		},
		Config: cfg,
	}

	deserializedManifest, err := schema2.FromStruct(manifest)
	if err != nil {
		t.Fatalf("creating OCI manifest: %v", err)
	}

	_, payload, _ = deserializedManifest.Payload()
	dgst = digest.FromBytes(payload)
	sm = deserializedManifest

	return NewBridge(ub, source, actor, request, fn, true)
}

func checkDeleted(t *testing.T, action string, event events.Event) {
	if event.(Event).Source != source {
		t.Fatalf("source not equal: %#v != %#v", event.(Event).Source, source)
	}

	if event.(Event).Request != request {
		t.Fatalf("request not equal: %#v != %#v", event.(Event).Request, request)
	}

	if event.(Event).Actor != actor {
		t.Fatalf("request not equal: %#v != %#v", event.(Event).Actor, actor)
	}

	if event.(Event).Target.Repository != repo {
		t.Fatalf("unexpected repository: %q != %q", event.(Event).Target.Repository, repo)
	}
}

func checkCommonManifest(t *testing.T, action string, event events.Event) {
	checkCommon(t, event)

	if event.(Event).Action != action {
		t.Fatalf("unexpected event action: %q != %q", event.(Event).Action, action)
	}

	repoRef, _ := reference.WithName(repo)
	ref, _ := reference.WithDigest(repoRef, dgst)
	u, err := ub.BuildManifestURL(ref)
	if err != nil {
		t.Fatalf("error building expected url: %v", err)
	}

	if event.(Event).Target.URL != u {
		t.Fatalf("incorrect url passed: \n%q != \n%q", event.(Event).Target.URL, u)
	}
}

func checkCommon(t *testing.T, event events.Event) {
	if event.(Event).Source != source {
		t.Fatalf("source not equal: %#v != %#v", event.(Event).Source, source)
	}

	if event.(Event).Request != request {
		t.Fatalf("request not equal: %#v != %#v", event.(Event).Request, request)
	}

	if event.(Event).Actor != actor {
		t.Fatalf("request not equal: %#v != %#v", event.(Event).Actor, actor)
	}

	if event.(Event).Target.Digest != dgst {
		t.Fatalf("unexpected digest on event target: %q != %q", event.(Event).Target.Digest, dgst)
	}

	if event.(Event).Target.Length != int64(len(payload)) {
		t.Fatalf("unexpected target length: %v != %v", event.(Event).Target.Length, len(payload))
	}

	if event.(Event).Target.Repository != repo {
		t.Fatalf("unexpected repository: %q != %q", event.(Event).Target.Repository, repo)
	}
}

type testSinkFn func(event events.Event) error

func (tsf testSinkFn) Write(event events.Event) error {
	return tsf(event)
}

func (tsf testSinkFn) Close() error { return nil }

func mustUB(ub *v2.URLBuilder, err error) *v2.URLBuilder {
	if err != nil {
		panic(err)
	}

	return ub
}

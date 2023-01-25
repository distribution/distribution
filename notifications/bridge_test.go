package notifications

import (
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema1"
	"github.com/docker/distribution/reference"
	v2 "github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/uuid"
	events "github.com/docker/go-events"
	"github.com/docker/libtrust"
	"github.com/opencontainers/go-digest"
)

var (
	// common environment for expected manifest events.

	repo   = "test/repo"
	source = SourceRecord{
		Addr:       "remote.test",
		InstanceID: uuid.Generate().String(),
	}
	ub = mustUB(v2.NewURLBuilderFromString("http://test.example.com/", false))

	actor = ActorRecord{
		Name: "test",
	}
	request = RequestRecord{}
	layers  = []schema1.FSLayer{
		{
			BlobSum: "asdf",
		},
		{
			BlobSum: "qwer",
		},
	}
	m = schema1.Manifest{
		Name:     repo,
		Tag:      "latest",
		FSLayers: layers,
	}

	sm      *schema1.SignedManifest
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
	if err := l.ManifestPushed(repoRef, sm, distribution.WithTag(m.Tag)); err != nil {
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
	if err := l.ManifestPulled(repoRef, sm, distribution.WithTag(m.Tag)); err != nil {
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
		if event.(Event).Target.Tag != m.Tag {
			t.Fatalf("unexpected tag on event target: %q != %q", event.(Event).Target.Tag, m.Tag)
		}
		return nil
	}))

	repoRef, _ := reference.WithName(repo)
	if err := l.TagDeleted(repoRef, m.Tag); err != nil {
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
	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("error generating private key: %v", err)
	}

	sm, err = schema1.Sign(&m, pk)
	if err != nil {
		t.Fatalf("error signing manifest: %v", err)
	}

	payload = sm.Canonical
	dgst = digest.FromBytes(payload)

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

	if len(event.(Event).Target.References) != len(layers) {
		t.Fatalf("unexpected number of references %v != %v", len(event.(Event).Target.References), len(layers))
	}
	for i, targetReference := range event.(Event).Target.References {
		if targetReference.Digest != layers[i].BlobSum {
			t.Fatalf("unexpected reference: %q != %q", targetReference.Digest, layers[i].BlobSum)
		}
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

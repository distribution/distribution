package notifications

import (
	"testing"

	"github.com/docker/distribution/digest"

	"github.com/docker/libtrust"

	"github.com/docker/distribution/manifest"

	"github.com/docker/distribution/registry/api/v2"
	"github.com/docker/distribution/uuid"
)

var (
	// common environment for expected manifest events.

	repo   = "test/repo"
	source = SourceRecord{
		Addr:       "remote.test",
		InstanceID: uuid.Generate().String(),
	}
	ub = mustUB(v2.NewURLBuilderFromString("http://test.example.com/"))

	actor = ActorRecord{
		Name: "test",
	}
	request = RequestRecord{}
	m       = manifest.Manifest{
		Name: repo,
		Tag:  "latest",
	}

	sm      *manifest.SignedManifest
	payload []byte
	dgst    digest.Digest
)

func TestEventBridgeManifestPulled(t *testing.T) {

	l := createTestEnv(t, testSinkFn(func(events ...Event) error {
		checkCommonManifest(t, EventActionPull, events...)

		return nil
	}))

	if err := l.ManifestPulled(repo, sm); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func TestEventBridgeManifestPushed(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(events ...Event) error {
		checkCommonManifest(t, EventActionPush, events...)

		return nil
	}))

	if err := l.ManifestPushed(repo, sm); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func TestEventBridgeManifestDeleted(t *testing.T) {
	l := createTestEnv(t, testSinkFn(func(events ...Event) error {
		checkCommonManifest(t, EventActionDelete, events...)

		return nil
	}))

	if err := l.ManifestDeleted(repo, sm); err != nil {
		t.Fatalf("unexpected error notifying manifest pull: %v", err)
	}
}

func createTestEnv(t *testing.T, fn testSinkFn) Listener {
	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("error generating private key: %v", err)
	}

	sm, err = manifest.Sign(&m, pk)
	if err != nil {
		t.Fatalf("error signing manifest: %v", err)
	}

	payload, err = sm.Payload()
	if err != nil {
		t.Fatalf("error getting manifest payload: %v", err)
	}

	dgst, err = digest.FromBytes(payload)
	if err != nil {
		t.Fatalf("error digesting manifest payload: %v", err)
	}

	return NewBridge(ub, source, actor, request, fn)
}

func checkCommonManifest(t *testing.T, action string, events ...Event) {
	checkCommon(t, events...)

	event := events[0]
	if event.Action != action {
		t.Fatalf("unexpected event action: %q != %q", event.Action, action)
	}

	u, err := ub.BuildManifestURL(repo, dgst.String())
	if err != nil {
		t.Fatalf("error building expected url: %v", err)
	}

	if event.Target.URL != u {
		t.Fatalf("incorrect url passed: %q != %q", event.Target.URL, u)
	}
}

func checkCommon(t *testing.T, events ...Event) {
	if len(events) != 1 {
		t.Fatalf("unexpected number of events: %v != 1", len(events))
	}

	event := events[0]

	if event.Source != source {
		t.Fatalf("source not equal: %#v != %#v", event.Source, source)
	}

	if event.Request != request {
		t.Fatalf("request not equal: %#v != %#v", event.Request, request)
	}

	if event.Actor != actor {
		t.Fatalf("request not equal: %#v != %#v", event.Actor, actor)
	}

	if event.Target.Digest != dgst {
		t.Fatalf("unexpected digest on event target: %q != %q", event.Target.Digest, dgst)
	}

	if event.Target.Length != int64(len(payload)) {
		t.Fatalf("unexpected target length: %v != %v", event.Target.Length, len(payload))
	}

	if event.Target.Repository != repo {
		t.Fatalf("unexpected repository: %q != %q", event.Target.Repository, repo)
	}

}

type testSinkFn func(events ...Event) error

func (tsf testSinkFn) Write(events ...Event) error {
	return tsf(events...)
}

func (tsf testSinkFn) Close() error { return nil }

func mustUB(ub *v2.URLBuilder, err error) *v2.URLBuilder {
	if err != nil {
		panic(err)
	}

	return ub
}

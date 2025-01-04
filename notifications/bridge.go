package notifications

import (
	"net/http"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/internal/requestutil"
	"github.com/distribution/reference"
	events "github.com/docker/go-events"
	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type bridge struct {
	ub                URLBuilder
	includeReferences bool
	actor             ActorRecord
	source            SourceRecord
	request           RequestRecord
	sink              events.Sink
}

var _ Listener = &bridge{}

// URLBuilder defines a subset of url builder to be used by the event listener.
type URLBuilder interface {
	BuildManifestURL(name reference.Named) (string, error)
	BuildBlobURL(ref reference.Canonical) (string, error)
}

// NewBridge returns a notification listener that writes records to sink,
// using the actor and source. Any urls populated in the events created by
// this bridge will be created using the URLBuilder.
// TODO(stevvooe): Update this to simply take a context.Context object.
func NewBridge(ub URLBuilder, source SourceRecord, actor ActorRecord, request RequestRecord, sink events.Sink, includeReferences bool) Listener {
	return &bridge{
		ub:                ub,
		includeReferences: includeReferences,
		actor:             actor,
		source:            source,
		request:           request,
		sink:              sink,
	}
}

// NewRequestRecord builds a RequestRecord for use in NewBridge from an
// http.Request, associating it with a request id.
func NewRequestRecord(id string, r *http.Request) RequestRecord {
	return RequestRecord{
		ID:        id,
		Addr:      requestutil.RemoteAddr(r),
		Host:      r.Host,
		Method:    r.Method,
		UserAgent: r.UserAgent(),
	}
}

func (b *bridge) ManifestPushed(repo reference.Named, sm distribution.Manifest, options ...distribution.ManifestServiceOption) error {
	manifestEvent, err := b.createManifestEvent(EventActionPush, repo, sm)
	if err != nil {
		return err
	}

	for _, option := range options {
		if opt, ok := option.(distribution.WithTagOption); ok {
			manifestEvent.Target.Tag = opt.Tag
			break
		}
	}
	return b.sink.Write(*manifestEvent)
}

func (b *bridge) ManifestPulled(repo reference.Named, sm distribution.Manifest, options ...distribution.ManifestServiceOption) error {
	manifestEvent, err := b.createManifestEvent(EventActionPull, repo, sm)
	if err != nil {
		return err
	}

	for _, option := range options {
		if opt, ok := option.(distribution.WithTagOption); ok {
			manifestEvent.Target.Tag = opt.Tag
			break
		}
	}
	return b.sink.Write(*manifestEvent)
}

func (b *bridge) ManifestDeleted(repo reference.Named, dgst digest.Digest) error {
	return b.createManifestDeleteEventAndWrite(EventActionDelete, repo, dgst)
}

func (b *bridge) BlobPushed(repo reference.Named, desc v1.Descriptor) error {
	return b.createBlobEventAndWrite(EventActionPush, repo, desc)
}

func (b *bridge) BlobPulled(repo reference.Named, desc v1.Descriptor) error {
	return b.createBlobEventAndWrite(EventActionPull, repo, desc)
}

func (b *bridge) BlobMounted(repo reference.Named, desc v1.Descriptor, fromRepo reference.Named) error {
	event, err := b.createBlobEvent(EventActionMount, repo, desc)
	if err != nil {
		return err
	}
	event.Target.FromRepository = fromRepo.Name()
	return b.sink.Write(*event)
}

func (b *bridge) BlobDeleted(repo reference.Named, dgst digest.Digest) error {
	return b.createBlobDeleteEventAndWrite(EventActionDelete, repo, dgst)
}

func (b *bridge) TagDeleted(repo reference.Named, tag string) error {
	event := b.createEvent(EventActionDelete)
	event.Target.Repository = repo.Name()
	event.Target.Tag = tag

	return b.sink.Write(*event)
}

func (b *bridge) RepoDeleted(repo reference.Named) error {
	event := b.createEvent(EventActionDelete)
	event.Target.Repository = repo.Name()

	return b.sink.Write(*event)
}

func (b *bridge) createManifestDeleteEventAndWrite(action string, repo reference.Named, dgst digest.Digest) error {
	event := b.createEvent(action)
	event.Target.Repository = repo.Name()
	event.Target.Digest = dgst

	return b.sink.Write(*event)
}

func (b *bridge) createManifestEvent(action string, repo reference.Named, sm distribution.Manifest) (*Event, error) {
	event := b.createEvent(action)
	event.Target.Repository = repo.Name()

	mt, p, err := sm.Payload()
	if err != nil {
		return nil, err
	}

	// Ensure we have the canonical manifest descriptor here
	manifest, desc, err := distribution.UnmarshalManifest(mt, p)
	if err != nil {
		return nil, err
	}

	event.Target.MediaType = mt
	event.Target.Digest = desc.Digest
	event.Target.Size = desc.Size
	event.Target.Length = desc.Size
	if b.includeReferences {
		event.Target.References = append(event.Target.References, manifest.References()...)
	}

	ref, err := reference.WithDigest(repo, event.Target.Digest)
	if err != nil {
		return nil, err
	}

	event.Target.URL, err = b.ub.BuildManifestURL(ref)
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (b *bridge) createBlobDeleteEventAndWrite(action string, repo reference.Named, dgst digest.Digest) error {
	event := b.createEvent(action)
	event.Target.Digest = dgst
	event.Target.Repository = repo.Name()

	return b.sink.Write(*event)
}

func (b *bridge) createBlobEventAndWrite(action string, repo reference.Named, desc v1.Descriptor) error {
	event, err := b.createBlobEvent(action, repo, desc)
	if err != nil {
		return err
	}

	return b.sink.Write(*event)
}

func (b *bridge) createBlobEvent(action string, repo reference.Named, desc v1.Descriptor) (*Event, error) {
	event := b.createEvent(action)
	event.Target.Descriptor = desc
	event.Target.Length = desc.Size
	event.Target.Repository = repo.Name()

	ref, err := reference.WithDigest(repo, desc.Digest)
	if err != nil {
		return nil, err
	}

	event.Target.URL, err = b.ub.BuildBlobURL(ref)
	if err != nil {
		return nil, err
	}

	return event, nil
}

// createEvent creates an event with actor and source populated.
func (b *bridge) createEvent(action string) *Event {
	event := createEvent(action)
	event.Source = b.source
	event.Actor = b.actor
	event.Request = b.request

	return event
}

// createEvent returns a new event, timestamped, with the specified action.
func createEvent(action string) *Event {
	return &Event{
		ID:        uuid.NewString(),
		Timestamp: time.Now(),
		Action:    action,
	}
}

package notifications

import (
	"net/http"
	"time"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/uuid"
)

type bridge struct {
	ub      URLBuilder
	actor   ActorRecord
	source  SourceRecord
	request RequestRecord
	sink    Sink
}

var _ Listener = &bridge{}

// URLBuilder defines a subset of url builder to be used by the event listener.
type URLBuilder interface {
	BuildManifestURL(name, tag string) (string, error)
	BuildBlobURL(name string, dgst digest.Digest) (string, error)
}

// NewBridge returns a notification listener that writes records to sink,
// using the actor and source. Any urls populated in the events created by
// this bridge will be created using the URLBuilder.
// TODO(stevvooe): Update this to simply take a context.Context object.
func NewBridge(ub URLBuilder, source SourceRecord, actor ActorRecord, request RequestRecord, sink Sink) Listener {
	return &bridge{
		ub:      ub,
		actor:   actor,
		source:  source,
		request: request,
		sink:    sink,
	}
}

// NewRequestRecord builds a RequestRecord for use in NewBridge from an
// http.Request, associating it with a request id.
func NewRequestRecord(id string, r *http.Request) RequestRecord {
	return RequestRecord{
		ID:        id,
		Addr:      context.RemoteAddr(r),
		Host:      r.Host,
		Method:    r.Method,
		UserAgent: r.UserAgent(),
	}
}

func (b *bridge) ManifestPushed(repo string, sm *manifest.SignedManifest) error {
	return b.createManifestEventAndWrite(EventActionPush, repo, sm)
}

func (b *bridge) ManifestPulled(repo string, sm *manifest.SignedManifest) error {
	return b.createManifestEventAndWrite(EventActionPull, repo, sm)
}

func (b *bridge) ManifestDeleted(repo string, sm *manifest.SignedManifest) error {
	return b.createManifestEventAndWrite(EventActionDelete, repo, sm)
}

func (b *bridge) BlobPushed(repo string, desc distribution.Descriptor) error {
	return b.createBlobEventAndWrite(EventActionPush, repo, desc)
}

func (b *bridge) BlobPulled(repo string, desc distribution.Descriptor) error {
	return b.createBlobEventAndWrite(EventActionPull, repo, desc)
}

func (b *bridge) BlobDeleted(repo string, desc distribution.Descriptor) error {
	return b.createBlobEventAndWrite(EventActionDelete, repo, desc)
}

func (b *bridge) createManifestEventAndWrite(action string, repo string, sm *manifest.SignedManifest) error {
	manifestEvent, err := b.createManifestEvent(action, repo, sm)
	if err != nil {
		return err
	}

	return b.sink.Write(*manifestEvent)
}

func (b *bridge) createManifestEvent(action string, repo string, sm *manifest.SignedManifest) (*Event, error) {
	event := b.createEvent(action)
	event.Target.MediaType = manifest.ManifestMediaType
	event.Target.Repository = repo

	p, err := sm.Payload()
	if err != nil {
		return nil, err
	}

	event.Target.Length = int64(len(p))
	event.Target.Size = int64(len(p))
	event.Target.Digest, err = digest.FromBytes(p)
	if err != nil {
		return nil, err
	}

	event.Target.URL, err = b.ub.BuildManifestURL(sm.Name, event.Target.Digest.String())
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (b *bridge) createBlobEventAndWrite(action string, repo string, desc distribution.Descriptor) error {
	event, err := b.createBlobEvent(action, repo, desc)
	if err != nil {
		return err
	}

	return b.sink.Write(*event)
}

func (b *bridge) createBlobEvent(action string, repo string, desc distribution.Descriptor) (*Event, error) {
	event := b.createEvent(action)
	event.Target.Descriptor = desc
	event.Target.Length = desc.Size
	event.Target.Repository = repo

	var err error
	event.Target.URL, err = b.ub.BuildBlobURL(repo, desc.Digest)
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
		ID:        uuid.Generate().String(),
		Timestamp: time.Now(),
		Action:    action,
	}
}

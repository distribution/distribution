package notifications

import (
	"net/http"
	"time"

	"code.google.com/p/go-uuid/uuid"
	"github.com/docker/distribution"
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
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
		Addr:      r.RemoteAddr,
		Host:      r.Host,
		Method:    r.Method,
		UserAgent: r.UserAgent(),
	}
}

func (b *bridge) ManifestPushed(repo distribution.Repository, sm *manifest.SignedManifest) error {
	return b.createManifestEventAndWrite(EventActionPush, repo, sm)
}

func (b *bridge) ManifestPulled(repo distribution.Repository, sm *manifest.SignedManifest) error {
	return b.createManifestEventAndWrite(EventActionPull, repo, sm)
}

func (b *bridge) ManifestDeleted(repo distribution.Repository, sm *manifest.SignedManifest) error {
	return b.createManifestEventAndWrite(EventActionDelete, repo, sm)
}

func (b *bridge) LayerPushed(repo distribution.Repository, layer distribution.Layer) error {
	return b.createLayerEventAndWrite(EventActionPush, repo, layer.Digest())
}

func (b *bridge) LayerPulled(repo distribution.Repository, layer distribution.Layer) error {
	return b.createLayerEventAndWrite(EventActionPull, repo, layer.Digest())
}

func (b *bridge) LayerDeleted(repo distribution.Repository, layer distribution.Layer) error {
	return b.createLayerEventAndWrite(EventActionDelete, repo, layer.Digest())
}

func (b *bridge) createManifestEventAndWrite(action string, repo distribution.Repository, sm *manifest.SignedManifest) error {
	event, err := b.createManifestEvent(action, repo, sm)
	if err != nil {
		return err
	}

	return b.sink.Write(*event)
}

func (b *bridge) createManifestEvent(action string, repo distribution.Repository, sm *manifest.SignedManifest) (*Event, error) {
	event := b.createEvent(action)
	event.Target.Type = EventTargetTypeManifest
	event.Target.Name = repo.Name()
	event.Target.Tag = sm.Tag

	p, err := sm.Payload()
	if err != nil {
		return nil, err
	}

	event.Target.Digest, err = digest.FromBytes(p)
	if err != nil {
		return nil, err
	}

	// TODO(stevvooe): Currently, the is the "tag" url: once the digest url is
	// implemented, this should be replaced.
	event.Target.URL, err = b.ub.BuildManifestURL(sm.Name, sm.Tag)
	if err != nil {
		return nil, err
	}

	return event, nil
}

func (b *bridge) createLayerEventAndWrite(action string, repo distribution.Repository, dgst digest.Digest) error {
	event, err := b.createLayerEvent(action, repo, dgst)
	if err != nil {
		return err
	}

	return b.sink.Write(*event)
}

func (b *bridge) createLayerEvent(action string, repo distribution.Repository, dgst digest.Digest) (*Event, error) {
	event := b.createEvent(action)
	event.Target.Type = EventTargetTypeBlob
	event.Target.Name = repo.Name()
	event.Target.Digest = dgst

	var err error
	event.Target.URL, err = b.ub.BuildBlobURL(repo.Name(), dgst)
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
		ID:        uuid.New(),
		Timestamp: time.Now(),
		Action:    action,
	}
}

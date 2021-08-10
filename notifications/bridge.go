package notifications

import (
	"encoding/json"
	"fmt"
	"github.com/distribution/distribution/v3/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"net/http"
	"time"

	gocontext "context"
	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/uuid"
	events "github.com/docker/go-events"
	"github.com/opencontainers/go-digest"
)

const (
	ComponentName = "notifications"
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
		Addr:      context.RemoteAddr(r),
		Host:      r.Host,
		Method:    r.Method,
		UserAgent: r.UserAgent(),
	}
}

func (b *bridge) ManifestPushed(ctx gocontext.Context, repo reference.Named, sm distribution.Manifest, options ...distribution.ManifestServiceOption) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "ManifestPushed"),
		trace.WithAttributes(attribute.String("repo", repo.String())))
	defer tracing.StopSpan(span)

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

	addSpanEvent(span, manifestEvent)

	return b.sink.Write(*manifestEvent)
}

func (b *bridge) ManifestPulled(ctx gocontext.Context, repo reference.Named, sm distribution.Manifest, options ...distribution.ManifestServiceOption) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "ManifestPulled"),
		trace.WithAttributes(attribute.String("repo", repo.String())))
	defer tracing.StopSpan(span)

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

	addSpanEvent(span, manifestEvent)

	return b.sink.Write(*manifestEvent)
}

func (b *bridge) ManifestDeleted(ctx gocontext.Context, repo reference.Named, dgst digest.Digest) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "ManifestDeleted"),
		trace.WithAttributes(attribute.String("repo", repo.String()),
			attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	return b.createManifestDeleteEventAndWrite(EventActionDelete, repo, dgst, span)
}

func (b *bridge) BlobPushed(ctx gocontext.Context, repo reference.Named, desc distribution.Descriptor) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "BlobPushed"),
		trace.WithAttributes(attribute.String("repo", repo.String()),
			attribute.String("desc.digest", desc.Digest.String())))
	defer tracing.StopSpan(span)

	return b.createBlobEventAndWrite(EventActionPush, repo, desc, span)
}

func (b *bridge) BlobPulled(ctx gocontext.Context, repo reference.Named, desc distribution.Descriptor) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "BlobPulled"),
		trace.WithAttributes(attribute.String("repo", repo.String()),
			attribute.String("desc.digest", desc.Digest.String())))
	defer tracing.StopSpan(span)

	return b.createBlobEventAndWrite(EventActionPull, repo, desc, span)
}

func (b *bridge) BlobMounted(ctx gocontext.Context, repo reference.Named, desc distribution.Descriptor, fromRepo reference.Named) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "BlobMounted"),
		trace.WithAttributes(attribute.String("repo", repo.String()),
			attribute.String("desc.digest", desc.Digest.String())))
	defer tracing.StopSpan(span)

	event, err := b.createBlobEvent(EventActionMount, repo, desc)
	if err != nil {
		return err
	}
	event.Target.FromRepository = fromRepo.Name()

	addSpanEvent(span, event)

	return b.sink.Write(*event)
}

func (b *bridge) BlobDeleted(ctx gocontext.Context, repo reference.Named, dgst digest.Digest) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "BlobDeleted"),
		trace.WithAttributes(attribute.String("repo", repo.String()),
			attribute.String("dgst", dgst.String())))
	defer tracing.StopSpan(span)

	return b.createBlobDeleteEventAndWrite(EventActionDelete, repo, dgst, span)
}

func (b *bridge) TagDeleted(ctx gocontext.Context, repo reference.Named, tag string) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "TagDeleted"),
		trace.WithAttributes(attribute.String("repo", repo.String()),
			attribute.String("tag", tag)))
	defer tracing.StopSpan(span)

	event := b.createEvent(EventActionDelete)
	event.Target.Repository = repo.Name()
	event.Target.Tag = tag

	addSpanEvent(span, event)

	return b.sink.Write(*event)
}

func (b *bridge) RepoDeleted(ctx gocontext.Context, repo reference.Named) error {
	span, ctx := tracing.StartSpan(ctx, fmt.Sprintf("%s:%s", ComponentName, "RepoDeleted"),
		trace.WithAttributes(attribute.String("repo", repo.String())))
	defer tracing.StopSpan(span)

	event := b.createEvent(EventActionDelete)
	event.Target.Repository = repo.Name()

	addSpanEvent(span, event)

	return b.sink.Write(*event)
}

func (b *bridge) createManifestDeleteEventAndWrite(action string, repo reference.Named, dgst digest.Digest, span trace.Span) error {
	event := b.createEvent(action)
	event.Target.Repository = repo.Name()
	event.Target.Digest = dgst

	addSpanEvent(span, event)

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
	event.Target.Length = desc.Size
	event.Target.Size = desc.Size
	event.Target.Digest = desc.Digest
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

func (b *bridge) createBlobDeleteEventAndWrite(action string, repo reference.Named, dgst digest.Digest, span trace.Span) error {
	event := b.createEvent(action)
	event.Target.Digest = dgst
	event.Target.Repository = repo.Name()

	addSpanEvent(span, event)

	return b.sink.Write(*event)
}

func (b *bridge) createBlobEventAndWrite(action string, repo reference.Named, desc distribution.Descriptor, span trace.Span) error {
	event, err := b.createBlobEvent(action, repo, desc)
	if err != nil {
		return err
	}

	addSpanEvent(span, event)

	return b.sink.Write(*event)
}

func (b *bridge) createBlobEvent(action string, repo reference.Named, desc distribution.Descriptor) (*Event, error) {
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
		ID:        uuid.Generate().String(),
		Timestamp: time.Now(),
		Action:    action,
	}
}

// addSpanEvent for every event add to span
func addSpanEvent(span trace.Span, event *Event) {
	bytes, err := json.Marshal(event)
	if err != nil {
		return
	}
	span.AddEvent("manifestEvent", trace.WithAttributes(
		attribute.String("content", string(bytes))))
}

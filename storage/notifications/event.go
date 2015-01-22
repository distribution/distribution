package notifications

import (
	"time"

	"github.com/docker/distribution/digest"
)

// Event provides the fields required to describe a registry event.
type Event struct {
	// UUID provides a unique identifier for the event.
	UUID string `json:"uuid,omitempty"`

	// Timestamp is the time at which the event occurred.
	Timestamp time.Time `json:"timestamp,omitempty"`

	// Action indicates what action encompasses the provided event.
	Action string `json:"action,omitempty"`

	// Target uniquely describes the target of the event.
	Target struct {
		// Type should be "manifest" or "blob"
		Type string `json:"type,omitempty"`

		// Name identifies the named repository.
		Name string `json:"name,omitempty"`

		// Digest should identify the object in the repository.
		Digest digest.Digest `json:"digest,omitempty"`

		// Tag is present if the operation involved a tagged manifest.
		Tag string `json:"tag,omitempty"`

		// URL provides a link to the content on the relevant repository instance.
		URL string `json:"url,omitempty"`
	} `json:"target,omitempty"`

	// Actor specifies the agent that initiated the event. For most
	// situations, this could be from the authorizaton context of the request.
	Actor struct {
		// Name corresponds to the subject or username associated with the
		// request context that generated the event.
		Name string `json:"name,omitempty"`

		// Addr contains the ip or hostname and possibly port of the client
		// connection that initiated the event.
		Addr string `json:"addr,omitempty"`
	} `json:"actor,omitempty"`

	// Source identifies the registry node that generated the event. Put
	// differently, while the actor "initiates" the event, the source
	// "generates" it.
	Source struct {
		// Addr contains the ip or hostname and the port of the registry node
		// that generated the event. Generally, this will be resolved by
		// os.Hostname() along with the running port.
		Addr string `json:"addr,omitempty"`

		// Host is the dns name of the registry cluster, as configured.
		Host string `json:"host,omitempty"`

		// TODO(stevvooe): Host configuration not yet present. Will require a
		// url builder configured with the hostname for this implementation.

		// TODO(stevvooe): Future fields:
		//  RequestID: but this is not yet implemented in webapp.

	} `json:"source,omitempty"`
}

// Sink accepts and sends events.
type Sink interface {
	// Write writes one or more events to the sink. If no error is returned,
	// the caller will assume that all events have been committed and will not
	// try to send them again. If an error is received, the caller may retry
	// sending the event. The caller should cede the slice of memory to the
	// sink and not modify it after calling this method.
	Write(events ...Event) error

	// TODO(stevvooe): The event type should be separate from the json format
	// for this interface but we'll leave as is for now.
}

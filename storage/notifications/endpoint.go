package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// EventsMediaType is the mediatype for the json event envelope. If the Event
// or Envelope struct changes, the version number should be incremented.
const EventsMediaType = "application/vnd.docker.distribution.events.v1+json"

// Envelope defines the fields of a json event envelope message.
type Envelope struct {
	Events []Event `json:"events,omitempty"`
}

// Endpoint implements a single-flight, http notificaiton endpoint. This is
// very lightweight in that it only makes an attempt at an http request.
// Reliability should be provided by the caller.
type Endpoint struct {
	name string
	url  string

	// TODO(stevvooe): Allow one to configure the media type accepted by this
	// endpoint and choose the serialization based on that.
	metrics EndpointMetrics
	mu      sync.Mutex
}

var _ Sink = &Endpoint{}

// EndpointMetrics track various actions taken by the endpoint, typically by
// number of events.
type EndpointMetrics struct {
	Events      int         // total events attempted, including repeats
	Successes   int         // total events written successfully
	Failures    int         // total events failed
	StatusCodes map[int]int // status code histogram, per call event
}

// NewEndpoint returns an endpoint prepared for action.
func NewEndpoint(name, u string) *Endpoint {
	e := Endpoint{
		name: name,
		url:  u,
	}

	e.metrics.StatusCodes = make(map[int]int)

	return &e
}

// Name returns the name of the endpoint, generally used for debugging.
func (e *Endpoint) Name() string {
	return e.name
}

// URL returns the url of the endpoint.
func (e *Endpoint) URL() string {
	return e.url
}

// Accept makes an attempt to notify the endpoint, returning an error if it
// fails. It is the caller's responsibility to retry on error. The events are
// accepted or rejected as a group.
func (e *Endpoint) Write(events ...Event) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.metrics.Events += len(events)

	envelope := Envelope{
		Events: events,
	}

	p, err := json.MarshalIndent(envelope, "", "   ")
	if err != nil {
		e.metrics.Failures++
		return fmt.Errorf("%v: error marshaling event envelope: %v", e, err)
	}

	body := bytes.NewReader(p)
	resp, err := http.Post(e.URL(), EventsMediaType, body)
	if err != nil {
		e.metrics.Failures++
		return fmt.Errorf("%v: error posting: %v", e, err)
	}

	e.metrics.StatusCodes[resp.StatusCode]++

	// The notifier will treat any 2xx or 3xx response as accepted by the
	// endpoint.
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 400:
		e.metrics.Successes++

		// TODO(stevvooe): This is a little accepting: we may want to support
		// unsupport media type responses with retries using the correct media
		// type. There may also be cases that will never work.

		return nil
	default:
		e.metrics.Failures++
		return fmt.Errorf("%v: response status %v unaccepted", e, resp.Status)
	}
}

// ReadMetrics populates em with metrics from the endpoint.
func (e *Endpoint) ReadMetrics(em *EndpointMetrics) {
	e.mu.Lock()
	defer e.mu.Unlock()

	*em = e.metrics

	// Map still need to copied in a threadsafe manner.
	em.StatusCodes = make(map[int]int)
	for k, v := range e.metrics.StatusCodes {
		em.StatusCodes[k] = v
	}
}

func (e *Endpoint) String() string {
	return fmt.Sprintf("notification.Endpoint{Name: %q, URL: %q}", e.Name(), e.URL())
}

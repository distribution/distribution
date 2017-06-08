package notifications

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"code.google.com/p/go-uuid/uuid"
)

// TestEndpoint mocks out an http endpoint and notifies it under a couple of
// conditions, ensuring correct behavior.
func TestEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			t.Fatalf("unexpected request method: %v", r.Method)
			return
		}

		// Extract the content type and make sure it matches
		contentType := r.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			t.Fatalf("error parsing media type: %v, contenttype=%q", err, contentType)
			return
		}

		if mediaType != EventsMediaType {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			t.Fatalf("incorrect media type: %q != %q", mediaType, EventsMediaType)
			return
		}

		var envelope Envelope
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&envelope); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			t.Fatalf("error decoding request body: %v", err)
			return
		}

		// Let caller choose the status
		status, err := strconv.Atoi(r.FormValue("status"))
		if err != nil {
			t.Logf("error parsing status: %v", err)

			// May just be empty, set status to 200
			status = http.StatusOK
		}

		w.WriteHeader(status)
	}))

	endpoint := NewEndpoint("", server.URL)
	var expectedMetrics EndpointMetrics
	expectedMetrics.StatusCodes = make(map[int]int)

	for attempt, tc := range []struct {
		events     []Event // events to send
		url        string
		failure    bool // true if there should be a failure.
		statusCode int  // if not set, no status code should be incremented.
	}{
		{
			statusCode: http.StatusOK,
			events: []Event{
				createTestEvent("push", "library/test", "manifest")},
		},
		{
			statusCode: http.StatusOK,
			events: []Event{
				createTestEvent("push", "library/test", "manifest"),
				createTestEvent("push", "library/test", "layer"),
				createTestEvent("push", "library/test", "layer"),
			},
		},
		{
			statusCode: http.StatusTemporaryRedirect,
		},
		{
			statusCode: http.StatusBadRequest,
			failure:    true,
		},
		{
			// Case where connection never goes through.
			url:     "http://shoudlntresolve/",
			failure: true,
		},
	} {

		expectedMetrics.Events += len(tc.events)

		if tc.failure {
			expectedMetrics.Failures++
		} else {
			expectedMetrics.Successes++
		}

		if tc.statusCode > 0 {
			expectedMetrics.StatusCodes[tc.statusCode]++
		}

		// Normally this isn't okay, but we can set this in the test.
		endpoint.name = fmt.Sprintf("test-%d", attempt)

		url := tc.url
		if url == "" {
			url = server.URL + "/"
		}
		// setup endpoint to respond with expected status code.
		url += fmt.Sprintf("?status=%v", tc.statusCode)
		endpoint.url = url

		if endpoint.Name() != endpoint.name {
			t.Fatalf("endpoint name should match method return: %q != %q", endpoint.Name(), endpoint.name)
		}

		// Try a simple event emission.
		err := endpoint.Write(tc.events...)

		if !tc.failure {
			if err != nil {
				t.Fatalf("unexpected error send event: %v", err)
			}
		} else {
			if err == nil {
				t.Fatalf("the endpoint should have rejected the request")
			}
		}

		var metrics EndpointMetrics
		endpoint.ReadMetrics(&metrics)

		if !reflect.DeepEqual(metrics, expectedMetrics) {
			t.Fatalf("metrics not as expected: %#v != %#v", metrics, expectedMetrics)
		}
	}

}

// TestEventJSONFormat provides silly test to detect if the event format or
// envelope has changed. If this code fails, the revision of the protocol may
// need to be incremented.
func TestEventEnvelopeJSONFormat(t *testing.T) {
	var expected = strings.TrimSpace(`
{
   "events": [
      {
         "uuid": "asdf-asdf-asdf-asdf-0",
         "timestamp": "2006-01-02T15:04:05Z",
         "action": "push",
         "target": {
            "type": "manifest",
            "name": "library/test",
            "digest": "sha256:0123456789abcdef0",
            "tag": "latest",
            "url": "http://example.com/v2/library/test/manifests/latest"
         },
         "actor": {
            "name": "test-actor",
            "addr": "hostname.local"
         },
         "source": {
            "addr": "hostname.local",
            "host": "registrycluster.local"
         }
      },
      {
         "uuid": "asdf-asdf-asdf-asdf-1",
         "timestamp": "2006-01-02T15:04:05Z",
         "action": "push",
         "target": {
            "type": "blob",
            "name": "library/test",
            "digest": "tarsum.v2+sha256:0123456789abcdef1",
            "url": "http://example.com/v2/library/test/manifests/latest"
         },
         "actor": {
            "name": "test-actor",
            "addr": "hostname.local"
         },
         "source": {
            "addr": "hostname.local",
            "host": "registrycluster.local"
         }
      },
      {
         "uuid": "asdf-asdf-asdf-asdf-2",
         "timestamp": "2006-01-02T15:04:05Z",
         "action": "push",
         "target": {
            "type": "blob",
            "name": "library/test",
            "digest": "tarsum.v2+sha256:0123456789abcdef2",
            "url": "http://example.com/v2/library/test/manifests/latest"
         },
         "actor": {
            "name": "test-actor",
            "addr": "hostname.local"
         },
         "source": {
            "addr": "hostname.local",
            "host": "registrycluster.local"
         }
      }
   ]
}
	`)

	tm, err := time.Parse(time.RFC3339, time.RFC3339[:len(time.RFC3339)-5])
	if err != nil {
		t.Fatalf("error creating time: %v", err)
	}

	var prototype Event
	prototype.Action = "push"
	prototype.Timestamp = tm
	prototype.Actor.Addr = "hostname.local"
	prototype.Actor.Name = "test-actor"
	prototype.Source.Addr = "hostname.local"
	prototype.Source.Host = "registrycluster.local"

	var manifestPush Event
	manifestPush = prototype
	manifestPush.UUID = "asdf-asdf-asdf-asdf-0"
	manifestPush.Target.Digest = "sha256:0123456789abcdef0"
	manifestPush.Target.Type = "manifest"
	manifestPush.Target.Name = "library/test"
	manifestPush.Target.Tag = "latest"
	manifestPush.Target.URL = "http://example.com/v2/library/test/manifests/latest"

	var layerPush0 Event
	layerPush0 = prototype
	layerPush0.UUID = "asdf-asdf-asdf-asdf-1"
	layerPush0.Target.Digest = "tarsum.v2+sha256:0123456789abcdef1"
	layerPush0.Target.Type = "blob"
	layerPush0.Target.Name = "library/test"
	layerPush0.Target.URL = "http://example.com/v2/library/test/manifests/latest"

	var layerPush1 Event
	layerPush1 = prototype
	layerPush1.UUID = "asdf-asdf-asdf-asdf-2"
	layerPush1.Target.Digest = "tarsum.v2+sha256:0123456789abcdef2"
	layerPush1.Target.Type = "blob"
	layerPush1.Target.Name = "library/test"
	layerPush1.Target.URL = "http://example.com/v2/library/test/manifests/latest"

	var envelope Envelope
	envelope.Events = append(envelope.Events, manifestPush, layerPush0, layerPush1)

	p, err := json.MarshalIndent(envelope, "", "   ")
	if err != nil {
		t.Fatalf("unexpected error marshaling envelope: %v", err)
	}
	if string(p) != expected {
		t.Fatalf("format has changed\n%s\n != \n%s", string(p), expected)
	}
}

func createTestEvent(action, repo, typ string) Event {
	var event Event

	event.UUID = uuid.New()
	event.Timestamp = time.Now().UTC()
	event.Action = action
	event.Target.Type = typ
	event.Target.Name = repo

	return event
}

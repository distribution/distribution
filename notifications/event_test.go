package notifications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/propagation"
)

// TestEventJSONFormat provides silly test to detect if the event format or
// envelope has changed. If this code fails, the revision of the protocol may
// need to be incremented.
func TestEventEnvelopeJSONFormat(t *testing.T) {
	expected := strings.TrimSpace(`
{
   "events": [
      {
         "id": "asdf-asdf-asdf-asdf-0",
         "timestamp": "2006-01-02T15:04:05Z",
         "action": "push",
         "target": {
            "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
            "digest": "sha256:0123456789abcdef0",
            "size": 1,
            "length": 1,
            "repository": "library/test",
            "url": "http://example.com/v2/library/test/manifests/latest"
         },
         "request": {
            "id": "asdfasdf",
            "addr": "client.local",
            "host": "registrycluster.local",
            "method": "PUT",
            "useragent": "test/0.1"
         },
         "actor": {
            "name": "test-actor"
         },
         "source": {
            "addr": "hostname.local:port"
         }
      },
      {
         "id": "asdf-asdf-asdf-asdf-1",
         "timestamp": "2006-01-02T15:04:05Z",
         "action": "push",
         "target": {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-gtar",
            "digest": "sha256:3b3692957d439ac1928219a83fac91e7bf96c153725526874673ae1f2023f8d5",
            "size": 2,
            "length": 2,
            "repository": "library/test",
            "url": "http://example.com/v2/library/test/manifests/latest"
         },
         "request": {
            "id": "asdfasdf",
            "addr": "client.local",
            "host": "registrycluster.local",
            "method": "PUT",
            "useragent": "test/0.1"
         },
         "actor": {
            "name": "test-actor"
         },
         "source": {
            "addr": "hostname.local:port"
         }
      },
      {
         "id": "asdf-asdf-asdf-asdf-2",
         "timestamp": "2006-01-02T15:04:05Z",
         "action": "push",
         "target": {
            "mediaType": "application/vnd.docker.container.image.rootfs.diff+x-gtar",
            "digest": "sha256:3b3692957d439ac1928219a83fac91e7bf96c153725526874673ae1f2023f8d6",
            "size": 3,
            "length": 3,
            "repository": "library/test",
            "url": "http://example.com/v2/library/test/manifests/latest"
         },
         "request": {
            "id": "asdfasdf",
            "addr": "client.local",
            "host": "registrycluster.local",
            "method": "PUT",
            "useragent": "test/0.1"
         },
         "actor": {
            "name": "test-actor"
         },
         "source": {
            "addr": "hostname.local:port"
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
	prototype.Action = EventActionPush
	prototype.Timestamp = tm
	prototype.Actor.Name = "test-actor"
	prototype.Request.ID = "asdfasdf"
	prototype.Request.Addr = "client.local"
	prototype.Request.Host = "registrycluster.local"
	prototype.Request.Method = "PUT"
	prototype.Request.UserAgent = "test/0.1"
	prototype.Source.Addr = "hostname.local:port"

	manifestPush := prototype
	manifestPush.ID = "asdf-asdf-asdf-asdf-0"
	manifestPush.Target.MediaType = schema2.MediaTypeManifest
	manifestPush.Target.Digest = "sha256:0123456789abcdef0"
	manifestPush.Target.Size = 1
	manifestPush.Target.Length = 1
	manifestPush.Target.Repository = "library/test"
	manifestPush.Target.URL = "http://example.com/v2/library/test/manifests/latest"

	layerPush0 := prototype
	layerPush0.ID = "asdf-asdf-asdf-asdf-1"
	layerPush0.Target.MediaType = layerMediaType
	layerPush0.Target.Digest = "sha256:3b3692957d439ac1928219a83fac91e7bf96c153725526874673ae1f2023f8d5"
	layerPush0.Target.Size = 2
	layerPush0.Target.Length = 2
	layerPush0.Target.Repository = "library/test"
	layerPush0.Target.URL = "http://example.com/v2/library/test/manifests/latest"

	layerPush1 := prototype
	layerPush1.ID = "asdf-asdf-asdf-asdf-2"
	layerPush1.Target.MediaType = layerMediaType
	layerPush1.Target.Digest = "sha256:3b3692957d439ac1928219a83fac91e7bf96c153725526874673ae1f2023f8d6"
	layerPush1.Target.Size = 3
	layerPush1.Target.Length = 3
	layerPush1.Target.Repository = "library/test"
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

// TestEventTraceContextNotLogged ensures that the trace context recorded on an
// event does not end up in the text representation of the event, which is what
// gets logged, for example, when a sink retries a failed delivery.
func TestEventTraceContextNotLogged(t *testing.T) {
	const (
		traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
		baggage     = "userId=alice"
	)

	event := createTestEvent("push", "library/test", schema2.MediaTypeManifest)
	event.traceContext = propagation.MapCarrier{
		"traceparent": traceparent,
		"baggage":     baggage,
	}

	var buf bytes.Buffer
	logger := logrus.New()
	logger.Out = &buf
	logger.Formatter = &logrus.TextFormatter{DisableColors: true}
	logger.WithField("event", event).Info("retrying")

	for name, got := range map[string]string{
		"fmt.Sprint":   fmt.Sprint(event),
		"fmt.Sprintf":  fmt.Sprintf("%+v", &event),
		"text logging": buf.String(),
	} {
		for _, leaked := range []string{traceparent, baggage} {
			if strings.Contains(got, leaked) {
				t.Errorf("%s leaks the trace context %q: %s", name, leaked, got)
			}
		}
		if !strings.Contains(got, event.ID) {
			t.Errorf("%s does not identify the event: %s", name, got)
		}
	}
}

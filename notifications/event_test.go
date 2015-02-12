package notifications

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestEventJSONFormat provides silly test to detect if the event format or
// envelope has changed. If this code fails, the revision of the protocol may
// need to be incremented.
func TestEventEnvelopeJSONFormat(t *testing.T) {
	var expected = strings.TrimSpace(`
{
   "events": [
      {
         "id": "asdf-asdf-asdf-asdf-0",
         "timestamp": "2006-01-02T15:04:05Z",
         "action": "push",
         "target": {
            "type": "manifest",
            "name": "library/test",
            "digest": "sha256:0123456789abcdef0",
            "tag": "latest",
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
            "type": "blob",
            "name": "library/test",
            "digest": "tarsum.v2+sha256:0123456789abcdef1",
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
            "type": "blob",
            "name": "library/test",
            "digest": "tarsum.v2+sha256:0123456789abcdef2",
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
	prototype.Action = "push"
	prototype.Timestamp = tm
	prototype.Actor.Name = "test-actor"
	prototype.Request.ID = "asdfasdf"
	prototype.Request.Addr = "client.local"
	prototype.Request.Host = "registrycluster.local"
	prototype.Request.Method = "PUT"
	prototype.Request.UserAgent = "test/0.1"
	prototype.Source.Addr = "hostname.local:port"

	var manifestPush Event
	manifestPush = prototype
	manifestPush.ID = "asdf-asdf-asdf-asdf-0"
	manifestPush.Target.Digest = "sha256:0123456789abcdef0"
	manifestPush.Target.Type = EventTargetTypeManifest
	manifestPush.Target.Name = "library/test"
	manifestPush.Target.Tag = "latest"
	manifestPush.Target.URL = "http://example.com/v2/library/test/manifests/latest"

	var layerPush0 Event
	layerPush0 = prototype
	layerPush0.ID = "asdf-asdf-asdf-asdf-1"
	layerPush0.Target.Digest = "tarsum.v2+sha256:0123456789abcdef1"
	layerPush0.Target.Type = EventTargetTypeBlob
	layerPush0.Target.Name = "library/test"
	layerPush0.Target.URL = "http://example.com/v2/library/test/manifests/latest"

	var layerPush1 Event
	layerPush1 = prototype
	layerPush1.ID = "asdf-asdf-asdf-asdf-2"
	layerPush1.Target.Digest = "tarsum.v2+sha256:0123456789abcdef2"
	layerPush1.Target.Type = EventTargetTypeBlob
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

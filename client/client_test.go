package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/docker/docker-registry"
	"github.com/docker/docker-registry/test"
)

type testLayer struct {
	tarSum   string
	contents []byte
}

func TestPush(t *testing.T) {
	name := "hello/world"
	tag := "sometag"
	testLayers := []testLayer{
		{
			tarSum:   "12345",
			contents: []byte("some contents"),
		},
		{
			tarSum:   "98765",
			contents: []byte("some other contents"),
		},
	}
	uploadLocations := make([]string, len(testLayers))
	layers := make([]registry.FSLayer, len(testLayers))
	history := make([]registry.ManifestHistory, len(testLayers))

	for i, layer := range testLayers {
		uploadLocations[i] = fmt.Sprintf("/v2/%s/layer/%s/upload-location-%d", name, layer.tarSum, i)
		layers[i] = registry.FSLayer{BlobSum: layer.tarSum}
		history[i] = registry.ManifestHistory{V1Compatibility: layer.tarSum}
	}

	manifest := &registry.ImageManifest{
		Name:          name,
		Tag:           tag,
		Architecture:  "x86",
		FSLayers:      layers,
		History:       history,
		SchemaVersion: 1,
	}
	manifestBytes, err := json.Marshal(manifest)

	layerRequestResponseMappings := make([]test.RequestResponseMapping, 2*len(testLayers))
	for i, layer := range testLayers {
		layerRequestResponseMappings[2*i] = test.RequestResponseMapping{
			Request: test.Request{
				Method: "POST",
				Route:  "/v2/" + name + "/layer/" + layer.tarSum + "/upload/",
			},
			Responses: []test.Response{
				{
					StatusCode: http.StatusAccepted,
					Headers: http.Header(map[string][]string{
						"Location": {uploadLocations[i]},
					}),
				},
			},
		}
		layerRequestResponseMappings[2*i+1] = test.RequestResponseMapping{
			Request: test.Request{
				Method: "PUT",
				Route:  uploadLocations[i],
				Body:   layer.contents,
			},
			Responses: []test.Response{
				{
					StatusCode: http.StatusCreated,
				},
			},
		}
	}

	handler := test.NewHandler(append(layerRequestResponseMappings, test.RequestResponseMap{
		test.RequestResponseMapping{
			Request: test.Request{
				Method: "PUT",
				Route:  "/v2/" + name + "/image/" + tag,
				Body:   manifestBytes,
			},
			Responses: []test.Response{
				{
					StatusCode: http.StatusOK,
				},
			},
		},
	}...))
	server := httptest.NewServer(handler)
	client := New(server.URL)
	objectStore := &memoryObjectStore{
		mutex:           new(sync.Mutex),
		manifestStorage: make(map[string]*registry.ImageManifest),
		layerStorage:    make(map[string]Layer),
	}

	for _, layer := range testLayers {
		l, err := objectStore.Layer(layer.tarSum)
		if err != nil {
			t.Fatal(err)
		}

		writer, err := l.Writer()
		if err != nil {
			t.Fatal(err)
		}

		writer.Write(layer.contents)
		writer.Close()
	}

	objectStore.WriteManifest(name, tag, manifest)

	err = Push(client, objectStore, name, tag)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPull(t *testing.T) {
	name := "hello/world"
	tag := "sometag"
	testLayers := []testLayer{
		{
			tarSum:   "12345",
			contents: []byte("some contents"),
		},
		{
			tarSum:   "98765",
			contents: []byte("some other contents"),
		},
	}
	layers := make([]registry.FSLayer, len(testLayers))
	history := make([]registry.ManifestHistory, len(testLayers))

	for i, layer := range testLayers {
		layers[i] = registry.FSLayer{BlobSum: layer.tarSum}
		history[i] = registry.ManifestHistory{V1Compatibility: layer.tarSum}
	}

	manifest := &registry.ImageManifest{
		Name:          name,
		Tag:           tag,
		Architecture:  "x86",
		FSLayers:      layers,
		History:       history,
		SchemaVersion: 1,
	}
	manifestBytes, err := json.Marshal(manifest)

	layerRequestResponseMappings := make([]test.RequestResponseMapping, len(testLayers))
	for i, layer := range testLayers {
		layerRequestResponseMappings[i] = test.RequestResponseMapping{
			Request: test.Request{
				Method: "GET",
				Route:  "/v2/" + name + "/layer/" + layer.tarSum,
			},
			Responses: []test.Response{
				{
					StatusCode: http.StatusOK,
					Body:       layer.contents,
				},
			},
		}
	}

	handler := test.NewHandler(append(layerRequestResponseMappings, test.RequestResponseMap{
		test.RequestResponseMapping{
			Request: test.Request{
				Method: "GET",
				Route:  "/v2/" + name + "/image/" + tag,
			},
			Responses: []test.Response{
				{
					StatusCode: http.StatusOK,
					Body:       manifestBytes,
				},
			},
		},
	}...))
	server := httptest.NewServer(handler)
	client := New(server.URL)
	objectStore := &memoryObjectStore{
		mutex:           new(sync.Mutex),
		manifestStorage: make(map[string]*registry.ImageManifest),
		layerStorage:    make(map[string]Layer),
	}

	err = Pull(client, objectStore, name, tag)
	if err != nil {
		t.Fatal(err)
	}

	m, err := objectStore.Manifest(name, tag)
	if err != nil {
		t.Fatal(err)
	}

	mBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	if string(mBytes) != string(manifestBytes) {
		t.Fatal("Incorrect manifest")
	}

	for _, layer := range testLayers {
		l, err := objectStore.Layer(layer.tarSum)
		if err != nil {
			t.Fatal(err)
		}

		reader, err := l.Reader()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()

		layerBytes, err := ioutil.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}

		if string(layerBytes) != string(layer.contents) {
			t.Fatal("Incorrect layer")
		}
	}
}

package client

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"

	"github.com/docker/docker-registry/digest"
	"github.com/docker/docker-registry/storage"
)

var (
	// ErrLayerAlreadyExists is returned when attempting to create a layer with
	// a tarsum that is already in use.
	ErrLayerAlreadyExists = errors.New("Layer already exists")

	// ErrLayerLocked is returned when attempting to write to a layer which is
	// currently being written to.
	ErrLayerLocked = errors.New("Layer locked")
)

// ObjectStore is an interface which is designed to approximate the docker
// engine storage. This interface is subject to change to conform to the
// future requirements of the engine.
type ObjectStore interface {
	// Manifest retrieves the image manifest stored at the given repository name
	// and tag
	Manifest(name, tag string) (*storage.SignedManifest, error)

	// WriteManifest stores an image manifest at the given repository name and
	// tag
	WriteManifest(name, tag string, manifest *storage.SignedManifest) error

	// Layer returns a handle to a layer for reading and writing
	Layer(dgst digest.Digest) (Layer, error)
}

// Layer is a generic image layer interface.
// A Layer may only be written to once
type Layer interface {
	// Reader returns an io.ReadCloser which reads the contents of the layer
	Reader() (io.ReadCloser, error)

	// Writer returns an io.WriteCloser which may write the contents of the
	// layer. This method may only be called once per Layer, and the contents
	// are made available on Close
	Writer() (io.WriteCloser, error)

	// Wait blocks until the Layer can be read from
	Wait() error
}

// memoryObjectStore is an in-memory implementation of the ObjectStore interface
type memoryObjectStore struct {
	mutex           *sync.Mutex
	manifestStorage map[string]*storage.SignedManifest
	layerStorage    map[digest.Digest]Layer
}

func (objStore *memoryObjectStore) Manifest(name, tag string) (*storage.SignedManifest, error) {
	objStore.mutex.Lock()
	defer objStore.mutex.Unlock()

	manifest, ok := objStore.manifestStorage[name+":"+tag]
	if !ok {
		return nil, fmt.Errorf("No manifest found with Name: %q, Tag: %q", name, tag)
	}
	return manifest, nil
}

func (objStore *memoryObjectStore) WriteManifest(name, tag string, manifest *storage.SignedManifest) error {
	objStore.mutex.Lock()
	defer objStore.mutex.Unlock()

	objStore.manifestStorage[name+":"+tag] = manifest
	return nil
}

func (objStore *memoryObjectStore) Layer(dgst digest.Digest) (Layer, error) {
	objStore.mutex.Lock()
	defer objStore.mutex.Unlock()

	layer, ok := objStore.layerStorage[dgst]
	if !ok {
		layer = &memoryLayer{cond: sync.NewCond(new(sync.Mutex))}
		objStore.layerStorage[dgst] = layer
	}

	return layer, nil
}

type memoryLayer struct {
	cond    *sync.Cond
	buffer  *bytes.Buffer
	written bool
}

func (ml *memoryLayer) Writer() (io.WriteCloser, error) {
	ml.cond.L.Lock()
	defer ml.cond.L.Unlock()

	if ml.buffer != nil {
		if !ml.written {
			return nil, ErrLayerLocked
		}
		return nil, ErrLayerAlreadyExists
	}

	ml.buffer = new(bytes.Buffer)
	return &memoryLayerWriter{cond: ml.cond, buffer: ml.buffer, done: &ml.written}, nil
}

func (ml *memoryLayer) Reader() (io.ReadCloser, error) {
	ml.cond.L.Lock()
	defer ml.cond.L.Unlock()

	if ml.buffer == nil {
		return nil, fmt.Errorf("Layer has not been written to yet")
	}
	if !ml.written {
		return nil, ErrLayerLocked
	}

	return ioutil.NopCloser(bytes.NewReader(ml.buffer.Bytes())), nil
}

func (ml *memoryLayer) Wait() error {
	ml.cond.L.Lock()
	defer ml.cond.L.Unlock()

	if ml.buffer == nil {
		return fmt.Errorf("No writer to wait on")
	}

	for !ml.written {
		ml.cond.Wait()
	}

	return nil
}

type memoryLayerWriter struct {
	cond   *sync.Cond
	buffer *bytes.Buffer
	done   *bool
}

func (mlw *memoryLayerWriter) Write(p []byte) (int, error) {
	return mlw.buffer.Write(p)
}

func (mlw *memoryLayerWriter) Close() error {
	*mlw.done = true
	mlw.cond.Broadcast()
	return nil
}

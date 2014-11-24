package storage

import (
	"reflect"
	"testing"

	"github.com/docker/libtrust"

	"github.com/docker/docker-registry/digest"
	"github.com/docker/docker-registry/storagedriver/inmemory"
)

func TestManifestStorage(t *testing.T) {
	driver := inmemory.New()
	ms := &manifestStore{
		driver: driver,
		pathMapper: &pathMapper{
			root:    "/storage/testing",
			version: storagePathVersion,
		},
		layerService: newMockedLayerService(),
	}

	name := "foo/bar"
	tag := "thetag"

	exists, err := ms.Exists(name, tag)
	if err != nil {
		t.Fatalf("unexpected error checking manifest existence: %v", err)
	}

	if exists {
		t.Fatalf("manifest should not exist")
	}

	if _, err := ms.Get(name, tag); err != ErrManifestUnknown {
		t.Fatalf("expected manifest unknown error: %v != %v", err, ErrManifestUnknown)
	}

	manifest := Manifest{
		Versioned: Versioned{
			SchemaVersion: 1,
		},
		Name: name,
		Tag:  tag,
		FSLayers: []FSLayer{
			{
				BlobSum: "asdf",
			},
			{
				BlobSum: "qwer",
			},
		},
	}

	pk, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	sm, err := manifest.Sign(pk)
	if err != nil {
		t.Fatalf("error signing manifest: %v", err)
	}

	err = ms.Put(name, tag, sm)
	if err == nil {
		t.Fatalf("expected errors putting manifest")
	}

	// TODO(stevvooe): We expect errors describing all of the missing layers.

	ms.layerService.(*mockedExistenceLayerService).add(name, "asdf")
	ms.layerService.(*mockedExistenceLayerService).add(name, "qwer")

	if err = ms.Put(name, tag, sm); err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	exists, err = ms.Exists(name, tag)
	if err != nil {
		t.Fatalf("unexpected error checking manifest existence: %v", err)
	}

	if !exists {
		t.Fatalf("manifest should exist")
	}

	fetchedManifest, err := ms.Get(name, tag)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	if !reflect.DeepEqual(fetchedManifest, sm) {
		t.Fatalf("fetched manifest not equal: %#v != %#v", fetchedManifest, sm)
	}
}

type layerKey struct {
	name   string
	digest digest.Digest
}

type mockedExistenceLayerService struct {
	exists map[layerKey]struct{}
}

func newMockedLayerService() *mockedExistenceLayerService {
	return &mockedExistenceLayerService{
		exists: make(map[layerKey]struct{}),
	}
}

var _ LayerService = &mockedExistenceLayerService{}

func (mels *mockedExistenceLayerService) add(name string, digest digest.Digest) {
	mels.exists[layerKey{name: name, digest: digest}] = struct{}{}
}

func (mels *mockedExistenceLayerService) remove(name string, digest digest.Digest) {
	delete(mels.exists, layerKey{name: name, digest: digest})
}

func (mels *mockedExistenceLayerService) Exists(name string, digest digest.Digest) (bool, error) {
	_, ok := mels.exists[layerKey{name: name, digest: digest}]
	return ok, nil
}

func (mockedExistenceLayerService) Fetch(name string, digest digest.Digest) (Layer, error) {
	panic("not implemented")
}

func (mockedExistenceLayerService) Upload(name string) (LayerUpload, error) {
	panic("not implemented")
}

func (mockedExistenceLayerService) Resume(uuid string) (LayerUpload, error) {
	panic("not implemented")
}

package storage

import (
	"reflect"
	"testing"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storagedriver/inmemory"
	"github.com/docker/libtrust"
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

	if _, err := ms.Get(name, tag); true {
		switch err.(type) {
		case ErrUnknownManifest:
			break
		default:
			t.Fatalf("expected manifest unknown error: %#v", err)
		}
	}

	m := manifest.Manifest{
		Versioned: manifest.Versioned{
			SchemaVersion: 1,
		},
		Name: name,
		Tag:  tag,
		FSLayers: []manifest.FSLayer{
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

	sm, err := manifest.Sign(&m, pk)
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

	// Grabs the tags and check that this tagged manifest is present
	tags, err := ms.Tags(name)
	if err != nil {
		t.Fatalf("unexpected error fetching tags: %v", err)
	}

	if len(tags) != 1 {
		t.Fatalf("unexpected tags returned: %v", tags)
	}

	if tags[0] != tag {
		t.Fatalf("unexpected tag found in tags: %v != %v", tags, []string{tag})
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

func (mockedExistenceLayerService) Resume(lus LayerUploadState) (LayerUpload, error) {
	panic("not implemented")
}

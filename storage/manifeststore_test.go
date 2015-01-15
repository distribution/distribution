package storage

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/storagedriver/inmemory"
	"github.com/docker/libtrust"
)

func TestManifestStorage(t *testing.T) {
	driver := inmemory.New()
	pm := pathMapper{
		root:    "/storage/testing",
		version: storagePathVersion,
	}
	bs := blobStore{
		driver: driver,
		pm:     &pm,
	}
	ms := &manifestStore{
		driver:     driver,
		pathMapper: &pm,
		revisionStore: &revisionStore{
			driver:     driver,
			pathMapper: &pm,
			blobStore:  &bs,
		},
		tagStore: &tagStore{
			driver:     driver,
			pathMapper: &pm,
			blobStore:  &bs,
		},
		blobStore:    &bs,
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

	fetchedJWS, err := libtrust.ParsePrettySignature(fetchedManifest.Raw, "signatures")
	if err != nil {
		t.Fatalf("unexpected error parsing jws: %v", err)
	}

	payload, err := fetchedJWS.Payload()
	if err != nil {
		t.Fatalf("unexpected error extracting payload: %v", err)
	}

	sigs, err := fetchedJWS.Signatures()
	if err != nil {
		t.Fatalf("unable to extract signatures: %v", err)
	}

	if len(sigs) != 1 {
		t.Fatalf("unexpected number of signatures: %d != %d", len(sigs), 1)
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

	// Now, push the same manifest with a different key
	pk2, err := libtrust.GenerateECP256PrivateKey()
	if err != nil {
		t.Fatalf("unexpected error generating private key: %v", err)
	}

	sm2, err := manifest.Sign(&m, pk2)
	if err != nil {
		t.Fatalf("unexpected error signing manifest: %v", err)
	}

	jws2, err := libtrust.ParsePrettySignature(sm2.Raw, "signatures")
	if err != nil {
		t.Fatalf("error parsing signature: %v", err)
	}

	sigs2, err := jws2.Signatures()
	if err != nil {
		t.Fatalf("unable to extract signatures: %v", err)
	}

	if len(sigs2) != 1 {
		t.Fatalf("unexpected number of signatures: %d != %d", len(sigs2), 1)
	}

	if err = ms.Put(name, tag, sm2); err != nil {
		t.Fatalf("unexpected error putting manifest: %v", err)
	}

	fetched, err := ms.Get(name, tag)
	if err != nil {
		t.Fatalf("unexpected error fetching manifest: %v", err)
	}

	if _, err := manifest.Verify(fetched); err != nil {
		t.Fatalf("unexpected error verifying manifest: %v", err)
	}

	// Assemble our payload and two signatures to get what we expect!
	expectedJWS, err := libtrust.NewJSONSignature(payload, sigs[0], sigs2[0])
	if err != nil {
		t.Fatalf("unexpected error merging jws: %v", err)
	}

	expectedSigs, err := expectedJWS.Signatures()
	if err != nil {
		t.Fatalf("unexpected error getting expected signatures: %v", err)
	}

	receivedJWS, err := libtrust.ParsePrettySignature(fetched.Raw, "signatures")
	if err != nil {
		t.Fatalf("unexpected error parsing jws: %v", err)
	}

	receivedPayload, err := receivedJWS.Payload()
	if err != nil {
		t.Fatalf("unexpected error extracting received payload: %v", err)
	}

	if !bytes.Equal(receivedPayload, payload) {
		t.Fatalf("payloads are not equal")
	}

	receivedSigs, err := receivedJWS.Signatures()
	if err != nil {
		t.Fatalf("error getting signatures: %v", err)
	}

	for i, sig := range receivedSigs {
		if !bytes.Equal(sig, expectedSigs[i]) {
			t.Fatalf("mismatched signatures from remote: %v != %v", string(sig), string(expectedSigs[i]))
		}
	}

	if err := ms.Delete(name, tag); err != nil {
		t.Fatalf("unexpected error deleting manifest: %v", err)
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

func (mockedExistenceLayerService) Resume(name, uuid string) (LayerUpload, error) {
	panic("not implemented")
}

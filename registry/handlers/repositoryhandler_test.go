package handlers

import (
	"reflect"
	"testing"

	"github.com/docker/distribution"
)

type fakeRepository struct{}

func (f *fakeRepository) Name() string {
	return ""
}

func (f *fakeRepository) Manifests() distribution.ManifestService {
	return nil
}

func (f *fakeRepository) Layers() distribution.LayerService {
	return nil
}

func (f *fakeRepository) Signatures() distribution.SignatureService {
	return nil
}

type testRepositoryHandler struct {
	repository       distribution.Repository
	options          map[string]interface{}
	manifestsCalled  bool
	layersCalled     bool
	signaturesCalled bool
}

func (h *testRepositoryHandler) Name() string {
	return "testRepo"
}

func (h *testRepositoryHandler) Manifests() distribution.ManifestService {
	h.manifestsCalled = true
	return nil
}

func (h *testRepositoryHandler) Layers() distribution.LayerService {
	h.layersCalled = true
	return nil
}

func (h *testRepositoryHandler) Signatures() distribution.SignatureService {
	h.signaturesCalled = true
	return nil
}

func newTestRepositoryHandler(repository distribution.Repository, options map[string]interface{}) (distribution.Repository, error) {
	return &testRepositoryHandler{
		repository: repository,
		options:    options,
	}, nil
}

func TestRepositoryHandler(t *testing.T) {
	RegisterRepositoryHandler("testRepositoryHandler", newTestRepositoryHandler)

	options := map[string]interface{}{"option1": "value1"}
	existingRepository := &fakeRepository{}

	handler, err := GetRepositoryHandler("testRepositoryHandler", existingRepository, options)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if e, a := handler.(*testRepositoryHandler).repository, existingRepository; e != a {
		t.Fatalf("init func repository: expected %#v, got %#v", e, a)
	}
	if e, a := handler.(*testRepositoryHandler).options, options; !reflect.DeepEqual(e, a) {
		t.Fatalf("init func options: expected %#v, got %#v", e, a)
	}

	if e, a := "testRepo", handler.Name(); e != a {
		t.Fatalf("name: expected %q, got %q", e, a)
	}

	handler.Manifests()
	if !handler.(*testRepositoryHandler).manifestsCalled {
		t.Fatal("expected handler's Manifests() to have been called")
	}

	handler.Layers()
	if !handler.(*testRepositoryHandler).layersCalled {
		t.Fatal("expected handler's Layers() to have been called")
	}

	handler.Signatures()
	if !handler.(*testRepositoryHandler).signaturesCalled {
		t.Fatal("expected handler's Signatures() to have been called")
	}
}

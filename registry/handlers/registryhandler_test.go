package handlers

import (
	"reflect"
	"testing"

	"github.com/docker/distribution"
	"golang.org/x/net/context"
)

type fakeRegistry struct{}

func (f *fakeRegistry) Repository(ctx context.Context, name string) (distribution.Repository, error) {
	return nil, nil
}

type testRegistryHandler struct {
	registry distribution.Registry
	options  map[string]interface{}
	ctx      context.Context
	name     string
}

func (h *testRegistryHandler) Repository(ctx context.Context, name string) (distribution.Repository, error) {
	h.ctx = ctx
	h.name = name
	return nil, nil
}

func newTestHandler(registry distribution.Registry, options map[string]interface{}) (distribution.Registry, error) {
	return &testRegistryHandler{
		registry: registry,
		options:  options,
	}, nil
}

func TestRegistryHandler(t *testing.T) {
	RegisterRegistryHandler("testRegistryHandler", newTestHandler)

	options := map[string]interface{}{"option1": "value1"}
	existingRegistry := &fakeRegistry{}

	handler, err := GetRegistryHandler("testRegistryHandler", existingRegistry, options)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if e, a := handler.(*testRegistryHandler).registry, existingRegistry; e != a {
		t.Fatalf("init func registry: expected %#v, got %#v", e, a)
	}
	if e, a := handler.(*testRegistryHandler).options, options; !reflect.DeepEqual(e, a) {
		t.Fatalf("init func options: expected %#v, got %#v", e, a)
	}

	ctx := context.Background()
	name := "repo"
	_, err = handler.Repository(ctx, name)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if e, a := ctx, handler.(*testRegistryHandler).ctx; e != a {
		t.Fatalf("ctx: expected %#v, got %#v", e, a)
	}
	if e, a := name, handler.(*testRegistryHandler).name; e != a {
		t.Fatalf("name: expected %s, got %s", e, a)
	}
}

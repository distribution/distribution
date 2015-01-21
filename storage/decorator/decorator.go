package decorator

import (
	"github.com/docker/distribution/digest"
	"github.com/docker/distribution/storage"
)

// Decorator provides an interface for intercepting object creation within a
// registry. The single method accepts an registry storage object, such as a
// Layer, optionally replacing it upon with an alternative object or a
// wrapper.
//
// For example, if one wants to intercept the instantiation of a layer, an
// implementation might be as follows:
//
// 	func (md *DecoratorImplementation) Decorate(v interface{}) interface{} {
// 		switch v := v.(type) {
// 		case Layer:
// 			return wrapLayer(v)
// 		}
//
//		// Make sure to return the object or nil if the decorator doesn't require
//		// replacement.
//		return v
// 	}
//
// Such a decorator can be used to intercept calls to support implementing
// complex features outside of the storage package.
type Decorator interface {
	Decorate(v interface{}) interface{}
}

// Func provides a shortcut handler for decorators that only need a
// function. Use is similar to http.HandlerFunc.
type Func func(v interface{}) interface{}

// Decorate allows DecoratorFunc to implement the Decorator interface.
func (df Func) Decorate(v interface{}) interface{} {
	return df(v)
}

// DecorateRegistry the provided registry with decorator. Registries may be
// decorated multiple times.
func DecorateRegistry(registry storage.Registry, decorator Decorator) storage.Registry {
	return &registryDecorator{
		Registry:  registry,
		decorator: decorator,
	}
}

// registryDecorator intercepts registry object creation with a decorator.
type registryDecorator struct {
	storage.Registry
	decorator Decorator
}

// Repository overrides the method of the same name on the Registry, replacing
// the returned instance with a decorator.
func (rd *registryDecorator) Repository(name string) storage.Repository {
	delegate := rd.Registry.Repository(name)
	decorated := rd.decorator.Decorate(delegate)
	if decorated != nil {
		repository, ok := decorated.(storage.Repository)

		if ok {
			delegate = repository
		}
	}

	return &repositoryDecorator{
		Repository: delegate,
		decorator:  rd.decorator,
	}
}

// repositoryDecorator decorates a repository, intercepting calls to Layers
// and Manifests with injected variants.
type repositoryDecorator struct {
	storage.Repository
	decorator Decorator
}

// Layers overrides the Layers method of Repository.
func (rd *repositoryDecorator) Layers() storage.LayerService {
	delegate := rd.Repository.Layers()
	decorated := rd.decorator.Decorate(delegate)

	if decorated != nil {
		layers, ok := decorated.(storage.LayerService)

		if ok {
			delegate = layers
		}
	}

	return &layerServiceDecorator{
		LayerService: delegate,
		decorator:    rd.decorator,
	}
}

// Manifests overrides the Manifests method of Repository.
func (rd *repositoryDecorator) Manifests() storage.ManifestService {
	delegate := rd.Repository.Manifests()
	decorated := rd.decorator.Decorate(delegate)

	if decorated != nil {
		manifests, ok := decorated.(storage.ManifestService)

		if ok {
			delegate = manifests
		}
	}

	// NOTE(stevvooe): We do not have to intercept delegate calls to the
	// manifest service since it doesn't produce any interfaces for which
	// interception is supported.
	return delegate
}

// layerServiceDecorator intercepts calls that generate Layer and LayerUpload
// instances, replacing them with instances from the decorator.
type layerServiceDecorator struct {
	storage.LayerService
	decorator Decorator
}

// Fetch overrides the Fetch method of LayerService.
func (lsd *layerServiceDecorator) Fetch(digest digest.Digest) (storage.Layer, error) {
	delegate, err := lsd.LayerService.Fetch(digest)
	return decorateLayer(lsd.decorator, delegate), err
}

// Upload overrides the Upload method of LayerService.
func (lsd *layerServiceDecorator) Upload() (storage.LayerUpload, error) {
	delegate, err := lsd.LayerService.Upload()
	return decorateLayerUpload(lsd.decorator, delegate), err
}

// Resume overrides the Resume method of LayerService.
func (lsd *layerServiceDecorator) Resume(uuid string) (storage.LayerUpload, error) {
	delegate, err := lsd.LayerService.Resume(uuid)
	return decorateLayerUpload(lsd.decorator, delegate), err
}

// layerUploadDecorator intercepts calls that generate Layer instances,
// replacing them with instances from the decorator.
type layerUploadDecorator struct {
	storage.LayerUpload
	decorator Decorator
}

func (lud *layerUploadDecorator) Finish(dgst digest.Digest) (storage.Layer, error) {
	delegate, err := lud.LayerUpload.Finish(dgst)
	return decorateLayer(lud.decorator, delegate), err
}

// decorateLayer guarantees that a layer gets correctly decorated.
func decorateLayer(decorator Decorator, delegate storage.Layer) storage.Layer {
	decorated := decorator.Decorate(delegate)
	if decorated != nil {
		layer, ok := decorated.(storage.Layer)
		if ok {
			delegate = layer
		}
	}

	return delegate
}

// decorateLayerUpload guarantees that an upload gets correctly decorated.
func decorateLayerUpload(decorator Decorator, delegate storage.LayerUpload) storage.LayerUpload {
	decorated := decorator.Decorate(delegate)
	if decorated != nil {
		layerUpload, ok := decorated.(storage.LayerUpload)
		if ok {
			delegate = layerUpload
		}
	}

	return &layerUploadDecorator{
		LayerUpload: delegate,
		decorator:   decorator,
	}
}

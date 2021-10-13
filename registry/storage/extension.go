package storage

import (
	"context"
	"fmt"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/registry/storage/cache"
	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/extension"
	registryextension "github.com/distribution/distribution/v3/registry/storage/extension/registry"
	repositoryextension "github.com/distribution/distribution/v3/registry/storage/extension/repository"
)

func ApplyRegistryExtension(ctx context.Context, name string, options map[string]interface{}) RegistryOption {
	return func(r *registry) error {
		if r.registryExtensions == nil {
			r.registryExtensions = make(map[string]registryextension.RegistryExtension)
		}

		ext, err := registryextension.Get(ctx, name, options)
		if err != nil {
			return err
		}

		extName := ext.Name()
		if _, exists := r.registryExtensions[extName]; exists {
			return fmt.Errorf("storage registry extension already registered (%s): %s", name, extName)
		}

		r.registryExtensions[extName] = ext
		return nil
	}
}

type registryExtension struct {
	*registry
	extensions map[string]registryextension.RegistryExtension
}

func (re *registryExtension) Get(ctx context.Context, name string) (interface{}, error) {
	ext, ok := re.extensions[name]
	if !ok {
		return nil, fmt.Errorf("extension %q is not supported", name)
	}

	return ext.RegistryExtension(ctx, re.registry, &storeExtension{
		driver: re.driver,
	})
}

func (re *registryExtension) All(ctx context.Context) ([]string, error) {
	var extNames []string
	for _, ext := range re.extensions {
		extNames = append(extNames, composeExtensionName(ext)...)
	}
	return extNames, nil
}

func ApplyRepositoryExtension(ctx context.Context, name string, options map[string]interface{}) RegistryOption {
	return func(r *registry) error {
		if r.repositoryExtensions == nil {
			r.repositoryExtensions = make(map[string]repositoryextension.RepositoryExtension)
		}

		ext, err := repositoryextension.Get(ctx, name, options)
		if err != nil {
			return err
		}

		extName := ext.Name()
		if _, exists := r.repositoryExtensions[extName]; exists {
			return fmt.Errorf("storage repository extension already registered (%s): %s", name, extName)
		}

		r.repositoryExtensions[extName] = ext
		return nil
	}
}

type repositoryExtension struct {
	*repository
	extensions map[string]repositoryextension.RepositoryExtension
}

func (re *repositoryExtension) Get(ctx context.Context, name string) (interface{}, error) {
	ext, ok := re.extensions[name]
	if !ok {
		return nil, fmt.Errorf("extension %q is not supported", name)
	}

	return ext.RepositoryExtension(ctx, re.repository, &storeExtension{
		repository: re.repository,
		driver:     re.repository.driver,
	})
}

func (re *repositoryExtension) All(ctx context.Context) ([]string, error) {
	var extNames []string
	for _, ext := range re.extensions {
		extNames = append(extNames, composeExtensionName(ext)...)
	}
	return extNames, nil
}

func composeExtensionName(ext extension.Extension) []string {
	name := ext.Name()
	components := ext.Components()
	extNames := make([]string, 0, len(components))
	for _, component := range components {
		extNames = append(extNames, fmt.Sprintf("_%s/%s", name, component))
	}
	return extNames
}

type storeExtension struct {
	*repository
	driver storagedriver.StorageDriver
}

func (se *storeExtension) StorageDriver() storagedriver.StorageDriver {
	return se.driver
}

func (se *storeExtension) LinkedBlobStore(ctx context.Context, opts repositoryextension.LinkedBlobStoreOptions) (repositoryextension.LinkedBlobStore, error) {
	linkPathFns := []linkPathFunc{
		linkPathFunc(opts.ResolvePath),
	}
	var statter distribution.BlobDescriptorService = &linkedBlobStatter{
		blobStore:   se.repository.blobStore,
		repository:  se.repository,
		linkPathFns: linkPathFns,
	}
	if opts.UseCache && se.repository.descriptorCache != nil {
		statter = cache.NewCachedBlobStatter(se.repository.descriptorCache, statter)
	}
	if opts.UseMiddleware && se.repository.registry.blobDescriptorServiceFactory != nil {
		statter = se.repository.registry.blobDescriptorServiceFactory.BlobAccessController(statter)
	}
	return &linkedBlobStoreExtention{
		linkedBlobStore: &linkedBlobStore{
			ctx:                    ctx,
			blobStore:              se.repository.blobStore,
			repository:             se.repository,
			deleteEnabled:          se.repository.registry.deleteEnabled,
			resumableDigestEnabled: se.repository.resumableDigestEnabled,
			blobAccessController:   statter,
			linkPathFns:            linkPathFns,
			linkDirectoryPathSpec: extensionPathSpec{
				path: opts.RootPath,
			},
		},
	}, nil
}

type linkedBlobStoreExtention struct {
	*linkedBlobStore
}

func (lbs *linkedBlobStoreExtention) LinkBlob(ctx context.Context, desc distribution.Descriptor) error {
	return lbs.linkBlob(ctx, desc)
}

package proxy

import (
	"fmt"

	"github.com/docker/distribution"
	"github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/middleware/registry"
	"github.com/docker/distribution/registry/proxy/scheduler"
	"github.com/docker/distribution/registry/storage"
	"github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/factory"
)

type proxyRegistry struct {
	proxiedRegistry distribution.Namespace
	vacuum          storage.Vacuum
}

func newProxyRegistry(registry distribution.Namespace, options map[string]interface{}) (distribution.Namespace, error) {
	d, err := configureStorage(options)
	if err != nil {
		return nil, fmt.Errorf("Unable to configure storage from : %#v", options)
	}
	ctx := context.Background()

	// todo(richardscothern): this would be cleaner with
	// functional arguments
	v := storage.NewVacuum(ctx, d)
	scheduler.OnBlobExpire(func(digest string) error {
		return v.RemoveBlob(digest)
	})
	scheduler.OnManifestExpire(func(repoName string) error {
		return v.RemoveRepository(repoName)
	})

	scheduler.Start()

	proxiedRegistry := proxyRegistry{
		proxiedRegistry: registry,
	}

	return proxiedRegistry, nil
}

func (pr proxyRegistry) Scope() distribution.Scope {
	return pr.proxiedRegistry.Scope()
}

func (pr proxyRegistry) Repository(ctx context.Context, name string) (distribution.Repository, error) {
	return pr.proxiedRegistry.Repository(ctx, name)
}

// init registers the proxy registry
func init() {
	middleware.Register("proxy", middleware.InitFunc(newProxyRegistry))
}

func configureStorage(options map[string]interface{}) (driver.StorageDriver, error) {
	storageConfig, ok := options["storage"]
	if !ok {
		return nil, fmt.Errorf("Unable to configure storage from : %#v", options)
	}

	var driver driver.StorageDriver
	var err error
	// We have a reference to a YAML object here, so therefore lose the strong typing of a
	// Configuration object.  We can still get the factory to create a driver with some
	// munging, but there is probably a better way to do this.
	for k, v := range storageConfig.(map[interface{}]interface{}) {
		driverOptions := map[string]interface{}{}
		for k2, v2 := range v.(map[interface{}]interface{}) {
			driverOptions[k2.(string)] = v2
		}

		driver, err = factory.Create(k.(string), driverOptions)
		if err == nil {
			break
		}

	}
	if driver == nil {
		return nil, fmt.Errorf("Unable to configure storage driver from : %#v", options)
	}
	return driver, err
}

package factory

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/docker/distribution/context"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/uuid"
)

// driverFactories stores an internal mapping between storage driver names and their respective
// factories
var driverFactories = make(map[string]StorageDriverFactory)

// StorageDriverFactory is a factory interface for creating storagedriver.StorageDriver interfaces
// Storage drivers should call Register() with a factory to make the driver available by name
type StorageDriverFactory interface {
	// Create returns a new storagedriver.StorageDriver with the given parameters
	// Parameters will vary by driver and may be ignored
	// Each parameter key must only consist of lowercase letters and numbers
	Create(parameters map[string]interface{}) (storagedriver.StorageDriver, error)
}

// Register makes a storage driver available by the provided name.
// If Register is called twice with the same name or if driver factory is nil, it panics.
func Register(name string, factory StorageDriverFactory) {
	if factory == nil {
		panic("Must not provide nil StorageDriverFactory")
	}
	_, registered := driverFactories[name]
	if registered {
		panic(fmt.Sprintf("StorageDriverFactory named %s already registered", name))
	}

	driverFactories[name] = factory
}

// Create a new storagedriver.StorageDriver with the given name and
// parameters. To use a driver, the StorageDriverFactory must first be
// registered with the given name. If no drivers are found, an
// InvalidStorageDriverError is returned
func Create(ctx context.Context, name string, parameters map[string]interface{}) (storagedriver.StorageDriver, error) {
	driverFactory, ok := driverFactories[name]
	if !ok {
		return nil, InvalidStorageDriverError{name}
	}
	d, err := driverFactory.Create(parameters)
	if err != nil {
		return nil, err
	}
	err = verify(ctx, d)
	if err != nil {
		return nil, fmt.Errorf(`Unable to verify read, write and delete permissions on storage type %q.  
The registry requires these permissions in order to operate correctly (error : %v)`, name, err)
	}
	return d, nil
}

// Ensure that the configured storage driver has permissions for the registry to function
// i.e. read, write and delete a file
func verify(ctx context.Context, driver storagedriver.StorageDriver) error {
	randomFile := fmt.Sprintf("/%s", uuid.Generate().String())
	err := driver.PutContent(ctx, randomFile, []byte(""))
	if err != nil {
		return fmt.Errorf("unable to write verification file: %s", err)
	}
	rand.Seed(time.Now().UTC().UnixNano())

	// May have eventually consistent storage
	max := 3 * time.Second
	duration := 10 * time.Millisecond

	for duration < max {
		if _, err := driver.Stat(ctx, randomFile); err != nil {
			switch err := err.(type) {
			case storagedriver.PathNotFoundError:
				time.Sleep(duration)
				duration = backOffSeconds(duration)
				continue
			default:
				return err
			}
		}
		_, err := driver.GetContent(ctx, randomFile)
		if err == nil {
			break
		}
		return fmt.Errorf("unable to read verification file: %s", err)
	}

	err = driver.Delete(ctx, randomFile)
	if err != nil {
		return fmt.Errorf("unable to delete verification file: %s", err)
	}
	return nil
}

func backOffSeconds(d time.Duration) time.Duration {
	d *= 2
	d += time.Microsecond * time.Duration(rand.Int63n(1000))
	return d
}

// InvalidStorageDriverError records an attempt to construct an unregistered storage driver
type InvalidStorageDriverError struct {
	Name string
}

func (err InvalidStorageDriverError) Error() string {
	return fmt.Sprintf("StorageDriver not registered: %s", err.Name)
}

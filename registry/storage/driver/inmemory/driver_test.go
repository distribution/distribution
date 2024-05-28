package inmemory

import (
	"testing"

	storagedriver "github.com/distribution/distribution/v3/registry/storage/driver"
	"github.com/distribution/distribution/v3/registry/storage/driver/testsuites"
)

func newDriverConstructor() (storagedriver.StorageDriver, error) {
	return New(), nil
}

func TestInMemoryDriverSuite(t *testing.T) {
	testsuites.Driver(t, newDriverConstructor)
}

func BenchmarkInMemoryDriverSuite(b *testing.B) {
	testsuites.BenchDriver(b, newDriverConstructor)
}

package inmemory

import (
	"testing"

	storagedriver "github.com/goharbor/distribution/registry/storage/driver"
	"github.com/goharbor/distribution/registry/storage/driver/testsuites"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

func init() {
	inmemoryDriverConstructor := func() (storagedriver.StorageDriver, error) {
		return New(), nil
	}
	testsuites.RegisterSuite(inmemoryDriverConstructor, testsuites.NeverSkip)
}

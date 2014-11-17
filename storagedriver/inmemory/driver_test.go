package inmemory

import (
	"testing"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

func init() {
	inmemoryDriverConstructor := func() (storagedriver.StorageDriver, error) {
		return New(), nil
	}
	testsuites.RegisterInProcessSuite(inmemoryDriverConstructor, testsuites.NeverSkip)
	testsuites.RegisterIPCSuite(driverName, nil, testsuites.NeverSkip)
}

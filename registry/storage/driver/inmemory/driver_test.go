package inmemory

import (
	"testing"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"

	"gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { check.TestingT(t) }

func init() {
	inmemoryDriverConstructor := func() (storagedriver.StorageDriver, error) {
		return New(), nil
	}
	testsuites.RegisterInProcessSuite(inmemoryDriverConstructor, testsuites.NeverSkip)

	// BUG(stevvooe): Disable flaky IPC tests for now when we can troubleshoot
	// the problems with libchan.
	// testsuites.RegisterIPCSuite(driverName, nil, testsuites.NeverSkip)
}

package filesystem

import (
	"os"
	"testing"

	"github.com/docker/docker-registry/storagedriver"
	"github.com/docker/docker-registry/storagedriver/testsuites"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

func init() {
	rootDirectory := "/tmp/driver"
	os.RemoveAll(rootDirectory)

	filesystemDriverConstructor := func() (storagedriver.StorageDriver, error) {
		return New(rootDirectory), nil
	}
	testsuites.RegisterInProcessSuite(filesystemDriverConstructor, testsuites.NeverSkip)
	testsuites.RegisterIPCSuite(DriverName, map[string]string{"rootdirectory": rootDirectory}, testsuites.NeverSkip)
}

package filesystem

import (
	"io/ioutil"
	"os"
	"testing"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

func init() {
	root, err := ioutil.TempDir("", "driver-")
	if err != nil {
		panic(err)
	}
	defer os.Remove(root)

	testsuites.RegisterInProcessSuite(func() (storagedriver.StorageDriver, error) {
		return New(root), nil
	}, testsuites.NeverSkip)

	// BUG(stevvooe): IPC is broken so we're disabling for now. Will revisit later.
	// testsuites.RegisterIPCSuite(driverName, map[string]string{"rootdirectory": root}, testsuites.NeverSkip)
}

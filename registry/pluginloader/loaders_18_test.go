// +build go1.8,!race

package pluginloader

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	check "gopkg.in/check.v1"

	ctxu "github.com/docker/distribution/context"
	"github.com/docker/distribution/registry/auth"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/docker/distribution/registry/storage/driver/factory"
	"github.com/docker/distribution/registry/storage/driver/testsuites"
)

func TestPlugins(t *testing.T) {
	tempdir, err := ioutil.TempDir("", "ditributiontestplugins")
	if err != nil {
		t.Fatalf("can not create tempdirectory %v\n", err)
	}
	defer os.RemoveAll(tempdir)

	pluginpath := filepath.Join(tempdir, "testplugin"+suffix)
	gobinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("LookPath can not locate go binary: %v", err)
	}
	t.Logf("compile plugin into %s by %s\n", pluginpath, gobinary)
	cmd := exec.Command(gobinary, "build", "-o", pluginpath, "-buildmode", "plugin", "github.com/docker/distribution/registry/pluginloader/testplugin")
	if err = cmd.Run(); err != nil {
		output, _ := cmd.CombinedOutput()
		t.Fatalf("plugin compilation failed %v %s\n", err, output)
	}

	t.Run("TestLoadPlugin", func(t *testing.T) {
		_, err := factory.Create("inmemory", nil)
		if _, ok := err.(factory.InvalidStorageDriverError); !ok {
			t.Fatalf("inmemory driver is not expected to be built in: %T\n", err)
		}

		_, err = auth.GetAccessController("silly", nil)
		if _, ok := err.(auth.InvalidAccessControllerError); !ok {
			t.Fatalf("silly plugin is not expected to be built in: %T\n", err)
		}

		err = LoadPlugins(ctxu.Background(), []string{pluginpath})
		if err != nil {
			t.Fatalf("loading failed %v\n", err)
		}

		dr, err := factory.Create("inmemory", nil)
		if err != nil {
			t.Fatalf("dynamic driver construction failed %v\n", err)
		}
		if dr == nil {
			t.Fatal("driver is not expected to be nil")
		}

		ac, err := auth.GetAccessController("silly", map[string]interface{}{"realm": "realm", "service": "dummy"})
		if err != nil {
			t.Fatalf("AccessController construction failed %v\n", err)
		}
		if ac == nil {
			t.Fatal("AccessController is not expected to be nil")
		}
	})

	t.Run("InmemoryDriverTestSuite", func(t *testing.T) {
		inmemoryDriverConstructor := func() (storagedriver.StorageDriver, error) {
			return factory.Create("inmemory", nil)
		}
		testsuites.RegisterSuite(inmemoryDriverConstructor, testsuites.NeverSkip)
		check.TestingT(t)
	})
}

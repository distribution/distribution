// +build go1.8

package pluginloader

import (
	"os"
	"path/filepath"
	"plugin"

	ctxu "github.com/docker/distribution/context"
)

// LoadPlugins loads plugins pointed by paths. If a path points to a directory
// this directory is scanned for files with a platform specific shared library suffix (like .so, .dylib, .dll).
// NOTE: Plugins are expected to register themselves the same way as built-ins do.
// Storage drivers should use `factory.Register`, AccessControllers `auth.Register` in init().
func LoadPlugins(ctx ctxu.Context, paths []string) error {
	for _, pluginpath := range paths {
		fi, err := os.Stat(pluginpath)
		if err != nil {
			if os.IsNotExist(err) {
				ctxu.GetLogger(ctx).Errorf("plugin file %s does not exist", pluginpath)
			} else {
				ctxu.GetLogger(ctx).Errorf("could not Stat plugin file %s: %v", pluginpath, err)
			}
			continue
		}

		if !fi.IsDir() {
			if err = loadplugin(pluginpath); err != nil {
				ctxu.GetLogger(ctx).Errorf("could not load plugin %s: %v", pluginpath, err)
			}
			continue
		}

		// To preserve the order we do not append this plugins to the paths
		matches, err := filepath.Glob(filepath.Join(pluginpath, "*"+suffix))
		if err != nil {
			return err
		}

		for _, pluginpath := range matches {
			if err = loadplugin(pluginpath); err != nil {
				ctxu.GetLogger(ctx).Errorf("could not load plugin %s: %v", pluginpath, err)
			}
		}
	}
	return nil
}

func loadplugin(path string) error {
	_, err := plugin.Open(path)
	return err
}

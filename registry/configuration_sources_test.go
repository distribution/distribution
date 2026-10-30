package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigurationLayers(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "base.yml")
	overlay := filepath.Join(dir, "overlay.yml")
	for path, data := range map[string]string{
		base:    "version: 0.1\nstorage: inmemory\nhttp:\n  addr: :5000\n",
		overlay: "http:\n  addr: :6000\n",
	} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("REGISTRY_CONFIGURATION_PATH", filepath.Join(dir, "not-used.yml"))
	config, err := resolveConfiguration([]string{base, overlay})
	if err != nil || config.HTTP.Addr != ":6000" {
		t.Fatalf("explicit paths did not override environment path: %#v, %v", config, err)
	}
	t.Setenv("REGISTRY_CONFIGURATION_PATH", base)
	config, err = resolveConfiguration(nil)
	if err != nil || config.HTTP.Addr != ":5000" {
		t.Fatalf("environment path fallback failed: %#v, %v", config, err)
	}
	t.Setenv("REGISTRY_CONFIGURATION_PATH", "")
	_, err = resolveConfiguration(nil)
	if err == nil || !strings.Contains(err.Error(), "configuration path unspecified") {
		t.Fatalf("unexpected missing-path error: %v", err)
	}
}

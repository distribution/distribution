package configuration

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func configSource(name, data string) Source {
	return Source{Name: name, Reader: strings.NewReader(data)}
}

func TestParseSourcesMergeAndEnvironment(t *testing.T) {
	base := `version: 0.1
storage:
  filesystem:
    rootdirectory: /registry
  delete:
    enabled: false
log:
  level: warn
  fields:
    component: registry
http:
  addr: :5000
  headers:
    X-Test: [base, second]
`
	overlay := `storage:
  delete:
    enabled: true
log:
  fields:
    environment: staging
http:
  headers:
    X-Test: [overlay]
`
	t.Setenv("REGISTRY_LOG_LEVEL", "debug")
	config, err := ParseSources(configSource("base", base), configSource("overlay", overlay))
	if err != nil {
		t.Fatal(err)
	}
	if config.Log.Level != "debug" || config.HTTP.Addr != ":5000" {
		t.Fatalf("unexpected log level or address: %s, %s", config.Log.Level, config.HTTP.Addr)
	}
	if got := config.Storage.Parameters()["rootdirectory"]; got != "/registry" {
		t.Fatalf("base storage parameter lost: %v", got)
	}
	if !reflect.DeepEqual(config.HTTP.Headers["X-Test"], []string{"overlay"}) {
		t.Fatalf("sequence was not replaced: %v", config.HTTP.Headers)
	}
	if config.Log.Fields["component"] != "registry" || config.Log.Fields["environment"] != "staging" {
		t.Fatalf("mapping was not merged: %v", config.Log.Fields)
	}
	if config.Storage["delete"]["enabled"] != true {
		t.Fatal("boolean override was not applied")
	}
}

func TestParseSourcesValidation(t *testing.T) {
	const base = "version: 0.1\nstorage: inmemory\n"
	for _, tc := range []struct {
		name    string
		overlay string
		want    string
	}{
		{"version conflict", "version: 0.2\n", "versions differ"},
		{"null version", "version: null\n", "version must not be null"},
		{"invalid version", "version: broken\n", "invalid configuration version"},
		{"duplicate key", "http:\n  addr: one\n  addr: two\n", "already set"},
		{"multiple documents", "http: {}\n---\nlog: {}\n", "single YAML document"},
		{"sequence root", "- one\n- two\n", "YAML mapping"},
		{"non-string key", "http:\n  headers:\n    123: value\n", "non-string mapping key"},
		{"malformed yaml", "http: [\n", "parse configuration"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSources(configSource("base.yaml", base), configSource("overlay.yaml", tc.overlay))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted error containing %q, got %v", tc.want, err)
			}
			if !strings.Contains(err.Error(), "overlay.yaml") {
				t.Fatalf("error omitted its source: %v", err)
			}
		})
	}
}

func TestParseSourcesSingleSourceCompatibility(t *testing.T) {
	const document = "version: 0.1\nstorage:\n  inmemory:\nlog:\n  level: warn\n"
	want, err := Parse(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseSources(configSource("config.yaml", document))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("single-source behavior changed: %#v != %#v", got, want)
	}
	got, err = ParseSources(configSource("empty", "# comment only\n"), configSource("config.yaml", document), configSource("empty-last", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("empty layers changed the effective configuration")
	}
}

func TestParseSourcesMergeShapes(t *testing.T) {
	destination := map[string]any{
		"object": map[string]any{"keep": "base", "change": "before"},
		"list":   []any{"old"},
		"value":  "before",
	}
	overlay := map[string]any{
		"object": map[string]any{"change": "after"},
		"list":   []any{},
		"value":  nil,
	}
	mergeConfigurationLayer(destination, overlay)
	want := map[string]any{
		"object": map[string]any{"keep": "base", "change": "after"},
		"list":   []any{},
		"value":  nil,
	}
	if !reflect.DeepEqual(destination, want) {
		t.Fatalf("unexpected merge result: %#v", destination)
	}
	value, err := normalizeConfigurationValue([]any{map[any]any{"key": "value"}}, "root")
	if err != nil || !reflect.DeepEqual(value, []any{map[string]any{"key": "value"}}) {
		t.Fatalf("nested sequence mapping: %#v, %v", value, err)
	}
}

type failingConfigReader struct{ err error }

func (r failingConfigReader) Read([]byte) (int, error) { return 0, r.err }

func TestParseSourcesReaderErrors(t *testing.T) {
	_, err := ParseSources(configSource("base.yml", ""), configSource("overlay.yml", "# empty"))
	if err == nil || !strings.Contains(err.Error(), "contain no settings") || !strings.Contains(err.Error(), "overlay.yml") {
		t.Fatalf("expected source names in empty-configuration error: %v", err)
	}
	_, err = ParseSources()
	if err == nil {
		t.Fatal("expected no-sources error")
	}
	_, err = ParseSources(Source{})
	if err == nil || !strings.Contains(err.Error(), "source 1") {
		t.Fatalf("expected named nil-reader error: %v", err)
	}
	want := errors.New("read failure")
	_, err = ParseSources(Source{Name: "broken", Reader: failingConfigReader{want}})
	if !errors.Is(err, want) {
		t.Fatalf("wrapped reader error lost: %v", err)
	}
}

func TestParseFiles(t *testing.T) {
	dir := t.TempDir()
	base, overlay := filepath.Join(dir, "base.yml"), filepath.Join(dir, "overlay.yml")
	for path, text := range map[string]string{
		base:    "version: 0.1\nstorage: inmemory\nhttp:\n  addr: :5000\n",
		overlay: "http:\n  addr: :6000\n",
	} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config, err := ParseFiles(base, overlay)
	if err != nil || config.HTTP.Addr != ":6000" {
		t.Fatalf("unexpected file configuration: %#v, %v", config, err)
	}
	for _, paths := range [][]string{nil, {""}, {base, filepath.Join(dir, "missing.yml")}, {dir}} {
		if _, err := ParseFiles(paths...); err == nil {
			t.Fatalf("expected error for %v", paths)
		}
	}
	// Files are closed before returning, including on platforms that deny
	// removal while an open handle is held.
	if err := os.Remove(base); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(overlay); err != nil {
		t.Fatal(err)
	}
}

var _ io.Reader = failingConfigReader{}

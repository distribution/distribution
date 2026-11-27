package configuration

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"go.yaml.in/yaml/v2"
)

// Source is one named configuration layer. ParseSources does not close Reader.
type Source struct {
	Name   string
	Reader io.Reader
}

// ParseFiles loads configuration files in argument order. Later files override
// earlier files, and environment overrides are applied after merging.
func ParseFiles(paths ...string) (*Configuration, error) {
	if len(paths) == 0 {
		return nil, errors.New("no configuration files specified")
	}
	sources := make([]Source, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			return nil, errors.New("configuration path must not be empty")
		}
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open configuration %q: %w", path, err)
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read configuration %q: %w", path, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close configuration %q: %w", path, closeErr)
		}
		sources = append(sources, Source{Name: path, Reader: bytes.NewReader(data)})
	}
	return ParseSources(sources...)
}

// ParseSources merges YAML mappings recursively. Scalars, sequences, and null
// replace earlier values. A single source retains the existing Parse behavior.
// Multiple sources must each contain at most one YAML mapping document and may
// omit version, but explicit versions must agree.
func ParseSources(sources ...Source) (*Configuration, error) {
	if len(sources) == 0 {
		return nil, errors.New("no configuration sources specified")
	}
	for i, source := range sources {
		if source.Reader == nil {
			return nil, fmt.Errorf("configuration %q has no reader", sourceName(source, i))
		}
	}
	if len(sources) == 1 {
		config, err := Parse(sources[0].Reader)
		if err != nil {
			return nil, fmt.Errorf("parse configuration %q: %w", sourceName(sources[0], 0), err)
		}
		return config, nil
	}
	merged := make(map[string]any)
	var version Version
	var versionSource string
	for i, source := range sources {
		name := sourceName(source, i)
		layer, err := readConfigurationLayer(source.Reader)
		if err != nil {
			return nil, fmt.Errorf("parse configuration %q: %w", name, err)
		}
		layerVersion, present, err := configurationLayerVersion(layer)
		if err != nil {
			return nil, fmt.Errorf("configuration %q: %w", name, err)
		}
		if present {
			if version != "" && layerVersion != version {
				return nil, fmt.Errorf("configuration versions differ between %q (%s) and %q (%s)", versionSource, version, name, layerVersion)
			}
			version, versionSource = layerVersion, name
		}
		mergeConfigurationLayer(merged, layer)
	}
	if len(merged) == 0 {
		names := make([]string, len(sources))
		for i, source := range sources {
			names[i] = sourceName(source, i)
		}
		return nil, fmt.Errorf("configuration sources %q contain no settings", names)
	}
	data, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode merged configuration: %w", err)
	}
	config, err := Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse merged configuration: %w", err)
	}
	return config, nil
}

func sourceName(source Source, position int) string {
	if source.Name != "" {
		return source.Name
	}
	return "source " + strconv.Itoa(position+1)
}

func readConfigurationLayer(reader io.Reader) (map[string]any, error) {
	decoder := yaml.NewDecoder(reader)
	decoder.SetStrict(true)
	var document any
	err := decoder.Decode(&document)
	if errors.Is(err, io.EOF) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("expected a single YAML document per configuration source")
	}
	if document == nil {
		return map[string]any{}, nil
	}
	normalized, err := normalizeConfigurationValue(document, "configuration")
	if err != nil {
		return nil, err
	}
	mapping, ok := normalized.(map[string]any)
	if !ok {
		return nil, errors.New("configuration source must contain a YAML mapping")
	}
	return mapping, nil
}

func normalizeConfigurationValue(value any, path string) (any, error) {
	switch value := value.(type) {
	case map[any]any:
		mapping := make(map[string]any, len(value))
		for key, child := range value {
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("%s contains a non-string mapping key", path)
			}
			normalized, err := normalizeConfigurationValue(child, path+"."+name)
			if err != nil {
				return nil, err
			}
			mapping[name] = normalized
		}
		return mapping, nil
	case []any:
		sequence := make([]any, len(value))
		for i, child := range value {
			normalized, err := normalizeConfigurationValue(child, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			sequence[i] = normalized
		}
		return sequence, nil
	default:
		return value, nil
	}
}

func configurationLayerVersion(layer map[string]any) (Version, bool, error) {
	value, exists := layer["version"]
	if !exists {
		return "", false, nil
	}
	if value == nil {
		return "", true, errors.New("configuration version must not be null")
	}
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return "", true, err
	}
	var version Version
	if err := yaml.Unmarshal(encoded, &version); err != nil {
		return "", true, fmt.Errorf("invalid configuration version: %w", err)
	}
	if version == "" {
		return "", true, errors.New("configuration version must not be empty")
	}
	return version, true, nil
}

func mergeConfigurationLayer(destination, overlay map[string]any) {
	for key, value := range overlay {
		newMap, isMap := value.(map[string]any)
		oldMap, wasMap := destination[key].(map[string]any)
		if isMap && wasMap {
			mergeConfigurationLayer(oldMap, newMap)
			continue
		}
		destination[key] = value
	}
}

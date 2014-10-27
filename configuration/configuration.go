package configuration

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

var CurrentVersion = Version{Major: 0, Minor: 1}

type Configuration struct {
	Version  Version  `yaml:"version"`
	Registry Registry `yaml:"registry"`
}

type Version struct {
	Major uint
	Minor uint
}

func (version Version) String() string {
	return fmt.Sprintf("%d.%d", version.Major, version.Minor)
}

func (version Version) MarshalYAML() (interface{}, error) {
	return version.String(), nil
}

type Registry struct {
	LogLevel string
	Storage  Storage
}

type Storage struct {
	Type       string
	Parameters map[string]string
}

func (storage Storage) MarshalYAML() (interface{}, error) {
	return yaml.MapSlice{yaml.MapItem{storage.Type, storage.Parameters}}, nil
}

type untypedConfiguration struct {
	Version  string      `yaml:"version"`
	Registry interface{} `yaml:"registry"`
}

type v_0_1_RegistryConfiguration struct {
	LogLevel string      `yaml:"loglevel"`
	Storage  interface{} `yaml:"storage"`
}

func Parse(in []byte) (*Configuration, error) {
	var untypedConfig untypedConfiguration
	var config Configuration

	err := yaml.Unmarshal(in, &untypedConfig)
	if err != nil {
		return nil, err
	}
	if untypedConfig.Version == "" {
		return nil, fmt.Errorf("Please specify a configuration version. Current version is %s", CurrentVersion)
	}
	versionParts := strings.Split(untypedConfig.Version, ".")
	if len(versionParts) != 2 {
		return nil, fmt.Errorf("Invalid version: %s Expected format: X.Y", untypedConfig.Version)
	}
	majorVersion, err := strconv.ParseUint(versionParts[0], 10, 0)
	if err != nil {
		return nil, fmt.Errorf("Major version must be of type uint, received %v", versionParts[0])
	}
	minorVersion, err := strconv.ParseUint(versionParts[1], 10, 0)
	if err != nil {
		return nil, fmt.Errorf("Minor version must be of type uint, received %v", versionParts[1])
	}
	config.Version = Version{Major: uint(majorVersion), Minor: uint(minorVersion)}

	switch config.Version {
	case Version{0, 1}:
		registry, err := parseV_0_1_Registry(untypedConfig.Registry)
		if err != nil {
			return nil, err
		}

		config.Registry = *registry
	default:
		return nil, fmt.Errorf("Unsupported configuration version %s Current version is %s", config.Version, CurrentVersion)
	}

	switch config.Registry.LogLevel {
	case "error", "warn", "info", "debug":
	default:
		return nil, fmt.Errorf("Invalid loglevel %s Must be one of [error, warn, info, debug]", config.Registry.LogLevel)
	}

	return &config, nil
}

func parseV_0_1_Registry(registry interface{}) (*Registry, error) {
	envMap := getEnvMap()

	registryBytes, err := yaml.Marshal(registry)
	if err != nil {
		return nil, err
	}
	var v_0_1 v_0_1_RegistryConfiguration
	err = yaml.Unmarshal(registryBytes, &v_0_1)
	if err != nil {
		return nil, err
	}

	if logLevel, ok := envMap["REGISTRY_LOGLEVEL"]; ok {
		v_0_1.LogLevel = logLevel
	}
	v_0_1.LogLevel = strings.ToLower(v_0_1.LogLevel)

	var storage Storage
	storage.Parameters = make(map[string]string)

	switch v_0_1.Storage.(type) {
	case string:
		storage.Type = v_0_1.Storage.(string)
	case map[interface{}]interface{}:
		storageMap := v_0_1.Storage.(map[interface{}]interface{})
		if len(storageMap) > 1 {
			keys := make([]string, 0, len(storageMap))
			for key := range storageMap {
				keys = append(keys, toString(key))
			}
			return nil, fmt.Errorf("Must provide exactly one storage type. Provided: %v", keys)
		}
		var params map[interface{}]interface{}
		// There will only be one key-value pair at this point
		for k, v := range storageMap {
			storage.Type = toString(k)
			paramsMap, ok := v.(map[interface{}]interface{})
			if !ok {
				return nil, fmt.Errorf("Must provide parameters as a map[string]string. Provided: %#v", v)
			}
			params = paramsMap
		}
		for k, v := range params {
			storage.Parameters[toString(k)] = toString(v)
		}

	case interface{}:
		// Bad type for storage
		return nil, fmt.Errorf("Registry storage must be provided by name, optionally with parameters. Provided: %v", v_0_1.Storage)
	}

	if storageType, ok := envMap["REGISTRY_STORAGE"]; ok {
		if storageType != storage.Type {
			storage.Type = storageType
			// Reset the storage parameters because we're using a different storage type
			storage.Parameters = make(map[string]string)
		}
	}

	if storage.Type == "" {
		return nil, fmt.Errorf("Must provide exactly one storage type, optionally with parameters. Provided: %v", v_0_1.Storage)
	}

	storageParamsRegexp, err := regexp.Compile(fmt.Sprintf("^REGISTRY_STORAGE_%s_([A-Z0-9]+)$", strings.ToUpper(storage.Type)))
	if err != nil {
		return nil, err
	}
	for k, v := range envMap {
		if submatches := storageParamsRegexp.FindStringSubmatch(k); submatches != nil {
			storage.Parameters[strings.ToLower(submatches[1])] = v
		}
	}

	return &Registry{LogLevel: v_0_1.LogLevel, Storage: storage}, nil
}

func getEnvMap() map[string]string {
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		envParts := strings.SplitN(env, "=", 2)
		envMap[envParts[0]] = envParts[1]
	}
	return envMap
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

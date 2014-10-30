package configuration

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

// CurrentVersion is the most recent Version that can be parsed
var CurrentVersion = Version{Major: 0, Minor: 1}

// Configuration is a versioned system configuration
// When marshaled into yaml, this produces a document matching the current version's format
type Configuration struct {
	Version  Version  `yaml:"version"`
	Registry Registry `yaml:"registry"`
}

// Version is a major/minor version pair
// Minor version upgrades should be strictly additive
// Major version upgrades indicate structure or type changes
type Version struct {
	Major uint
	Minor uint
}

func (version Version) String() string {
	return fmt.Sprintf("%d.%d", version.Major, version.Minor)
}

// MarshalYAML is implemented to serialize the Version into a string format
func (version Version) MarshalYAML() (interface{}, error) {
	return version.String(), nil
}

// Registry defines the configuration for a registry
type Registry struct {
	// LogLevel specifies the level at which the registry will be logged
	LogLevel string

	// Storage specifies the configuration of the registry's object storage
	Storage Storage
}

// Storage defines the configuration for registry object storage
type Storage struct {
	// Type specifies the storage driver type (examples: inmemory, filesystem, s3, ...)
	Type string

	// Parameters specifies the key/value parameters map passed to the storage driver constructor
	Parameters map[string]string
}

func (storage Storage) MarshalYAML() (interface{}, error) {
	return yaml.MapSlice{yaml.MapItem{storage.Type, storage.Parameters}}, nil
}

// untypedConfiguration is the unmarshalable configuration struct that only assumes the existence of
// a version string parameter
// This is done to parse the configuration version, then parse the remainder with a version-specific
// parser
type untypedConfiguration struct {
	// Version is the version string defined in a configuration yaml
	// This can safely parse versions defined as float types in yaml
	Version string `yaml:"version"`

	// Registry is an untyped placeholder for the Registry configuration, which can later be parsed
	// into a current Registry struct
	Registry interface{} `yaml:"registry"`
}

// V_0_1_RegistryConfiguration is the unmarshalable Registry configuration struct specific to
// Version{0, 1}
type V_0_1_RegistryConfiguration struct {
	// LogLevel is the level at which the registry will log
	// The loglevel can be overridden with the environment variable REGISTRY_LOGLEVEL, for example:
	// REGISTRY_LOGLEVEL=info
	LogLevel string `yaml:"loglevel"`

	// Storage is an untyped placeholder for the Storage configuration, which can later be parsed as
	// a Storage struct
	// The storage type can be overridden with the environment variable REGISTRY_STORAGE, for
	// example: REGISTRY_STORAGE=s3
	// Note: If REGISTRY_STORAGE changes the storage type, all included parameters will be ignored
	// The storage parameters can be overridden with any environment variable of the format:
	// REGISTRY_STORAGE_<storage driver type>_<parameter name>, for example:
	// REGISTRY_STORAGE_S3_BUCKET=my-bucket
	Storage interface{} `yaml:"storage"`
}

// Parse parses an input configuration yaml document into a Configuration struct
// This should be capable of handling old configuration format versions
//
// Environment variables may be used to override configuration parameters other than version, which
// may be defined on a per-version basis. See V_0_1_RegistryConfiguration for more details
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

	// Convert the version string from X.Y to Version{X, Y}
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

	// Parse the remainder of the configuration depending on the provided version
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

// parseV_0_1_Registry parses a Registry configuration for Version{0, 1}
func parseV_0_1_Registry(registry interface{}) (*Registry, error) {
	envMap := getEnvMap()

	registryBytes, err := yaml.Marshal(registry)
	if err != nil {
		return nil, err
	}
	var v_0_1 V_0_1_RegistryConfiguration
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
		// Storage is provided only by type
		storage.Type = v_0_1.Storage.(string)
	case map[interface{}]interface{}:
		// Storage is provided as a {type: parameters} map
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
			// Parameters may be parsed as numerical or boolean values, so just convert these to
			// strings
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

	// Find all environment variables of the format:
	// REGISTRY_STORAGE_<storage driver type>_<parameter name>
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

// getEnvMap reads the current environment variables and converts these into a key/value map
func getEnvMap() map[string]string {
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		envParts := strings.SplitN(env, "=", 2)
		envMap[envParts[0]] = envParts[1]
	}
	return envMap
}

// toString converts reasonable objects into strings that may be used for configuration parameters
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

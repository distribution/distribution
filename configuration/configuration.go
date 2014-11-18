package configuration

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/BrianBland/yaml.v2"
)

// Configuration is a versioned registry configuration, intended to be provided by a yaml file, and
// optionally modified by environment variables
type Configuration struct {
	// Version is the version which defines the format of the rest of the configuration
	Version Version `yaml:"version"`

	// Loglevel is the level at which registry operations are logged
	Loglevel Loglevel `yaml:"loglevel"`

	// Storage is the configuration for the registry's storage driver
	Storage Storage `yaml:"storage"`
}

// v0_1Configuration is a Version 0.1 Configuration struct
// This is currently aliased to Configuration, as it is the current version
type v0_1Configuration Configuration

// Version is a major/minor version pair of the form Major.Minor
// Major version upgrades indicate structure or type changes
// Minor version upgrades should be strictly additive
type Version string

// MajorMinorVersion constructs a Version from its Major and Minor components
func MajorMinorVersion(major, minor uint) Version {
	return Version(fmt.Sprintf("%d.%d", major, minor))
}

func (version Version) major() (uint, error) {
	majorPart := strings.Split(string(version), ".")[0]
	major, err := strconv.ParseUint(majorPart, 10, 0)
	return uint(major), err
}

// Major returns the major version portion of a Version
func (version Version) Major() uint {
	major, _ := version.major()
	return major
}

func (version Version) minor() (uint, error) {
	minorPart := strings.Split(string(version), ".")[1]
	minor, err := strconv.ParseUint(minorPart, 10, 0)
	return uint(minor), err
}

// Minor returns the minor version portion of a Version
func (version Version) Minor() uint {
	minor, _ := version.minor()
	return minor
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
// Unmarshals a string of the form X.Y into a Version, validating that X and Y can represent uints
func (version *Version) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var versionString string
	err := unmarshal(&versionString)
	if err != nil {
		return err
	}

	newVersion := Version(versionString)
	if _, err := newVersion.major(); err != nil {
		return err
	}

	if _, err := newVersion.minor(); err != nil {
		return err
	}

	*version = newVersion
	return nil
}

// CurrentVersion is the most recent Version that can be parsed
var CurrentVersion = MajorMinorVersion(0, 1)

// Loglevel is the level at which operations are logged
// This can be error, warn, info, or debug
type Loglevel string

// UnmarshalYAML implements the yaml.Umarshaler interface
// Unmarshals a string into a Loglevel, lowercasing the string and validating that it represents a
// valid loglevel
func (loglevel *Loglevel) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var loglevelString string
	err := unmarshal(&loglevelString)
	if err != nil {
		return err
	}

	loglevelString = strings.ToLower(loglevelString)
	switch loglevelString {
	case "error", "warn", "info", "debug":
	default:
		return fmt.Errorf("Invalid loglevel %s Must be one of [error, warn, info, debug]", loglevelString)
	}

	*loglevel = Loglevel(loglevelString)
	return nil
}

// Storage defines the configuration for registry object storage
type Storage map[string]Parameters

// Type returns the storage driver type, such as filesystem or s3
func (storage Storage) Type() string {
	// Return only key in this map
	for k := range storage {
		return k
	}
	return ""
}

// Parameters returns the Parameters map for a Storage configuration
func (storage Storage) Parameters() Parameters {
	return storage[storage.Type()]
}

// setParameter changes the parameter at the provided key to the new value
func (storage Storage) setParameter(key, value string) {
	storage[storage.Type()][key] = value
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
// Unmarshals a single item map into a Storage or a string into a Storage type with no parameters
func (storage *Storage) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var storageMap map[string]Parameters
	err := unmarshal(&storageMap)
	if err == nil {
		if len(storageMap) > 1 {
			types := make([]string, 0, len(storageMap))
			for k := range storageMap {
				types = append(types, k)
			}
			return fmt.Errorf("Must provide exactly one storage type. Provided: %v", types)
		}
		*storage = storageMap
		return nil
	}

	var storageType string
	err = unmarshal(&storageType)
	if err == nil {
		*storage = Storage{storageType: Parameters{}}
		return nil
	}

	return err
}

// MarshalYAML implements the yaml.Marshaler interface
func (storage Storage) MarshalYAML() (interface{}, error) {
	if storage.Parameters() == nil {
		return storage.Type, nil
	}
	return map[string]Parameters(storage), nil
}

// Parameters defines a key-value parameters mapping
type Parameters map[string]string

// Parse parses an input configuration yaml document into a Configuration struct
// This should generally be capable of handling old configuration format versions
//
// Environment variables may be used to override configuration parameters other than version,
// following the scheme below:
// Configuration.Abc may be replaced by the value of REGISTRY_ABC,
// Configuration.Abc.Xyz may be replaced by the value of REGISTRY_ABC_XYZ, and so forth
func Parse(in []byte) (*Configuration, error) {
	var untypedConfig struct {
		Version Version
	}
	var config *Configuration

	err := yaml.Unmarshal(in, &untypedConfig)
	if err != nil {
		return nil, err
	}
	if untypedConfig.Version == "" {
		return nil, fmt.Errorf("Please specify a configuration version. Current version is %s", CurrentVersion)
	}

	// Parse the remainder of the configuration depending on the provided version
	switch untypedConfig.Version {
	case "0.1":
		config, err = parseV0_1Registry(in)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("Unsupported configuration version %s Current version is %s", untypedConfig.Version, CurrentVersion)
	}

	return config, nil
}

// parseV0_1Registry parses a registry Configuration for Version 0.1
func parseV0_1Registry(in []byte) (*Configuration, error) {
	envMap := getEnvMap()

	var config v0_1Configuration
	err := yaml.Unmarshal(in, &config)
	if err != nil {
		return nil, err
	}

	// Override config.Loglevel if environment variable is provided
	if loglevel, ok := envMap["REGISTRY_LOGLEVEL"]; ok {
		var newLoglevel Loglevel
		err := yaml.Unmarshal([]byte(loglevel), &newLoglevel)
		if err != nil {
			return nil, err
		}
		config.Loglevel = newLoglevel
	}

	// Override config.Storage if environment variable is provided
	if storageType, ok := envMap["REGISTRY_STORAGE"]; ok {
		if storageType != config.Storage.Type() {
			// Reset the storage parameters because we're using a different storage type
			config.Storage = Storage{storageType: Parameters{}}
		}
	}

	if config.Storage.Type() == "" {
		return nil, fmt.Errorf("Must provide exactly one storage type, optionally with parameters. Provided: %v", config.Storage)
	}

	// Override storage parameters with all environment variables of the format:
	// REGISTRY_STORAGE_<storage driver type>_<parameter name>
	storageParamsRegexp, err := regexp.Compile(fmt.Sprintf("^REGISTRY_STORAGE_%s_([A-Z0-9]+)$", strings.ToUpper(config.Storage.Type())))
	if err != nil {
		return nil, err
	}
	for k, v := range envMap {
		if submatches := storageParamsRegexp.FindStringSubmatch(k); submatches != nil {
			config.Storage.setParameter(strings.ToLower(submatches[1]), v)
		}
	}

	return (*Configuration)(&config), nil
}

// getEnvMap reads the current environment variables and converts these into a key/value map
// This is used to distinguish between empty strings returned by os.GetEnv(key) because of undefined
// environment variables and explicitly empty ones
func getEnvMap() map[string]string {
	envMap := make(map[string]string)
	for _, env := range os.Environ() {
		envParts := strings.SplitN(env, "=", 2)
		envMap[envParts[0]] = envParts[1]
	}
	return envMap
}

package configuration

import (
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strings"
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

	// Auth allows configuration of various authorization methods that may be
	// used to gate requests.
	Auth Auth `yaml:"auth"`

	// Reporting is the configuration for error reporting
	Reporting Reporting `yaml:"reporting"`

	// HTTP contains configuration parameters for the registry's http
	// interface.
	HTTP struct {
		// Addr specifies the bind address for the registry instance.
		Addr string `yaml:"addr"`

		// Secret specifies the secret key which HMAC tokens are created with.
		Secret string `yaml:"secret"`

		// LayerHandler specifies a middleware for serving image layers.
		LayerHandler LayerHandler `yaml:"layerhandler"`
	} `yaml:"http"`
}

// v0_1Configuration is a Version 0.1 Configuration struct
// This is currently aliased to Configuration, as it is the current version
type v0_1Configuration Configuration

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

// Parameters defines a key-value parameters mapping
type Parameters map[string]interface{}

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
func (storage Storage) setParameter(key string, value interface{}) {
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
		return storage.Type(), nil
	}
	return map[string]Parameters(storage), nil
}

// Auth defines the configuration for registry authorization.
type Auth map[string]Parameters

// Type returns the storage driver type, such as filesystem or s3
func (auth Auth) Type() string {
	// Return only key in this map
	for k := range auth {
		return k
	}
	return ""
}

// Parameters returns the Parameters map for an Auth configuration
func (auth Auth) Parameters() Parameters {
	return auth[auth.Type()]
}

// setParameter changes the parameter at the provided key to the new value
func (auth Auth) setParameter(key string, value interface{}) {
	auth[auth.Type()][key] = value
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
// Unmarshals a single item map into a Storage or a string into a Storage type with no parameters
func (auth *Auth) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var m map[string]Parameters
	err := unmarshal(&m)
	if err == nil {
		if len(m) > 1 {
			types := make([]string, 0, len(m))
			for k := range m {
				types = append(types, k)
			}

			// TODO(stevvooe): May want to change this slightly for
			// authorization to allow multiple challenges.
			return fmt.Errorf("must provide exactly one type. Provided: %v", types)

		}
		*auth = m
		return nil
	}

	var authType string
	err = unmarshal(&authType)
	if err == nil {
		*auth = Auth{authType: Parameters{}}
		return nil
	}

	return err
}

// MarshalYAML implements the yaml.Marshaler interface
func (auth Auth) MarshalYAML() (interface{}, error) {
	if auth.Parameters() == nil {
		return auth.Type(), nil
	}
	return map[string]Parameters(auth), nil
}

// Reporting defines error reporting methods.
type Reporting struct {
	// Bugsnag configures error reporting for Bugsnag (bugsnag.com).
	Bugsnag BugsnagReporting `yaml:"bugsnag"`
	// NewRelic configures error reporting for NewRelic (newrelic.com)
	NewRelic NewRelicReporting `yaml:"newrelic"`
}

// BugsnagReporting configures error reporting for Bugsnag (bugsnag.com).
type BugsnagReporting struct {
	// APIKey is the Bugsnag api key.
	APIKey string `yaml:"apikey"`
	// ReleaseStage tracks where the registry is deployed.
	// Examples: production, staging, development
	ReleaseStage string `yaml:"releasestage"`
	// Endpoint is used for specifying an enterprise Bugsnag endpoint.
	Endpoint string `yaml:"endpoint"`
}

// NewRelicReporting configures error reporting for NewRelic (newrelic.com)
type NewRelicReporting struct {
	// LicenseKey is the NewRelic user license key
	LicenseKey string `yaml:"licensekey"`
	// Name is the component name of the registry in NewRelic
	Name string `yaml:"name"`
}

// LayerHandler defines the configuration for middleware layer serving
type LayerHandler map[string]Parameters

// Type returns the layerhandler type
func (layerHandler LayerHandler) Type() string {
	// Return only key in this map
	for k := range layerHandler {
		return k
	}
	return ""
}

// Parameters returns the Parameters map for a LayerHandler configuration
func (layerHandler LayerHandler) Parameters() Parameters {
	return layerHandler[layerHandler.Type()]
}

// UnmarshalYAML implements the yaml.Unmarshaler interface
// Unmarshals a single item map into a Storage or a string into a Storage type with no parameters
func (layerHandler *LayerHandler) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var storageMap map[string]Parameters
	err := unmarshal(&storageMap)
	if err == nil {
		if len(storageMap) > 1 {
			types := make([]string, 0, len(storageMap))
			for k := range storageMap {
				types = append(types, k)
			}
			return fmt.Errorf("Must provide exactly one layerhandler type. Provided: %v", types)
		}
		*layerHandler = storageMap
		return nil
	}

	var storageType string
	err = unmarshal(&storageType)
	if err == nil {
		*layerHandler = LayerHandler{storageType: Parameters{}}
		return nil
	}

	return err
}

// MarshalYAML implements the yaml.Marshaler interface
func (layerHandler LayerHandler) MarshalYAML() (interface{}, error) {
	if layerHandler.Parameters() == nil {
		return layerHandler.Type(), nil
	}
	return map[string]Parameters(layerHandler), nil
}

// Parse parses an input configuration yaml document into a Configuration struct
// This should generally be capable of handling old configuration format versions
//
// Environment variables may be used to override configuration parameters other than version,
// following the scheme below:
// Configuration.Abc may be replaced by the value of REGISTRY_ABC,
// Configuration.Abc.Xyz may be replaced by the value of REGISTRY_ABC_XYZ, and so forth
func Parse(rd io.Reader) (*Configuration, error) {
	in, err := ioutil.ReadAll(rd)
	if err != nil {
		return nil, err
	}

	p := NewParser("registry", []VersionedParseInfo{
		{
			Version: MajorMinorVersion(0, 1),
			ParseAs: reflect.TypeOf(v0_1Configuration{}),
			ConversionFunc: func(c interface{}) (interface{}, error) {
				if v0_1, ok := c.(*v0_1Configuration); ok {
					if v0_1.Loglevel == Loglevel("") {
						v0_1.Loglevel = Loglevel("info")
					}
					if v0_1.Storage.Type() == "" {
						return nil, fmt.Errorf("No storage configuration provided")
					}
					return (*Configuration)(v0_1), nil
				}
				return nil, fmt.Errorf("Expected *v0_1Configuration, received %#v", c)
			},
		},
	})

	config := new(Configuration)
	err = p.Parse(in, config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

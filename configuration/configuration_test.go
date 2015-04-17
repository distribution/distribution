package configuration

import (
	"bytes"
	"net/http"
	"os"
	"testing"

	. "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

// Hook up gocheck into the "go test" runner
func Test(t *testing.T) { TestingT(t) }

// configStruct is a canonical example configuration, which should map to configYamlV0_1
var configStruct = Configuration{
	Version: "0.1",
	Log: struct {
		Level     Loglevel               `yaml:"level"`
		Formatter string                 `yaml:"formatter,omitempty"`
		Fields    map[string]interface{} `yaml:"fields,omitempty"`
		Hooks     []LogHook              `yaml:"hooks,omitempty"`
	}{
		Fields: map[string]interface{}{"environment": "test"},
	},
	Loglevel: "info",
	Storage: Storage{
		"s3": Parameters{
			"region":        "us-east-1",
			"bucket":        "my-bucket",
			"rootdirectory": "/registry",
			"encrypt":       true,
			"secure":        false,
			"accesskey":     "SAMPLEACCESSKEY",
			"secretkey":     "SUPERSECRET",
			"host":          nil,
			"port":          42,
		},
	},
	Auth: Auth{
		"silly": Parameters{
			"realm":   "silly",
			"service": "silly",
		},
	},
	Reporting: Reporting{
		Bugsnag: BugsnagReporting{
			APIKey: "BugsnagApiKey",
		},
	},
	Notifications: Notifications{
		Endpoints: []Endpoint{
			{
				Name: "endpoint-1",
				URL:  "http://example.com",
				Headers: http.Header{
					"Authorization": []string{"Bearer <example>"},
				},
			},
		},
	},
	HTTP: struct {
		Addr   string `yaml:"addr,omitempty"`
		Net    string `yaml:"net,omitempty"`
		Prefix string `yaml:"prefix,omitempty"`
		Secret string `yaml:"secret,omitempty"`
		TLS    struct {
			Certificate string   `yaml:"certificate,omitempty"`
			Key         string   `yaml:"key,omitempty"`
			ClientCAs   []string `yaml:"clientcas,omitempty"`
		} `yaml:"tls,omitempty"`
		Debug struct {
			Addr string `yaml:"addr,omitempty"`
		} `yaml:"debug,omitempty"`
	}{
		TLS: struct {
			Certificate string   `yaml:"certificate,omitempty"`
			Key         string   `yaml:"key,omitempty"`
			ClientCAs   []string `yaml:"clientcas,omitempty"`
		}{
			ClientCAs: []string{"/path/to/ca.pem"},
		},
	},
}

// configYamlV0_1 is a Version 0.1 yaml document representing configStruct
var configYamlV0_1 = `
version: 0.1
log:
  fields:
    environment: test
loglevel: info
storage:
  s3:
    region: us-east-1
    bucket: my-bucket
    rootdirectory: /registry
    encrypt: true
    secure: false
    accesskey: SAMPLEACCESSKEY
    secretkey: SUPERSECRET
    host: ~
    port: 42
auth:
  silly:
    realm: silly
    service: silly
notifications:
  endpoints:
    - name: endpoint-1
      url:  http://example.com
      headers:
        Authorization: [Bearer <example>]
reporting:
  bugsnag:
    apikey: BugsnagApiKey
http:
  clientcas:
    - /path/to/ca.pem
`

// inmemoryConfigYamlV0_1 is a Version 0.1 yaml document specifying an inmemory
// storage driver with no parameters
var inmemoryConfigYamlV0_1 = `
version: 0.1
loglevel: info
storage: inmemory
auth:
  silly:
    realm: silly
    service: silly
notifications:
  endpoints:
    - name: endpoint-1
      url:  http://example.com
      headers:
        Authorization: [Bearer <example>]
`

type ConfigSuite struct {
	expectedConfig *Configuration
}

var _ = Suite(new(ConfigSuite))

func (suite *ConfigSuite) SetUpTest(c *C) {
	os.Clearenv()
	suite.expectedConfig = copyConfig(configStruct)
}

// TestMarshalRoundtrip validates that configStruct can be marshaled and
// unmarshaled without changing any parameters
func (suite *ConfigSuite) TestMarshalRoundtrip(c *C) {
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	c.Assert(err, IsNil)
	config, err := Parse(bytes.NewReader(configBytes))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseSimple validates that configYamlV0_1 can be parsed into a struct
// matching configStruct
func (suite *ConfigSuite) TestParseSimple(c *C) {
	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseInmemory validates that configuration yaml with storage provided as
// a string can be parsed into a Configuration struct with no storage parameters
func (suite *ConfigSuite) TestParseInmemory(c *C) {
	suite.expectedConfig.Storage = Storage{"inmemory": Parameters{}}
	suite.expectedConfig.Reporting = Reporting{}
	suite.expectedConfig.Log.Fields = nil

	config, err := Parse(bytes.NewReader([]byte(inmemoryConfigYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseIncomplete validates that an incomplete yaml configuration cannot
// be parsed without providing environment variables to fill in the missing
// components.
func (suite *ConfigSuite) TestParseIncomplete(c *C) {
	incompleteConfigYaml := "version: 0.1"
	_, err := Parse(bytes.NewReader([]byte(incompleteConfigYaml)))
	c.Assert(err, NotNil)

	suite.expectedConfig.Log.Fields = nil
	suite.expectedConfig.Storage = Storage{"filesystem": Parameters{"rootdirectory": "/tmp/testroot"}}
	suite.expectedConfig.Auth = Auth{"silly": Parameters{"realm": "silly"}}
	suite.expectedConfig.Reporting = Reporting{}
	suite.expectedConfig.Notifications = Notifications{}

	os.Setenv("REGISTRY_STORAGE", "filesystem")
	os.Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")
	os.Setenv("REGISTRY_AUTH", "silly")
	os.Setenv("REGISTRY_AUTH_SILLY_REALM", "silly")

	config, err := Parse(bytes.NewReader([]byte(incompleteConfigYaml)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithSameEnvStorage validates that providing environment variables
// that match the given storage type will only include environment-defined
// parameters and remove yaml-defined parameters
func (suite *ConfigSuite) TestParseWithSameEnvStorage(c *C) {
	suite.expectedConfig.Storage = Storage{"s3": Parameters{"region": "us-east-1"}}

	os.Setenv("REGISTRY_STORAGE", "s3")
	os.Setenv("REGISTRY_STORAGE_S3_REGION", "us-east-1")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageParams validates that providing environment variables that change
// and add to the given storage parameters will change and add parameters to the parsed
// Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageParams(c *C) {
	suite.expectedConfig.Storage.setParameter("region", "us-west-1")
	suite.expectedConfig.Storage.setParameter("secure", true)
	suite.expectedConfig.Storage.setParameter("newparam", "some Value")

	os.Setenv("REGISTRY_STORAGE_S3_REGION", "us-west-1")
	os.Setenv("REGISTRY_STORAGE_S3_SECURE", "true")
	os.Setenv("REGISTRY_STORAGE_S3_NEWPARAM", "some Value")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageType validates that providing an environment variable that
// changes the storage type will be reflected in the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageType(c *C) {
	suite.expectedConfig.Storage = Storage{"inmemory": Parameters{}}

	os.Setenv("REGISTRY_STORAGE", "inmemory")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithExtraneousEnvStorageParams validates that environment variables
// that change parameters out of the scope of the specified storage type are
// ignored.
func (suite *ConfigSuite) TestParseWithExtraneousEnvStorageParams(c *C) {
	os.Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvStorageTypeAndParams validates that providing an environment variable
// that changes the storage type will be reflected in the parsed Configuration struct and that
// environment storage parameters will also be included
func (suite *ConfigSuite) TestParseWithDifferentEnvStorageTypeAndParams(c *C) {
	suite.expectedConfig.Storage = Storage{"filesystem": Parameters{}}
	suite.expectedConfig.Storage.setParameter("rootdirectory", "/tmp/testroot")

	os.Setenv("REGISTRY_STORAGE", "filesystem")
	os.Setenv("REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY", "/tmp/testroot")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithSameEnvLoglevel validates that providing an environment variable defining the log
// level to the same as the one provided in the yaml will not change the parsed Configuration struct
func (suite *ConfigSuite) TestParseWithSameEnvLoglevel(c *C) {
	os.Setenv("REGISTRY_LOGLEVEL", "info")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseWithDifferentEnvLoglevel validates that providing an environment variable defining the
// log level will override the value provided in the yaml document
func (suite *ConfigSuite) TestParseWithDifferentEnvLoglevel(c *C) {
	suite.expectedConfig.Loglevel = "error"

	os.Setenv("REGISTRY_LOGLEVEL", "error")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseInvalidLoglevel validates that the parser will fail to parse a
// configuration if the loglevel is malformed
func (suite *ConfigSuite) TestParseInvalidLoglevel(c *C) {
	invalidConfigYaml := "version: 0.1\nloglevel: derp\nstorage: inmemory"
	_, err := Parse(bytes.NewReader([]byte(invalidConfigYaml)))
	c.Assert(err, NotNil)

	os.Setenv("REGISTRY_LOGLEVEL", "derp")

	_, err = Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, NotNil)

}

// TestParseWithDifferentEnvReporting validates that environment variables
// properly override reporting parameters
func (suite *ConfigSuite) TestParseWithDifferentEnvReporting(c *C) {
	suite.expectedConfig.Reporting.Bugsnag.APIKey = "anotherBugsnagApiKey"
	suite.expectedConfig.Reporting.Bugsnag.Endpoint = "localhost:8080"
	suite.expectedConfig.Reporting.NewRelic.LicenseKey = "NewRelicLicenseKey"
	suite.expectedConfig.Reporting.NewRelic.Name = "some NewRelic NAME"

	os.Setenv("REGISTRY_REPORTING_BUGSNAG_APIKEY", "anotherBugsnagApiKey")
	os.Setenv("REGISTRY_REPORTING_BUGSNAG_ENDPOINT", "localhost:8080")
	os.Setenv("REGISTRY_REPORTING_NEWRELIC_LICENSEKEY", "NewRelicLicenseKey")
	os.Setenv("REGISTRY_REPORTING_NEWRELIC_NAME", "some NewRelic NAME")

	config, err := Parse(bytes.NewReader([]byte(configYamlV0_1)))
	c.Assert(err, IsNil)
	c.Assert(config, DeepEquals, suite.expectedConfig)
}

// TestParseInvalidVersion validates that the parser will fail to parse a newer configuration
// version than the CurrentVersion
func (suite *ConfigSuite) TestParseInvalidVersion(c *C) {
	suite.expectedConfig.Version = MajorMinorVersion(CurrentVersion.Major(), CurrentVersion.Minor()+1)
	configBytes, err := yaml.Marshal(suite.expectedConfig)
	c.Assert(err, IsNil)
	_, err = Parse(bytes.NewReader(configBytes))
	c.Assert(err, NotNil)
}

func copyConfig(config Configuration) *Configuration {
	configCopy := new(Configuration)

	configCopy.Version = MajorMinorVersion(config.Version.Major(), config.Version.Minor())
	configCopy.Loglevel = config.Loglevel
	configCopy.Log = config.Log
	configCopy.Log.Fields = make(map[string]interface{}, len(config.Log.Fields))
	for k, v := range config.Log.Fields {
		configCopy.Log.Fields[k] = v
	}

	configCopy.Storage = Storage{config.Storage.Type(): Parameters{}}
	for k, v := range config.Storage.Parameters() {
		configCopy.Storage.setParameter(k, v)
	}
	configCopy.Reporting = Reporting{
		Bugsnag:  BugsnagReporting{config.Reporting.Bugsnag.APIKey, config.Reporting.Bugsnag.ReleaseStage, config.Reporting.Bugsnag.Endpoint},
		NewRelic: NewRelicReporting{config.Reporting.NewRelic.LicenseKey, config.Reporting.NewRelic.Name, config.Reporting.NewRelic.Verbose},
	}

	configCopy.Auth = Auth{config.Auth.Type(): Parameters{}}
	for k, v := range config.Auth.Parameters() {
		configCopy.Auth.setParameter(k, v)
	}

	configCopy.Notifications = Notifications{Endpoints: []Endpoint{}}
	for _, v := range config.Notifications.Endpoints {
		configCopy.Notifications.Endpoints = append(configCopy.Notifications.Endpoints, v)
	}

	return configCopy
}
